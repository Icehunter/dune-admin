package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ampControl implements ControlPlane for CubeCoders AMP installations. AMP can
// run the game server in two modes:
//
//   - containerised: game processes run inside a podman container. Log/INI access
//     and broker control require `podman exec`. Set useContainer = true.
//   - native: game processes run directly on the host as the AMP user. Logs and
//     INI files are on the host filesystem; rabbitmqctl is on the host PATH. Set
//     useContainer = false.
//
// Process discovery (GetStatus/ListProcesses/CaptureJWT) is identical in both
// modes — game-server processes appear in the host's `ps` output regardless.
//
// All instance- and container-specific names come from config; this provider
// is not specialised to any particular AMP install.
type ampControl struct {
	instance     string // ampinstmgr instance name (e.g. "MehDune01")
	container    string // podman container name (only used when useContainer=true)
	ampUser      string // OS user that owns the AMP instance (default "amp")
	logPath      string // log directory — in-container path if containerised, host path if native
	directorURL  string // optional Battlegroup Director URL for status/exchange discovery
	iniDir       string // host path to UserGame.ini directory (configured)
	useContainer bool   // true: wrap in-container ops in `podman exec`; false: run on host directly
}

func (c *ampControl) Name() string { return "amp" }

// ── status & lifecycle ────────────────────────────────────────────────────────

var (
	ampPortRe = regexp.MustCompile(`-Port=(\d+)`)
	ampPartRe = regexp.MustCompile(`-PartitionIndex=(\d+)`)
)

func (c *ampControl) GetStatus(_ context.Context, exec Executor) (*BattlegroupStatus, error) {
	procs, err := c.listGameProcesses(exec)
	if err != nil {
		return nil, err
	}
	servers := make([]ServerRow, 0, len(procs))
	for _, p := range procs {
		servers = append(servers, ServerRow{
			Map:       p.mapName,
			Partition: p.partition,
			Phase:     "Running",
			Ready:     true,
			Players:   0,
		})
	}
	dbPhase := "Disconnected"
	if globalDB != nil {
		dbPhase = "Connected"
	}
	return &BattlegroupStatus{
		Name:     c.container,
		Title:    "AMP Managed",
		Phase:    "Running",
		Database: dbPhase,
		Servers:  servers,
	}, nil
}

func (c *ampControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	if c.instance == "" {
		return "", fmt.Errorf("amp control plane requires amp_instance to be set")
	}
	switch cmd {
	case "start":
		return exec.Exec(fmt.Sprintf("sudo -i -u %s ampinstmgr -s %s 2>&1", c.ampUser, c.instance))
	case "stop":
		return exec.Exec(fmt.Sprintf("sudo -i -u %s ampinstmgr -q %s 2>&1", c.ampUser, c.instance))
	case "restart":
		return exec.Exec(fmt.Sprintf("sudo -i -u %s ampinstmgr -q %s 2>&1 && sudo -i -u %s ampinstmgr -s %s 2>&1",
			c.ampUser, c.instance, c.ampUser, c.instance))
	default:
		return "", fmt.Errorf("amp control does not support %q", cmd)
	}
}

// ── process & log discovery ───────────────────────────────────────────────────

type ampGameProcess struct {
	pid       int
	mapName   string
	port      int
	partition int
}

func (c *ampControl) listGameProcesses(exec Executor) ([]ampGameProcess, error) {
	out, err := exec.Exec(`ps -eo pid,args --no-headers 2>/dev/null | grep 'DuneSandboxServer-Linux-Shipping' | grep -v grep`)
	if err != nil && strings.TrimSpace(out) == "" {
		return []ampGameProcess{}, nil
	}
	var procs []ampGameProcess
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		args := strings.Join(fields[1:], " ")
		mapName := ""
		for i, p := range fields[1:] {
			if p == "DuneSandbox" && i+1 < len(fields[1:]) {
				mapName = fields[1+i+1]
				break
			}
		}
		port := 0
		if m := ampPortRe.FindStringSubmatch(args); len(m) > 1 {
			port, _ = strconv.Atoi(m[1])
		}
		partition := 0
		if m := ampPartRe.FindStringSubmatch(args); len(m) > 1 {
			partition, _ = strconv.Atoi(m[1])
		}
		procs = append(procs, ampGameProcess{pid: pid, mapName: mapName, port: port, partition: partition})
	}
	return procs, nil
}

func (c *ampControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	procs, err := c.listGameProcesses(exec)
	if err != nil {
		return nil, "", err
	}
	var infos []ProcessInfo
	for _, p := range procs {
		infos = append(infos, ProcessInfo{
			Name:      fmt.Sprintf("%s (pid=%d port=%d partition=%d)", p.mapName, p.pid, p.port, p.partition),
			Namespace: c.container,
			Status:    "Running",
		})
	}
	if infos == nil {
		infos = []ProcessInfo{}
	}
	return infos, c.container, nil
}

// wrapInContainer returns a command string that, when executed via the host
// executor, runs the given remote command. In container mode this is wrapped
// in `sudo -i -u <ampUser> podman exec <container> sh -c '<remoteCmd>'`. In
// native mode it's wrapped in `sudo -i -u <ampUser> sh -c '<remoteCmd>'`.
//
// The remote command is single-quoted; the caller MUST NOT embed single quotes
// in the command itself.
func (c *ampControl) wrapInContainer(remoteCmd string) string {
	if c.useContainer {
		return fmt.Sprintf("sudo -i -u %s podman exec %s sh -c %s",
			c.ampUser, c.container, shellQuote(remoteCmd))
	}
	return fmt.Sprintf("sudo -i -u %s sh -c %s", c.ampUser, shellQuote(remoteCmd))
}

func (c *ampControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	if c.logPath == "" {
		return nil, fmt.Errorf("amp control requires amp_log_path to be set")
	}
	if c.useContainer && c.container == "" {
		return nil, fmt.Errorf("amp control in container mode requires amp_container to be set")
	}
	cmd := c.wrapInContainer(fmt.Sprintf("ls -1 %s 2>/dev/null", c.logPath))
	out, err := exec.Exec(cmd)
	if err != nil {
		return nil, fmt.Errorf("list log dir: %w (%s)", err, out)
	}
	ns := c.container
	if !c.useContainer {
		ns = "host:" + c.logPath
	}
	var sources []LogSource
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name := strings.TrimSpace(line)
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		sources = append(sources, LogSource{Namespace: ns, Name: name})
	}
	if sources == nil {
		sources = []LogSource{}
	}
	return sources, nil
}

var ampLogFileNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+\.log$`)

func (c *ampControl) StreamLog(_ context.Context, exec Executor, _, name string) (<-chan string, func(), error) {
	if !ampLogFileNameRe.MatchString(name) {
		return nil, func() {}, fmt.Errorf("invalid log file name %q", name)
	}
	cmd := c.wrapInContainer(fmt.Sprintf("tail -n 200 -f %s/%s", c.logPath, name))
	return exec.Stream(cmd)
}

// ── JWT capture ───────────────────────────────────────────────────────────────

func (c *ampControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	out, err := exec.Exec(`ps aux 2>/dev/null | grep DuneSandboxServer | grep -oP 'ServiceAuthToken=\K[^ ]+' | head -1`)
	if err != nil || strings.TrimSpace(out) == "" {
		return "", "", fmt.Errorf("could not find ServiceAuthToken in process args (game server not running?)")
	}
	token := strings.TrimSpace(out)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", fmt.Errorf("parse JWT payload: %w", err)
	}
	hostID := fmt.Sprintf("%v", claims["HostId"])
	return hostID, token, nil
}

// ── RabbitMQ admin (exchange listing + capture user provisioning) ─────────────

// rabbitmqctlPrefix returns the shell-prefix that runs rabbitmqctl for this
// AMP install. If an explicit broker_exec_prefix is configured we honour it.
// Otherwise: container mode wraps in `podman exec`; native mode runs rabbitmqctl
// directly on the host (as the AMP user, so RABBITMQ_ERLANG_COOKIE is read from
// ~amp/.erlang.cookie).
func (c *ampControl) rabbitmqctlPrefix(prefix string) string {
	if prefix != "" {
		return prefix + " rabbitmqctl"
	}
	if c.useContainer && c.container != "" {
		return fmt.Sprintf("sudo -i -u %s podman exec %s rabbitmqctl", c.ampUser, c.container)
	}
	return fmt.Sprintf("sudo -i -u %s rabbitmqctl", c.ampUser)
}

func (c *ampControl) ListExchanges(_ context.Context, exec Executor, _ string) ([]binding, error) {
	base := c.rabbitmqctlPrefix(loadedConfig.BrokerExecPrefix)
	raw, err := exec.Exec(base + " list_exchanges name 2>/dev/null")
	if err != nil {
		return nil, fmt.Errorf("rabbitmqctl: %w", err)
	}
	return parseExchanges(raw), nil
}

func (c *ampControl) EnsureCaptureUser(_ context.Context, exec Executor) {
	base := c.rabbitmqctlPrefix(loadedConfig.BrokerExecPrefix)
	out, _ := exec.Exec(fmt.Sprintf("%s add_user %s %s 2>&1", base, capUser, capPass))
	if !strings.Contains(out, "already exists") {
		fmt.Printf("[capture] [amp] created user %s\n", capUser)
	}
	_, _ = exec.Exec(fmt.Sprintf("%s change_password %s %s 2>&1", base, capUser, capPass))
	_, _ = exec.Exec(fmt.Sprintf("%s set_permissions -p / %s '.*' '.*' '.*' 2>&1", base, capUser))
	_, _ = exec.Exec(fmt.Sprintf(
		"%s eval 'application:set_env(rabbit, auth_backends, [{rabbit_auth_backend_cache, rabbit_auth_backend_http}, rabbit_auth_backend_internal]).' 2>&1",
		base))
	_, _ = exec.Exec(fmt.Sprintf(
		"%s eval 'application:set_env(rabbitmq_auth_backend_cache, cache_ttl, 86400000).' 2>&1",
		base))
	fmt.Println("[capture] [amp] auth backends updated")
}

// startEnsureCaptureUserLoop reapplies the dune_cap user every 15s. AMP can
// restart the broker container without notice, which resets the in-memory user
// list; this loop self-heals capture-mode after such restarts.
//
// Call once from runCapture for the amp provider; the goroutine runs until
// process exit.
func (c *ampControl) startEnsureCaptureUserLoop(exec Executor) {
	go func() {
		for {
			time.Sleep(15 * time.Second)
			c.EnsureCaptureUser(context.Background(), exec)
		}
	}()
}

// ── INI discovery ─────────────────────────────────────────────────────────────

func (c *ampControl) DiscoverIniDir(_ context.Context, _ Executor) (string, error) {
	if c.iniDir != "" {
		return c.iniDir, nil
	}
	if c.instance != "" {
		// Conventional AMP path. The setup wizard prefills this so users rarely
		// hit the fallback.
		return filepath.ToSlash(fmt.Sprintf(
			"/home/%s/.ampdata/instances/%s/duneawakening/server/state",
			c.ampUser, c.instance)), nil
	}
	return "", fmt.Errorf("amp control requires server_ini_dir or amp_instance to derive an INI directory")
}

// ReadDefaultINI returns the contents of DefaultGame.ini / DefaultEngine.ini.
// In container mode this `find`s inside the game container; in native mode it
// searches under the AMP install root. Returns "" when nothing matches so the
// host-path traversal in handlers_server_settings.go can take over.
func (c *ampControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	if c.useContainer && c.container == "" {
		return ""
	}
	findRoot := "/"
	if !c.useContainer {
		// Native AMP installs put the game tree under /AMP/<game>/. Scan that
		// instead of /, which is faster and avoids permission noise.
		findRoot = "/AMP"
	}
	out, err := exec.Exec(c.wrapInContainer(fmt.Sprintf(
		"find %s -name %s -not -path '*/Saved/*' -not -path '*/saved/*' 2>/dev/null | head -1",
		findRoot, filename)))
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	path := strings.TrimSpace(out)
	out, err = exec.Exec(c.wrapInContainer(fmt.Sprintf("cat %s 2>/dev/null", path)))
	if err != nil {
		return ""
	}
	return out
}
