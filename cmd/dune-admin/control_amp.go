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
)

// ampControl implements ControlPlane for CubeCoders AMP installations. AMP can
// run the game server in two modes:
//
//   - containerised: game processes run inside a container (podman or docker).
//     Log/INI access and broker control require `<runtime> exec`; choose the
//     runtime with containerRuntime. Set useContainer = true.
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
	instance         string // ampinstmgr instance name (e.g. "MehDune01")
	container        string // container name (only used when useContainer=true)
	ampUser          string // OS user that owns the AMP instance (default "amp")
	logPath          string // log directory — in-container path if containerised, host path if native
	directorURL      string // optional Battlegroup Director URL for status/exchange discovery
	iniDir           string // host path to UserGame.ini directory (configured)
	useContainer     bool   // true: wrap in-container ops in `<runtime> exec`; false: run on host directly
	containerRuntime string // "podman" (default) or "docker"; CLI for `<rt> exec` in container mode
	dataRoot         string // per-game data root (default /AMP/duneawakening)
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

func parseAMPMapName(argsFields []string) string {
	for i, field := range argsFields {
		if field == "DuneSandbox" && i+1 < len(argsFields) {
			return argsFields[i+1]
		}
	}
	return ""
}

func parseAMPArgInt(re *regexp.Regexp, args string) int {
	m := re.FindStringSubmatch(args)
	if len(m) <= 1 {
		return 0
	}
	value, _ := strconv.Atoi(m[1])
	return value
}

func parseAMPGameProcess(line string) (ampGameProcess, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ampGameProcess{}, false
	}
	pid, _ := strconv.Atoi(fields[0])
	argsFields := fields[1:]
	args := strings.Join(argsFields, " ")
	return ampGameProcess{
		pid:       pid,
		mapName:   parseAMPMapName(argsFields),
		port:      parseAMPArgInt(ampPortRe, args),
		partition: parseAMPArgInt(ampPartRe, args),
	}, true
}

func (c *ampControl) listGameProcesses(exec Executor) ([]ampGameProcess, error) {
	out, err := exec.Exec(`ps -eo pid,args --no-headers 2>/dev/null | grep 'DuneSandboxServer-Linux-Shipping' | grep -v grep`)
	if err != nil && strings.TrimSpace(out) == "" {
		return []ampGameProcess{}, nil
	}
	var procs []ampGameProcess
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		proc, ok := parseAMPGameProcess(line)
		if !ok {
			continue
		}
		procs = append(procs, proc)
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

// runtimeCLI returns the container CLI used to wrap in-container operations as
// `<rt> exec` when useContainer is true. Defaults to podman when unset so
// existing (podman) installs are unaffected.
func (c *ampControl) runtimeCLI() string {
	if c.containerRuntime == "" {
		return "podman"
	}
	return c.containerRuntime
}

// wrapInContainer returns a command string that, when executed via the host
// executor, runs the given remote command. In container mode this is wrapped
// in `sudo -i -u <ampUser> <runtime> exec <container> sh -c '<remoteCmd>'`. In
// native mode it's wrapped in `sudo -i -u <ampUser> sh -c '<remoteCmd>'`.
//
// The remote command is single-quoted; the caller MUST NOT embed single quotes
// in the command itself.
func (c *ampControl) wrapInContainer(remoteCmd string) string {
	if c.useContainer {
		return fmt.Sprintf("sudo -i -u %s %s exec %s sh -c %s",
			c.ampUser, c.runtimeCLI(), c.container, shellQuote(remoteCmd))
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

// defaultAmpDataRoot is the in-container per-game data root that AMP creates
// for the Dune Awakening module.
const defaultAmpDataRoot = "/AMP/duneawakening"

// ampDataRoot returns the AMP per-game data root (defaults to Dune Awakening).
func (c *ampControl) ampDataRoot() string {
	if c.dataRoot != "" {
		return c.dataRoot
	}
	return defaultAmpDataRoot
}

// buildRabbitmqctl emits a complete shell command that runs rabbitmqctl
// against one of AMP's brokers. AMP bundles its own musl-linked Erlang
// runtime but only patchelfs the binaries it boots at startup (beam.smp);
// the admin-CLI escript binary is left with the original /lib/ld-musl-* shebang.
// To call it from outside AMP's normal launch path we have to:
//   - invoke the bundled musl loader explicitly (works around the missing
//     /lib/ld-musl-x86_64.so.1 on Debian-based AMP containers)
//   - chain through the bundled escript and the rabbitmqctl escript wrapper
//   - set HOME to the broker's runtime dir so the right .erlang.cookie is
//     used (each broker has its own cookie under runtime/mq-<broker>-home/)
//   - point RABBITMQ_HOME at the AMP-bundled rabbitmq install
//   - target the right Erlang node name (rabbit-admin or rabbit-game)
//
// broker = "mq-admin" or "mq-game". args is the rabbitmqctl subcommand
// plus its arguments, already shell-quoted by the caller as needed.
func (c *ampControl) buildRabbitmqctl(broker, args string) string {
	root := c.ampDataRoot()
	mq := root + "/extracted/mq"
	home := root + "/runtime/" + broker + "-home"
	node := "rabbit-admin@localhost"
	if strings.Contains(broker, "game") {
		node = "rabbit-game@localhost"
	}
	inner := fmt.Sprintf(
		"env -i HOME=%s LC_ALL=C "+
			"LD_LIBRARY_PATH=%[2]s/lib:%[2]s/usr/lib:%[2]s/opt/openssl/lib "+
			"RABBITMQ_HOME=%[2]s/opt/rabbitmq "+
			"%[2]s/lib/ld-musl-x86_64.so.1 "+
			"%[2]s/opt/erlang/lib/erlang/bin/escript "+
			"%[2]s/opt/rabbitmq/escript/rabbitmqctl "+
			"--node %s %s",
		home, mq, node, args)
	if c.useContainer && c.container != "" {
		return fmt.Sprintf("sudo -i -u %s %s exec %s sh -c %s",
			c.ampUser, c.runtimeCLI(), c.container, shellQuote(inner))
	}
	return fmt.Sprintf("sudo -i -u %s sh -c %s", c.ampUser, shellQuote(inner))
}

// EvalOnGameBroker runs an Erlang expression via rabbitmqctl eval against the
// game broker. The RMQ server-commands publisher (rmq_commands.go) uses this to
// fetch broker-side data — e.g. the ServerCommandsAuthToken — that must be
// retrieved by an Erlang expression rather than a normal AMQP operation.
func (c *ampControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	cmd := c.buildRabbitmqctl("mq-game", "eval "+shellQuote(expr))
	out, err := exec.Exec(cmd + " 2>&1")
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// ── INI discovery ─────────────────────────────────────────────────────────────

func (c *ampControl) DiscoverIniDir(_ context.Context, exec Executor) (string, error) {
	if c.iniDir != "" {
		return c.iniDir, nil
	}
	if c.instance == "" {
		return "", fmt.Errorf("amp control requires server_ini_dir or amp_instance to derive an INI directory")
	}
	base := filepath.ToSlash(fmt.Sprintf(
		"/home/%s/.ampdata/instances/%s/duneawakening/server/state",
		c.ampUser, c.instance))

	// install.sh places UserGame.ini under ue5-saved/UserSettings/ inside the
	// state directory (UE5's canonical Saved path). Prefer it over the bare
	// state/ root so reads and writes target the file the game server actually uses.
	ue5Dir := base + "/ue5-saved/UserSettings"
	out, _ := exec.Exec(fmt.Sprintf(
		"test -f %s/UserGame.ini && echo yes || echo no",
		shellQuote(ue5Dir)))
	if strings.TrimSpace(out) == "yes" {
		return ue5Dir, nil
	}
	return base, nil
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
