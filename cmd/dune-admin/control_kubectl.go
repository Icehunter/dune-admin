package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// kubectlControl implements ControlPlane using kubectl commands.
// Commands run through the provided Executor (local or SSH-tunneled).
type kubectlControl struct {
	namespace string // e.g. "funcom-seabass-mybg"
}

func (c *kubectlControl) Name() string { return "kubectl" }

func kubectlCLI(exec Executor) string {
	if exec != nil && exec.Type() == "local" {
		return "kubectl"
	}
	return "sudo kubectl"
}

func (c *kubectlControl) bgName() string {
	return strings.TrimPrefix(c.namespace, "funcom-seabass-")
}

func (c *kubectlControl) GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error) {
	bgName := c.bgName()
	kctl := kubectlCLI(exec)

	// startTimestamp drives uptime/age; gamePort + per-server fields come from the
	// battlegroup status (NOT serverstats, which lacks them — verified on a live
	// k3s cluster, #203).
	bgOut, _ := exec.Exec(fmt.Sprintf(
		`%s get battlegroups -n %s -o jsonpath="{.items[0].spec.title}|{.items[0].status.phase}|{.items[0].status.database.phase}|{.items[0].status.startTimestamp}" 2>/dev/null`,
		kctl, c.namespace))
	bgParts := strings.SplitN(strings.TrimSpace(bgOut), "|", 4)

	// partition → gamePort, read from battlegroup status.servers[].
	portByPartition := c.gamePortsByPartition(exec, kctl)
	ageSeconds := ageSecondsFromStartTime(safeIdx(bgParts, 3), time.Now())

	ssOut, _ := exec.Exec(fmt.Sprintf(
		"%s get serverstats -n %s -o jsonpath='{range .items[*]}{.spec.area.map}|{.spec.area.sietch}|{.spec.area.dimension}|{.spec.area.partition}|{.status.runtime.gamePhase}|{.status.runtime.ready}|{.status.runtime.players}{\"\\n\"}{end}' 2>/dev/null",
		kctl, c.namespace))

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
			Map:        p[0],
			Sietch:     p[1],
			Dimension:  dim,
			Partition:  part,
			Phase:      p[4],
			Ready:      p[5] == "true",
			Players:    players,
			Port:       portByPartition[part],
			AgeSeconds: ageSeconds,
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

// discoverWebInterfaces surfaces the director and file-browser URLs straight
// from the battlegroup status (status.utilities), so operators don't have to
// configure them by hand on kubectl. Implements webInterfaceDiscoverer.
func (c *kubectlControl) discoverWebInterfaces(_ context.Context, exec Executor) []webInterface {
	kctl := kubectlCLI(exec)
	out, _ := exec.Exec(fmt.Sprintf(
		`%s get battlegroups -n %s -o jsonpath="{.items[0].status.utilities.director.address}|{.items[0].status.utilities.fileBrowser.address}" 2>/dev/null`,
		kctl, c.namespace))
	directorAddr, fileBrowserAddr, _ := strings.Cut(strings.TrimSpace(out), "|")
	return webInterfacesFromAddresses(vmHostIP(), directorAddr, fileBrowserAddr)
}

// vmHostIP returns the host portion of the SSH target — the IP the operator uses
// to reach the game VM. Empty for a local executor (no SSH).
func vmHostIP() string {
	if sshHost == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(sshHost); err == nil {
		return host
	}
	return sshHost
}

// webInterfacesFromAddresses builds the discovered web-interface links from the
// raw host:port addresses reported by the battlegroup CRD. Empty addresses are
// skipped. The game's director and file browser serve over http on node ports
// (matching director_url's http convention).
func webInterfacesFromAddresses(vmHost, directorAddr, fileBrowserAddr string) []webInterface {
	var out []webInterface
	if url := webInterfaceURL(vmHost, directorAddr); url != "" {
		out = append(out, webInterface{Label: directorInterfaceLabel, URL: url, Target: strings.TrimSpace(directorAddr)})
	}
	if url := webInterfaceURL(vmHost, fileBrowserAddr); url != "" {
		out = append(out, webInterface{Label: "File Browser", URL: url, Target: strings.TrimSpace(fileBrowserAddr)})
	}
	return out
}

// webInterfaceURL turns a CRD-reported host:port into an operator-reachable URL.
// The CRD advertises a node IP that is often a public/WAN address the operator
// can't route to, so the host is rewritten to the VM IP dune-admin connects to
// (vmHost), keeping the node port. Falls back to the reported host when vmHost
// is unknown (local executor).
func webInterfaceURL(vmHost, addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil { // no port component
		host, port = addr, ""
	}
	if vmHost != "" {
		host = vmHost
	}
	if host == "" {
		return ""
	}
	if port != "" {
		return "http://" + net.JoinHostPort(host, port) + "/"
	}
	return "http://" + host + "/"
}

// gamePortsByPartition reads partition→gamePort from the battlegroup status.
// Returns an empty map (never nil) when the field is absent so callers can index
// it freely; a missing port leaves ServerRow.Port at 0 (UI shows "—").
func (c *kubectlControl) gamePortsByPartition(exec Executor, kctl string) map[int]int {
	out, _ := exec.Exec(fmt.Sprintf(
		"%s get battlegroups -n %s -o jsonpath='{range .items[0].status.servers[*]}{.partitionIndex}|{.gamePort}{\"\\n\"}{end}' 2>/dev/null",
		kctl, c.namespace))
	ports := map[int]int{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p := strings.SplitN(line, "|", 2)
		if len(p) < 2 {
			continue
		}
		part, err1 := strconv.Atoi(strings.TrimSpace(p[0]))
		port, err2 := strconv.Atoi(strings.TrimSpace(p[1]))
		if err1 == nil && err2 == nil {
			ports[part] = port
		}
	}
	return ports
}

// ageSecondsFromStartTime parses an RFC3339 start timestamp and returns the
// elapsed seconds relative to now. Returns 0 for empty/unparseable/future
// values so a missing field leaves AgeSeconds at 0 (UI shows "—").
func ageSecondsFromStartTime(ts string, now time.Time) int {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return 0
	}
	start, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	secs := int(now.Sub(start).Seconds())
	if secs < 0 {
		return 0
	}
	return secs
}

func (c *kubectlControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	bgName := c.bgName()
	ns := c.namespace
	kctl := kubectlCLI(exec)

	switch cmd {
	case "start":
		return exec.Exec(fmt.Sprintf(
			`%s patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":false}}' 2>&1 && echo "Battlegroup starting"`,
			kctl, bgName, ns))
	case "stop":
		return exec.Exec(fmt.Sprintf(
			`%s patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":true}}' 2>&1 && echo "Battlegroup stopping"`,
			kctl, bgName, ns))
	case "restart":
		return exec.Exec(fmt.Sprintf(
			`%s patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":true}}' 2>/dev/null && sleep 5 && %s patch battlegroup %s -n %s --type=merge -p '{"spec":{"stop":false}}' 2>/dev/null && echo "Battlegroup restarting"`,
			kctl, bgName, ns, kctl, bgName, ns))
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
	kctl := kubectlCLI(exec)
	out, err := exec.Exec(fmt.Sprintf("%s get pods -n %s --no-headers 2>&1", kctl, c.namespace))
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
	kctl := kubectlCLI(exec)
	out, err := exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>&1", kctl, c.namespace))
	if err != nil {
		return nil, fmt.Errorf("kubectl: %w", err)
	}
	out2, _ := exec.Exec(
		fmt.Sprintf("%s get pods -n funcom-operators --no-headers -o custom-columns=NAME:.metadata.name 2>&1", kctl))

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
	kctl := kubectlCLI(exec)
	cmd := fmt.Sprintf("%s logs -f -n %s %s 2>&1", kctl, ns, name)
	return exec.Stream(cmd)
}

func (c *kubectlControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	kctl := kubectlCLI(exec)
	pod, err := exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
		kctl, c.namespace))
	if err != nil || strings.TrimSpace(pod) == "" {
		return "", "", fmt.Errorf("find bgd pod: %w", err)
	}
	pod = strings.TrimSpace(pod)

	existingToken, err := exec.Exec(fmt.Sprintf(
		"%s exec -n %s %s -- env 2>/dev/null | grep FuncomLiveServices__ServiceAuthToken | cut -d= -f2-",
		kctl, c.namespace, pod))
	if err != nil || strings.TrimSpace(existingToken) == "" {
		return "", "", fmt.Errorf("read ServiceAuthToken: %w", err)
	}
	return buildCaptureJWT(strings.TrimSpace(existingToken))
}

func (c *kubectlControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	if c.namespace == "" {
		return "", errNotSupported("kubectl", "EvalOnGameBroker (namespace not configured)")
	}
	kctl := kubectlCLI(exec)
	pod, err := exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep mq-game | head -1",
		kctl, c.namespace))
	if err != nil || strings.TrimSpace(pod) == "" {
		return "", fmt.Errorf("could not find mq-game pod in namespace %s", c.namespace)
	}
	pod = strings.TrimSpace(pod)
	out, err := exec.Exec(fmt.Sprintf(
		"%s exec -n %s %s -- rabbitmqctl eval %s 2>&1",
		kctl, c.namespace, pod, shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// ── kubectl-specific discovery helpers (used by setup wizard) ─────────────────

// discoverDBPod uses kubectl to find the DB pod, returning namespace, name, and pod IP.
func discoverDBPod(exec Executor) (ns, pod, podIP string, err error) {
	kctl := kubectlCLI(exec)
	out, err := exec.Exec(
		fmt.Sprintf(`%s get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{" "}{.status.podIP}{"\n"}{end}' 2>/dev/null | grep db-dbdepl-sts | head -1`, kctl))
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

// tryReadINIFromPod attempts to read filename from a specific pod by trying
// well-known Config paths first, then falling back to a find-based search.
func tryReadINIFromPod(exec Executor, kctl, namespace, pod, filename string) string {
	candidates := []string{
		"/DuneSandbox/Config/" + filename,
		"/home/dune/server/DuneSandbox/Config/" + filename,
		"/home/dune/DuneSandbox/Config/" + filename,
		"/game/DuneSandbox/Config/" + filename,
	}
	for _, p := range candidates {
		content, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- cat %s 2>/dev/null",
			kctl, namespace, pod, shellQuote(p)))
		if err == nil && len(strings.TrimSpace(content)) > 0 {
			componentLog("control_kubectl").Debug().Str("path", p).Int("bytes", len(content)).Str("pod", pod).Msg("default-ini read from pod")
			return content
		}
	}
	pathOut, _ := exec.Exec(fmt.Sprintf(
		"%s exec -n %s %s -- find -L /DuneSandbox /home /app /game -name %s -not -path '*/Saved/*' 2>/dev/null | head -1",
		kctl, namespace, pod, shellQuote(filename)))
	if p := strings.TrimSpace(pathOut); p != "" {
		content, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- cat %s 2>/dev/null",
			kctl, namespace, pod, shellQuote(p)))
		if err == nil && len(strings.TrimSpace(content)) > 0 {
			componentLog("control_kubectl").Debug().Str("path", p).Int("bytes", len(content)).Str("pod", pod).Msg("default-ini read from pod")
			return content
		}
	}
	return ""
}

func (c *kubectlControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	if c.namespace == "" {
		return ""
	}
	kctl := kubectlCLI(exec)

	podOut, err := exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null",
		kctl, c.namespace))
	if err != nil {
		return ""
	}

	var sgPods, bgdPods, otherPods []string
	for _, line := range strings.Split(podOut, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		switch {
		case strings.Contains(name, "-sg-"):
			sgPods = append(sgPods, name)
		case strings.Contains(name, "bgd"):
			bgdPods = append(bgdPods, name)
		default:
			otherPods = append(otherPods, name)
		}
	}
	sort.Strings(sgPods)
	sort.Strings(bgdPods)
	sort.Strings(otherPods)
	pods := append(append(sgPods, bgdPods...), otherPods...)
	if len(pods) == 0 {
		return ""
	}

	for _, pod := range pods {
		if content := tryReadINIFromPod(exec, kctl, c.namespace, pod, filename); content != "" {
			return content
		}
	}

	componentLog("control_kubectl").Warn().Str("filename", filename).Str("namespace", c.namespace).Msg("default-ini not found in namespace")
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
