package main

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
)

// dockerControl implements ControlPlane using the Docker CLI.
// It requires configured container names and expects the Docker socket to be
// accessible by the executor (locally or via SSH to a Docker host).
type dockerControl struct {
	gameserver  string // container name for the game server
	brokerGame  string // container name for mq-game broker
	brokerAdmin string // container name for mq-admin broker
	directorURL string // optional Battlegroup Director URL for per-server enrichment
	iniDir      string // optional server_ini_dir; used as a base for DiscoverIniDir
}

func (c *dockerControl) Name() string { return "docker" }

func (c *dockerControl) GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error) {
	if c.gameserver == "" {
		return nil, errNotSupported("docker", "GetStatus (docker_gameserver not configured)")
	}

	// Container lifecycle phase (running/exited/...). Best-effort: a failed
	// inspect should not blank out the server list discovered below.
	phase := "unknown"
	if out, err := exec.Exec(fmt.Sprintf(
		"docker inspect --format '{{.State.Status}}' %s 2>&1", c.gameserver)); err == nil {
		phase = strings.TrimSpace(out)
	}

	procs, err := c.listGameProcesses(exec)
	if err != nil {
		return nil, err
	}

	// The process args only carry -PartitionIndex, never a dimension. The
	// Battlegroup Director knows each partition's dimension, label, and player
	// counts, so enrich rows from there when configured. Best-effort: a missing
	// or unreachable director just leaves those fields at zero.
	dirMeta, derr := fetchDirectorPartitionsVia(ctx, exec, c.directorURL)
	if derr != nil {
		log.Printf("dockerControl.GetStatus: director enrichment unavailable: %v", derr)
	}

	servers := make([]ServerRow, 0, len(procs))
	for _, p := range procs {
		row := ServerRow{
			Map:       p.mapName,
			Partition: p.partition,
			Phase:     "Running",
			Ready:     true,
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
		Name:     c.gameserver,
		Title:    c.gameserver,
		Phase:    phase,
		Database: dbPhase,
		Servers:  servers,
	}, nil
}

// listGameProcesses discovers the DuneSandboxServer game processes running
// inside the gameserver container and parses each one's map and partition from
// its launch args. It reuses the AMP parser since the launch args are identical.
// Mirrors ampControl.listGameProcesses' error handling: an exec failure with no
// output (e.g. the container is stopped) yields an empty list, not an error.
func (c *dockerControl) listGameProcesses(exec Executor) ([]ampGameProcess, error) {
	if c.gameserver == "" {
		return nil, errNotSupported("docker", "listGameProcesses (docker_gameserver not configured)")
	}
	cmd := fmt.Sprintf(
		"docker exec %s ps -eo pid,args --no-headers 2>/dev/null | grep 'DuneSandboxServer-Linux-Shipping' | grep -v grep",
		c.gameserver)
	out, err := exec.Exec(cmd)
	if err != nil && strings.TrimSpace(out) == "" {
		return []ampGameProcess{}, nil
	}
	var procs []ampGameProcess
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if proc, ok := parseAMPGameProcess(line); ok {
			procs = append(procs, proc)
		}
	}
	return procs, nil
}

func (c *dockerControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	if c.gameserver == "" {
		return "", errNotSupported("docker", "ExecCommand (docker_gameserver not configured)")
	}
	var dockerCmd string
	switch cmd {
	case "start":
		dockerCmd = fmt.Sprintf("docker start %s 2>&1", c.gameserver)
	case "stop":
		dockerCmd = fmt.Sprintf("docker stop %s 2>&1", c.gameserver)
	case "restart":
		dockerCmd = fmt.Sprintf("docker restart %s 2>&1", c.gameserver)
	default:
		return "", fmt.Errorf("docker control does not support %q", cmd)
	}
	out, err := exec.Exec(dockerCmd)
	if err != nil {
		return out, fmt.Errorf("docker %s: %w — %s", cmd, err, out)
	}
	return out, nil
}

func (c *dockerControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	out, err := exec.Exec("docker ps --format '{{.Names}}\\t{{.Status}}' 2>&1")
	if err != nil {
		return nil, "", fmt.Errorf("docker ps: %w", err)
	}
	var procs []ProcessInfo
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		status := ""
		if len(parts) == 2 {
			status = parts[1]
		}
		procs = append(procs, ProcessInfo{Name: parts[0], Status: status})
	}
	return procs, "docker", nil
}

func (c *dockerControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	out, err := exec.Exec("docker ps --format '{{.Names}}' 2>&1")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var sources []LogSource
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name != "" {
			sources = append(sources, LogSource{Namespace: "docker", Name: name})
		}
	}
	return sources, nil
}

func (c *dockerControl) StreamLog(_ context.Context, exec Executor, _, name string) (<-chan string, func(), error) {
	return exec.Stream(fmt.Sprintf("docker logs -f %s 2>&1", name))
}

func (c *dockerControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	if c.gameserver == "" {
		return "", "", errNotSupported("docker", "CaptureJWT (docker_gameserver not configured)")
	}
	existingToken, err := exec.Exec(fmt.Sprintf(
		"docker exec %s env 2>/dev/null | grep FuncomLiveServices__ServiceAuthToken | cut -d= -f2-",
		c.gameserver))
	if err != nil || strings.TrimSpace(existingToken) == "" {
		return "", "", fmt.Errorf("read ServiceAuthToken from container: %w", err)
	}
	return buildCaptureJWT(strings.TrimSpace(existingToken))
}

func (c *dockerControl) EvalOnGameBroker(_ context.Context, exec Executor, expr string) (string, error) {
	if c.brokerGame == "" {
		return "", errNotSupported("docker", "EvalOnGameBroker (docker_broker_game not configured)")
	}
	out, err := exec.Exec(fmt.Sprintf(
		"docker exec %s rabbitmqctl eval %s 2>&1",
		c.brokerGame, shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

func (c *dockerControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	if c.gameserver == "" {
		return ""
	}
	pathOut, err := exec.Exec(fmt.Sprintf(
		"docker exec %s find / -name %s -not -path '*/Saved/*' -not -path '*/proc/*' -not -path '*/sys/*' -not -path '*/dev/*' 2>/dev/null | head -1",
		c.gameserver, shellQuote(filename)))
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(pathOut)
	if p == "" {
		return ""
	}
	content, err := exec.Exec(fmt.Sprintf("docker exec %s cat %s 2>/dev/null", c.gameserver, shellQuote(p)))
	if err != nil {
		return ""
	}
	return content
}

// DiscoverIniDir locates the directory containing UserGame.ini for a docker
// deployment. It is layout-agnostic: it probes the configured server_ini_dir (if
// set) and every host-side mount source of the gameserver container (from
// `docker inspect`, which covers bind mounts and named volumes alike — a named
// volume's source resolves to /var/lib/docker/volumes/<name>/_data on the host),
// returning the HOST path of the directory holding UserGame.ini so the executor's
// host-side reads and writes land inside the container. A UserSettings-scoped
// match is preferred. When nothing is found it returns the configured dir (if
// set) or an error so iniDir() falls back to config.
func (c *dockerControl) DiscoverIniDir(_ context.Context, exec Executor) (string, error) {
	var bases []string
	if c.iniDir != "" {
		bases = append(bases, c.iniDir)
	}
	bases = append(bases, c.containerMountSources(exec)...)

	for _, base := range bases {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			continue
		}
		if dir := findUserGameDir(exec, base); dir != "" {
			return dir, nil
		}
	}
	if c.iniDir != "" {
		return c.iniDir, nil // configured but probe inconclusive — trust config
	}
	return "", fmt.Errorf(
		"docker control could not locate UserGame.ini under container %q mounts; set server_ini_dir in config",
		c.gameserver)
}

// containerMountSources returns the host-side source paths of the gameserver
// container's mounts (bind mounts and named volumes). Returns nil when the
// gameserver is unconfigured or inspect yields nothing.
func (c *dockerControl) containerMountSources(exec Executor) []string {
	if c.gameserver == "" {
		return nil
	}
	out, err := exec.Exec(fmt.Sprintf(
		`docker inspect --format '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' %s 2>/dev/null`,
		c.gameserver))
	if err != nil && strings.TrimSpace(out) == "" {
		return nil
	}
	var srcs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			srcs = append(srcs, s)
		}
	}
	return srcs
}

// findUserGameDir returns the directory of the first UserGame.ini found under
// base (a remote Unix host path), preferring a UserSettings-scoped match over
// any stray copy. Returns "" when none is found. Uses path.Dir (forward-slash)
// since base is a remote Unix path regardless of where dune-admin is built.
func findUserGameDir(exec Executor, base string) string {
	for _, probe := range []string{
		fmt.Sprintf("find %s -maxdepth 8 -path '*/UserSettings/UserGame.ini' 2>/dev/null | head -1", shellQuote(base)),
		fmt.Sprintf("find %s -maxdepth 8 -name UserGame.ini 2>/dev/null | head -1", shellQuote(base)),
	} {
		out, _ := exec.Exec(probe)
		if f := strings.TrimSpace(out); f != "" {
			return path.Dir(f)
		}
	}
	return ""
}
