package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

// kubectlControl implements ControlPlane using kubectl commands.
// Commands run through the provided Executor (local or SSH-tunneled).
type kubectlControl struct {
	namespace string // e.g. "funcom-seabass-mybg"
}

func (c *kubectlControl) Name() string { return "kubectl" }

func (c *kubectlControl) bgName() string {
	return strings.TrimPrefix(c.namespace, "funcom-seabass-")
}

func (c *kubectlControl) GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error) {
	bgName := c.bgName()

	bgOut, _ := exec.Exec(fmt.Sprintf(
		`sudo kubectl get battlegroups -n %s -o jsonpath="{.items[0].spec.title}|{.items[0].status.phase}|{.items[0].status.database.phase}" 2>/dev/null`,
		c.namespace))

	bgParts := strings.SplitN(strings.TrimSpace(bgOut), "|", 3)

	ssOut, _ := exec.Exec(fmt.Sprintf(
		"sudo kubectl get serverstats -n %s -o jsonpath='{range .items[*]}{.spec.area.map}|{.spec.area.sietch}|{.spec.area.dimension}|{.spec.area.partition}|{.status.runtime.gamePhase}|{.status.runtime.ready}|{.status.runtime.players}{\"\\n\"}{end}' 2>/dev/null",
		c.namespace))

	var servers []ServerRow
	for _, line := range strings.Split(strings.TrimSpace(ssOut), "\n") {
		if line == "" {
			continue
		}
		p := strings.SplitN(line, "|", 7)
		if len(p) < 7 {
			continue
		}
		dim, _ := strconv.Atoi(p[2])
		part, _ := strconv.Atoi(p[3])
		players, _ := strconv.Atoi(p[6])
		servers = append(servers, ServerRow{
			Map:       p[0],
			Sietch:    p[1],
			Dimension: dim,
			Partition: part,
			Phase:     p[4],
			Ready:     p[5] == "true",
			Players:   players,
		})
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Map < servers[j].Map })
	if servers == nil {
		servers = []ServerRow{}
	}

	return &BattlegroupStatus{
		Name:     bgName,
		Title:    safeIdx(bgParts, 0),
		Phase:    safeIdx(bgParts, 1),
		Database: safeIdx(bgParts, 2),
		Servers:  servers,
	}, nil
}

func (c *kubectlControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	bgName := c.bgName()
	ns := c.namespace

	switch cmd {
	case "start":
		return exec.Exec(fmt.Sprintf(
			`sudo kubectl patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":false}}' 2>&1 && echo "Battlegroup starting"`,
			bgName, ns))
	case "stop":
		return exec.Exec(fmt.Sprintf(
			`sudo kubectl patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":true}}' 2>&1 && echo "Battlegroup stopping"`,
			bgName, ns))
	case "restart":
		return exec.Exec(fmt.Sprintf(
			`sudo kubectl patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":true}}' 2>/dev/null && sleep 5 && sudo kubectl patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":false}}' 2>/dev/null && echo "Battlegroup restarting"`,
			bgName, ns, bgName, ns))
	default:
		// TODO: NEVER run battlegroup.sh with sudo. The script manages files under
		// /home/dune/.dune/ and runs as the dune user. Using sudo corrupts ownership
		// of those files (bin/, symlinks, etc.) and breaks all subsequent battlegroup
		// commands until permissions are manually repaired. kubectl commands above
		// legitimately need sudo; battlegroup.sh does NOT.
		return exec.Exec(fmt.Sprintf("~/.dune/download/scripts/battlegroup.sh %s 2>&1", cmd))
	}
}

func (c *kubectlControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	out, err := exec.Exec(fmt.Sprintf("sudo kubectl get pods -n %s --no-headers 2>&1", c.namespace))
	if err != nil {
		return nil, "", fmt.Errorf("kubectl: %w", err)
	}
	var procs []ProcessInfo
	for _, line := range splitLines(out) {
		if line != "" {
			procs = append(procs, ProcessInfo{Name: line, Namespace: c.namespace})
		}
	}
	return procs, c.namespace, nil
}

func (c *kubectlControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	out, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>&1", c.namespace))
	if err != nil {
		return nil, fmt.Errorf("kubectl: %w", err)
	}
	out2, _ := exec.Exec(
		"sudo kubectl get pods -n funcom-operators --no-headers -o custom-columns=NAME:.metadata.name 2>&1")

	var sources []LogSource
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name != "" && !strings.Contains(name, "db-dbdepl") {
			sources = append(sources, LogSource{Namespace: c.namespace, Name: name})
		}
	}
	for _, line := range splitLines(out2) {
		name := strings.TrimSpace(line)
		if name != "" {
			sources = append(sources, LogSource{Namespace: "funcom-operators", Name: name})
		}
	}
	return sources, nil
}

func (c *kubectlControl) StreamLog(_ context.Context, exec Executor, ns, name string) (<-chan string, func(), error) {
	cmd := fmt.Sprintf("sudo kubectl logs -f -n %s %s 2>&1", ns, name)
	return exec.Stream(cmd)
}

func (c *kubectlControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	pod, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
		c.namespace))
	if err != nil || strings.TrimSpace(pod) == "" {
		return "", "", fmt.Errorf("find bgd pod: %w", err)
	}
	pod = strings.TrimSpace(pod)

	existingToken, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl exec -n %s %s -- env 2>/dev/null | grep FuncomLiveServices__ServiceAuthToken | cut -d= -f2-",
		c.namespace, pod))
	if err != nil || strings.TrimSpace(existingToken) == "" {
		return "", "", fmt.Errorf("read ServiceAuthToken: %w", err)
	}
	return buildCaptureJWT(strings.TrimSpace(existingToken))
}

func (c *kubectlControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	if c.namespace == "" {
		return "", errNotSupported("kubectl", "EvalOnGameBroker (namespace not configured)")
	}
	pod, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep mq-game | head -1",
		c.namespace))
	if err != nil || strings.TrimSpace(pod) == "" {
		return "", fmt.Errorf("could not find mq-game pod in namespace %s", c.namespace)
	}
	pod = strings.TrimSpace(pod)
	out, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl exec -n %s %s -- rabbitmqctl eval %s 2>&1",
		c.namespace, pod, shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// ── kubectl-specific discovery helpers (used by setup wizard) ─────────────────

// discoverDBPod uses kubectl to find the DB pod, returning namespace, name, and pod IP.
func discoverDBPod(exec Executor) (ns, pod, podIP string, err error) {
	out, err := exec.Exec(
		`sudo kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{" "}{.status.podIP}{"\n"}{end}' 2>/dev/null | grep db-dbdepl-sts | head -1`)
	if err != nil {
		return "", "", "", fmt.Errorf("kubectl: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("database pod not found in cluster")
	}
	return parts[0], parts[1], parts[2], nil
}

// battlegroupFromPod extracts the battlegroup name from a pod name.
// Pod naming pattern: <battlegroup>-db-dbdepl-sts-<N>
func battlegroupFromPod(pod string) string {
	const suffix = "-db-dbdepl-sts-"
	if idx := strings.LastIndex(pod, suffix); idx != -1 {
		return pod[:idx]
	}
	return ""
}

// listBattlegroups returns battlegroup names via the battlegroup CLI.
func listBattlegroups(exec Executor) []string {
	out, err := exec.Exec("bash -lc 'battlegroup list' 2>/dev/null")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			if name := strings.TrimSpace(line[2:]); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// extractPasswordFromYAML reads DB credentials from a battlegroup YAML on the executor.
func extractPasswordFromYAML(exec Executor, filePath string) (user, pass string) {
	out, err := exec.Exec(fmt.Sprintf("cat %s 2>/dev/null", shellQuote(filePath)))
	if err != nil || len(out) == 0 {
		out, err = exec.Exec(fmt.Sprintf("bash -c 'cat %s'", filePath))
		if err != nil || len(out) == 0 {
			return "", ""
		}
	}
	return parseDeploymentCredentials([]byte(out))
}

func (c *kubectlControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	if c.namespace == "" {
		return ""
	}
	// Find the game daemon pod — same pattern used by CaptureJWT.
	pod, err := exec.Exec(fmt.Sprintf(
		"sudo kubectl get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
		c.namespace))
	if err != nil || strings.TrimSpace(pod) == "" {
		return ""
	}
	pod = strings.TrimSpace(pod)

	// Try well-known Config directories directly. find / misses paths that are
	// only accessible via bind-mount roots or require following symlinks.
	candidates := []string{
		"/DuneSandbox/Config/" + filename,
		"/home/dune/server/DuneSandbox/Config/" + filename,
		"/home/dune/DuneSandbox/Config/" + filename,
		"/game/DuneSandbox/Config/" + filename,
	}
	for _, p := range candidates {
		content, err := exec.Exec(fmt.Sprintf(
			"sudo kubectl exec -n %s %s -- cat %s 2>/dev/null",
			c.namespace, pod, shellQuote(p)))
		if err == nil && len(strings.TrimSpace(content)) > 0 {
			log.Printf("[default-ini] kubectl: read %s (%d bytes) from pod %s", p, len(content), pod)
			return content
		}
	}

	// Fall back: find with symlink-following under game-relevant roots.
	pathOut, _ := exec.Exec(fmt.Sprintf(
		"sudo kubectl exec -n %s %s -- find -L /DuneSandbox /home /app /game -name %s -not -path '*/Saved/*' 2>/dev/null | head -1",
		c.namespace, pod, shellQuote(filename)))
	if p := strings.TrimSpace(pathOut); p != "" {
		content, err := exec.Exec(fmt.Sprintf(
			"sudo kubectl exec -n %s %s -- cat %s 2>/dev/null",
			c.namespace, pod, shellQuote(p)))
		if err == nil && len(strings.TrimSpace(content)) > 0 {
			log.Printf("[default-ini] kubectl: read %s (%d bytes) from pod %s", p, len(content), pod)
			return content
		}
	}

	log.Printf("[default-ini] kubectl: %s not found in pod %s", filename, pod)
	return ""
}

func (c *kubectlControl) DiscoverIniDir(_ context.Context, exec Executor) (string, error) {
	if c.namespace == "" {
		return "", fmt.Errorf("namespace not discovered yet; reconnect or set server_ini_dir in config")
	}
	// k3s local-path storage: /var/lib/rancher/k3s/storage/<vol>_<ns>_<pvc>/Saved/UserSettings
	out, err := exec.Exec(fmt.Sprintf(
		`sudo ls /var/lib/rancher/k3s/storage/ 2>/dev/null | grep -F %s | grep -v -- '-db-pvc' | head -1`,
		shellQuote(c.namespace)))
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("could not auto-discover ini dir for namespace %s; set server_ini_dir in config", c.namespace)
	}
	dir := "/var/lib/rancher/k3s/storage/" + strings.TrimSpace(out) + "/Saved/UserSettings"
	return dir, nil
}
