package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

	// AMP Web API credentials — used to write server settings through AMP's own
	// config (Core/SetConfig) so they survive AMP regenerating the game INIs.
	apiUser string
	apiPass string
	apiPort int // 0 → defaultAmpAPIPort (8081)

	// Postgres client tooling inside the container, for #150 DB backups. The
	// game's PG17 ships a musl pg_dump under pgBin, but its libpq dir lacks the
	// compression/SSL libs — those live in the sibling db-utils tree, so pgLib is
	// a colon-joined path spanning both. Empty → validated AMP defaults.
	pgBin string // dir containing pg_dump/pg_restore
	pgLib string // LD_LIBRARY_PATH for the above
}

const (
	defaultAmpPgBin = "/AMP/duneawakening/extracted/postgres/usr/local/bin"
	defaultAmpPgLib = "/AMP/duneawakening/extracted/postgres/usr/local/lib:" +
		"/AMP/duneawakening/extracted/db-utils/usr/lib"
)

func (c *ampControl) pgBinDir() string {
	if c.pgBin != "" {
		return c.pgBin
	}
	return defaultAmpPgBin
}

func (c *ampControl) pgLibPath() string {
	if c.pgLib != "" {
		return c.pgLib
	}
	return defaultAmpPgLib
}

func (c *ampControl) Name() string { return "amp" }

// ── status & lifecycle ────────────────────────────────────────────────────────

var (
	ampPortRe = regexp.MustCompile(`-Port=(\d+)`)
	ampPartRe = regexp.MustCompile(`-PartitionIndex=(\d+)`)
)

func (c *ampControl) GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error) {
	procs, err := c.listGameProcesses(exec)
	if err != nil {
		return nil, err
	}
	// The host process args only carry -PartitionIndex, never a dimension. The
	// Battlegroup Director knows each partition's dimensionIndex and label, so
	// enrich rows from there. Best-effort: a missing/unreachable director just
	// leaves Dimension at zero.
	dirMeta, err := c.fetchDirectorPartitions(ctx, exec)
	if err != nil {
		componentLog("control_amp").Warn().Err(err).Msg("director enrichment unavailable")
	}
	pids := make([]int, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, p.pid)
	}
	ages := c.fetchProcessAges(exec, pids)
	servers := make([]ServerRow, 0, len(procs))
	for _, p := range procs {
		row := ServerRow{
			Map:        p.mapName,
			Partition:  p.partition,
			Phase:      "Running",
			Ready:      true,
			Players:    0,
			Port:       p.port,
			AgeSeconds: ages[p.pid],
		}
		if meta, ok := dirMeta[p.partition]; ok {
			row.Dimension = meta.dimension
			row.Players = meta.players
			row.PlayerHardCap = meta.playerHardCap
			row.Queue = meta.queue
			if meta.label != "" {
				row.Sietch = meta.label
			}
		}
		servers = append(servers, row)
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

// partitionMeta is director-sourced metadata for one game-server partition.
type partitionMeta struct {
	dimension     int
	label         string
	players       int
	playerHardCap int
	queue         int
}

// fetchDirectorPartitions queries the Battlegroup Director's /v0/battlegroup
// endpoint and returns a map of partitionId → metadata. It returns nil (no
// error) when no director URL is configured; transport, status, and decode
// failures are returned as errors so the caller can log them and continue.
func (c *ampControl) fetchDirectorPartitions(ctx context.Context, exec Executor) (map[int]partitionMeta, error) {
	if c.directorURL == "" {
		return nil, nil
	}
	endpoint := strings.TrimRight(c.directorURL, "/") + "/v0/battlegroup"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build director request: %w", err)
	}
	// Route through the executor so the director is reachable from wherever the
	// executor runs (e.g. the AMP box over SSH), not the dune-admin host. Status
	// polling must stay snappy, so a short timeout falls back fast.
	client := &http.Client{Timeout: 3 * time.Second, Transport: httpTransportVia(exec.Dial)}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query director: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("director returned status %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode director response: %w", err)
	}
	meta := map[int]partitionMeta{}
	collectPartitions(raw, meta)
	return meta, nil
}

// collectPartitions recursively walks a decoded director response, recording
// the dimensionIndex and label of every "partition" object it finds keyed by
// partitionId. This is structure-agnostic: it picks up single-server,
// dimension (serversByDimension), and instanced (instances) maps alike.
func collectPartitions(v any, out map[int]partitionMeta) {
	switch t := v.(type) {
	case map[string]any:
		if p, ok := t["partition"].(map[string]any); ok {
			if id, ok := jsonPartitionID(p["partitionId"]); ok {
				// Player count, queue, and caps are siblings of "partition" on
				// the server node.
				out[id] = partitionMeta{
					dimension:     jsonInt(p["dimensionIndex"]),
					label:         jsonString(p["label"]),
					players:       jsonInt(t["numPlayersInGame"]),
					playerHardCap: effectivePlayerHardCap(t),
					queue:         jsonInt(t["numPlayersInQueue"]),
				}
			}
		}
		for _, child := range t {
			collectPartitions(child, out)
		}
	case []any:
		for _, child := range t {
			collectPartitions(child, out)
		}
	}
}

// jsonPartitionID extracts a partition ID from a decoded JSON number, reporting
// whether the value was present and numeric (a partition ID may legitimately
// be 0, so absence must be distinguished from zero).
func jsonPartitionID(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// jsonInt coerces a decoded JSON number to int, returning 0 for non-numbers.
func jsonInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}

// jsonString coerces a decoded JSON value to string, returning "" otherwise.
func jsonString(v any) string {
	s, _ := v.(string)
	return s
}

// effectivePlayerHardCap resolves a server node's player cap: the per-server
// override (serverPlayerHardCap) wins when positive, otherwise the configured
// cap (cfg.playerHardCap). The director uses -1 for "no override".
func effectivePlayerHardCap(node map[string]any) int {
	if override := jsonInt(node["serverPlayerHardCap"]); override > 0 {
		return override
	}
	if cfg, ok := node["cfg"].(map[string]any); ok {
		return jsonInt(cfg["playerHardCap"])
	}
	return 0
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
		return c.restartGame(exec)
	default:
		return "", fmt.Errorf("amp control does not support %q", cmd)
	}
}

// restartGame cycles the game server so config changes (CVars / UPROPERTYs)
// actually take effect.
//
// In container mode it restarts the whole AMP container. This is deliberate:
// `ampinstmgr -q` does NOT reap the DuneSandboxServer processes — confirmed
// in-game, where the game kept 4d+ uptime through both dune-admin's old restart
// AND AMP's own Stop, so any setting needing a game restart never applied. A
// `<runtime> restart` is the proven action that actually recycles the game, and
// it preserves the container filesystem so AMP regenerates the game INIs from
// its config on the way back up. Blast radius: this briefly cycles the
// in-container Postgres and broker too — dune-admin reconnects to the DB after.
//
// In native mode (no container) the game runs as host processes ampinstmgr
// manages directly, so the stop/start cycle is retained.
func (c *ampControl) restartGame(exec Executor) (string, error) {
	if c.useContainer {
		if c.container == "" {
			return "", fmt.Errorf("amp control in container mode requires amp_container to be set")
		}
		return exec.Exec(fmt.Sprintf("sudo -i -u %s %s restart %s 2>&1",
			c.ampUser, c.runtimeCLI(), c.container))
	}
	return exec.Exec(fmt.Sprintf("sudo -i -u %s ampinstmgr -q %s 2>&1 && sudo -i -u %s ampinstmgr -s %s 2>&1",
		c.ampUser, c.instance, c.ampUser, c.instance))
}

// ── database backup/restore (#150) ──────────────────────────────────────────

// pgDumpCommand builds the host shell command that runs pg_dump (-Fc) inside the
// container, redirecting its stdout to a host file. The '>' redirect is handled
// by the outer host shell (run by the dune-admin service user), so the dump
// lands host-side and service-user-owned.
func (c *ampControl) pgDumpCommand(conn dbConn, destPath string) string {
	inner := fmt.Sprintf(
		"%s exec -e PGPASSWORD=%s -e LD_LIBRARY_PATH=%s %s %s -Fc -h %s -p %d -U %s -d %s",
		c.runtimeCLI(),
		shellQuote(conn.Pass),
		shellQuote(c.pgLibPath()),
		shellQuote(c.container),
		shellQuote(c.pgBinDir()+"/pg_dump"),
		shellQuote(conn.Host), conn.Port, shellQuote(conn.User), shellQuote(conn.Name),
	)
	return fmt.Sprintf("sudo -i -u %s %s > %s", c.ampUser, inner, shellQuote(destPath))
}

// pgRestoreCommand builds the host shell command that pipes a host dump file into
// pg_restore (--clean --if-exists) running inside the container. DESTRUCTIVE:
// the caller must ensure the game is stopped.
func (c *ampControl) pgRestoreCommand(conn dbConn, srcPath string) string {
	inner := fmt.Sprintf(
		"%s exec -i -e PGPASSWORD=%s -e LD_LIBRARY_PATH=%s %s %s --clean --if-exists --no-owner -h %s -p %d -U %s -d %s",
		c.runtimeCLI(),
		shellQuote(conn.Pass),
		shellQuote(c.pgLibPath()),
		shellQuote(c.container),
		shellQuote(c.pgBinDir()+"/pg_restore"),
		shellQuote(conn.Host), conn.Port, shellQuote(conn.User), shellQuote(conn.Name),
	)
	return fmt.Sprintf("sudo -i -u %s %s < %s", c.ampUser, inner, shellQuote(srcPath))
}

// BackupDatabase runs pg_dump in-container and writes the archive to destPath on
// the host. Implements dbBackupProvider.
func (c *ampControl) BackupDatabase(exec Executor, conn dbConn, destPath string) (string, error) {
	if !c.useContainer || c.container == "" {
		return "", fmt.Errorf("AMP database backup requires container mode (amp_container)")
	}
	out, err := exec.Exec(c.pgDumpCommand(conn, destPath))
	if err != nil {
		return out, fmt.Errorf("pg_dump: %w", err)
	}
	return out, nil
}

// RestoreDatabase pipes a host dump into pg_restore in-container. DESTRUCTIVE.
// Implements dbBackupProvider.
func (c *ampControl) RestoreDatabase(exec Executor, conn dbConn, srcPath string) (string, error) {
	if !c.useContainer || c.container == "" {
		return "", fmt.Errorf("AMP database restore requires container mode (amp_container)")
	}
	out, err := exec.Exec(c.pgRestoreCommand(conn, srcPath))
	if err != nil {
		return out, fmt.Errorf("pg_restore: %w", err)
	}
	return out, nil
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

// parseProcessAges parses the output of `ps -o pid=,etimes=` into a pid→elapsed
// seconds map. Each non-empty line has two whitespace-separated columns; lines
// that don't parse cleanly are skipped rather than failing the whole map.
func parseProcessAges(out string) map[int]int {
	ages := map[int]int{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		age, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		ages[pid] = age
	}
	return ages
}

// fetchProcessAges returns a best-effort pid→uptime-seconds map for the given
// pids. It is deliberately separate from listGameProcesses so a `ps` that lacks
// the etimes field (or any error) degrades to "no ages" rather than breaking the
// core process listing that the status table and lifecycle commands depend on.
func (c *ampControl) fetchProcessAges(exec Executor, pids []int) map[int]int {
	if len(pids) == 0 {
		return map[int]int{}
	}
	ids := make([]string, len(pids))
	for i, p := range pids {
		ids[i] = strconv.Itoa(p)
	}
	cmd := "ps -o pid=,etimes= -p " + strings.Join(ids, ",") + " 2>/dev/null"
	if c.useContainer {
		if c.container == "" {
			return map[int]int{}
		}
		cmd = c.wrapInContainer(cmd)
	}
	out, err := exec.Exec(cmd)
	if err != nil && strings.TrimSpace(out) == "" {
		return map[int]int{}
	}
	return parseProcessAges(out)
}

func (c *ampControl) listGameProcesses(exec Executor) ([]ampGameProcess, error) {
	cmd := `ps -eo pid,args --no-headers 2>/dev/null | grep 'DuneSandboxServer-Linux-Shipping' | grep -v grep`
	if c.useContainer {
		if c.container == "" {
			return nil, fmt.Errorf("amp_container not configured")
		}
		cmd = c.wrapInContainer(cmd)
	}
	out, err := exec.Exec(cmd)
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

// ── server settings (AMP Web API) ─────────────────────────────────────────────

// writeServerSettings applies fieldName→value updates through AMP's Web API
// (Core/SetConfig). AMP persists them to its own config (GenericModule.kvp →
// App.AppSettings) and regenerates UserEngine.ini / UserGame.ini with these
// values on the next start. This is the only durable write path under AMP: a
// direct INI edit is clobbered when AMP regenerates the files.
//
// Callers pass raw AMP FieldNames; the "Meta.GenericModule." node prefix is
// added here. The write is fail-fast — a SetConfig error aborts the batch and
// is returned naming the field, so partial application is possible on error.
func (c *ampControl) writeServerSettings(_ context.Context, exec Executor, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	if c.apiUser == "" || c.apiPass == "" {
		return fmt.Errorf("amp api credentials not configured — set amp_api_user and amp_api_pass to manage server settings under AMP")
	}
	client := newAMPAPIClient(exec, c.wrapInContainer, c.apiUser, c.apiPass, c.apiPort)
	for field, value := range updates {
		if err := client.setConfig("Meta.GenericModule."+field, value); err != nil {
			return fmt.Errorf("write server setting %s: %w", field, err)
		}
	}
	return nil
}

// readServerSettings reads the current value of each curated FieldName back from
// AMP's live config (Core/GetConfig on node "Meta.GenericModule.<FieldName>").
// AMP — not the INI files — is the source of truth for these settings, so this
// lets the read path reflect values saved through the AMP API immediately,
// without waiting for AMP to regenerate UserEngine.ini / UserGame.ini on the
// next game restart. Implements serverSettingsReader. The session is reused
// across fields (login happens once on the first GetConfig).
func (c *ampControl) readServerSettings(_ context.Context, exec Executor, fields []string) (map[string]string, error) {
	if len(fields) == 0 {
		return map[string]string{}, nil
	}
	if c.apiUser == "" || c.apiPass == "" {
		return nil, fmt.Errorf("amp api credentials not configured — set amp_api_user and amp_api_pass to read server settings under AMP")
	}
	client := newAMPAPIClient(exec, c.wrapInContainer, c.apiUser, c.apiPass, c.apiPort)
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		v, err := client.getConfig("Meta.GenericModule." + field)
		if err != nil {
			return nil, fmt.Errorf("read server setting %s: %w", field, err)
		}
		out[field] = v
	}
	return out, nil
}

// ── INI discovery ─────────────────────────────────────────────────────────────

// gameOverridePath returns the file AMP appends to UserGame.ini at boot:
// UserOverrides.ini in the instance state dir. AMP owns UserGame.ini (written
// from its dashboard), so dune-admin writes game-scoped settings here instead
// of clobbering it. Keys in UserOverrides.ini take precedence at runtime.
//
// dir is the discovered INI directory. In the standard container layout that is
// …/state/ue5-saved/UserSettings; UserOverrides.ini lives two levels up in
// …/state. If dir does not match that layout the override file is placed
// alongside it, so the method always returns a usable path.
func (c *ampControl) gameOverridePath(dir string) string {
	d := strings.TrimRight(filepath.ToSlash(dir), "/")
	d = strings.TrimSuffix(d, "/ue5-saved/UserSettings")
	return d + "/UserOverrides.ini"
}

// readINIFile reads a host INI file as the amp user so amp-owned UserGame.ini /
// UserEngine.ini / UserOverrides.ini (which the dune-admin user can't cat and
// the narrow AMP sudoers won't `sudo cat`) are readable on the Server Settings
// page (#173). Symmetric with how AMP writes them and reads director_config.ini.
// Implements iniFileReader.
func (c *ampControl) readINIFile(exec Executor, path string) (string, error) {
	out, err := exec.Exec(fmt.Sprintf("sudo -i -u %s cat %s 2>/dev/null", shellQuote(c.ampUser), shellQuote(path)))
	if err != nil {
		return "", fmt.Errorf("read ini %s as %s: %w", path, c.ampUser, err)
	}
	return out, nil
}

// defaultINIDir returns the host directory holding the game's stock
// DefaultGame.ini / DefaultEngine.ini so default discovery needs no
// configuration under AMP. The game ships them in the extracted game-server
// tree at <gameRoot>/extracted/game-server/home/dune/server/DuneSandbox/Config,
// where gameRoot is the instance's duneawakening dir. gameRoot is recovered
// from the discovered INI dir, then the configured server_ini_dir (both contain
// "…/server/state"), and finally the conventional ampdata path for the
// instance. Returns "" when none apply (e.g. native layout), letting the other
// discovery strategies take over.
func (c *ampControl) defaultINIDir(iniDir string) string {
	for _, base := range []string{iniDir, c.iniDir} {
		if i := strings.Index(base, "/server/state"); i > 0 {
			return base[:i] + ampDefaultsConfigSuffix
		}
	}
	if c.useContainer && c.instance != "" {
		user := c.ampUser
		if user == "" {
			user = "amp"
		}
		return fmt.Sprintf("/home/%s/.ampdata/instances/%s/duneawakening%s",
			user, c.instance, ampDefaultsConfigSuffix)
	}
	return ""
}

// ampDefaultsConfigSuffix is the path, relative to the instance's duneawakening
// gameRoot, to the directory containing the stock Default*.ini files.
const ampDefaultsConfigSuffix = "/extracted/game-server/home/dune/server/DuneSandbox/Config"

func (c *ampControl) DiscoverIniDir(_ context.Context, exec Executor) (string, error) {
	base := c.iniDir
	if base == "" {
		if c.instance == "" {
			return "", fmt.Errorf("amp control requires server_ini_dir or amp_instance to derive an INI directory")
		}
		base = filepath.ToSlash(fmt.Sprintf(
			"/home/%s/.ampdata/instances/%s/duneawakening/server/state",
			c.ampUser, c.instance))
	}

	// install.sh places UserGame.ini under ue5-saved/UserSettings/ inside the
	// state directory. Prefer that subdirectory over the base path — this probe
	// runs even when server_ini_dir is explicitly configured so the configured
	// path acts as a base directory rather than bypassing auto-detection.
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

// ── Battlegroup Director config (#147) ──────────────────────────────────────
// director_config.ini is a HOST file ($STATE/director_config.ini, amp-owned
// 0700) — NOT in the game container — so it's read/written on the host as the
// AMP user. prestart.sh copies it into runtime/director-conf.d on every start,
// so edits persist and apply on the next instance restart.

// directorConfigPath derives $STATE/director_config.ini from the resolved server
// INI dir, which is $STATE/ue5-saved/UserSettings (so $STATE is two levels up).
func (c *ampControl) directorConfigPath(exec Executor) (string, error) {
	dir, err := iniDir(c, exec)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(filepath.Dir(dir)), "director_config.ini"), nil
}

func (c *ampControl) readDirectorConfig(exec Executor) (string, string, error) {
	path, err := c.directorConfigPath(exec)
	if err != nil {
		return "", "", err
	}
	out, err := exec.Exec(fmt.Sprintf("sudo -i -u %s cat %s 2>/dev/null", shellQuote(c.ampUser), shellQuote(path)))
	if err != nil {
		return path, "", fmt.Errorf("read %s: %w", path, err)
	}
	if strings.TrimSpace(out) == "" {
		return path, "", fmt.Errorf("director config empty or unreadable at %s", path)
	}
	return path, out, nil
}

func (c *ampControl) writeDirectorConfig(exec Executor, content string) (string, error) {
	path, err := c.directorConfigPath(exec)
	if err != nil {
		return "", err
	}
	if err := exec.WriteFile(path, strings.NewReader(content)); err != nil {
		return path, fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
