package main

import (
	"context"
	"fmt"
	"strings"
)

// dockerControl implements ControlPlane using the Docker CLI.
// It expects the Docker socket to be accessible by the executor (locally or via
// SSH to a Docker host).
//
// A Dune install runs ONE game-server container per map/partition (see #311) —
// e.g. dune-server-overmap, dune-server-deepdesert-1-8, dune-server-survival-1.
// Sibling containers on the same compose stack (gateway, text-router, brokers)
// are not game servers. The container set is therefore discovered per call
// rather than pinned to a single configured name.
type dockerControl struct {
	// gameserver is the legacy singular docker_gameserver key. It is honoured
	// for back-compat when gameservers is empty.
	gameserver  string
	gameservers []string // explicit docker_gameservers override
	brokerGame  string   // container name for mq-game broker
	brokerAdmin string   // container name for mq-admin broker
	directorURL string   // optional Battlegroup Director for partition metadata
}

// dockerGameContainer is one discovered game-server container.
type dockerGameContainer struct {
	name      string
	state     string // docker ps state: running / exited / created / …
	partition int    // -PartitionIndex= from the container args (0 if absent)
	port      int    // -Port= from the container args (0 if absent)
	mapName   string // container name minus the dune-server- prefix
	// known is false for a name in docker_gameservers that docker has no record
	// of. Such a row is still listed so the operator sees the stale entry, but
	// it has no args to parse and so no partition identity — it must stay out
	// of the collision math and out of partition-keyed restarts.
	known bool
}

const (
	// dockerGameImageBase is the image repository basename shared by every
	// game-server container. Matching on this rather than the container name is
	// what separates the real game servers from seabass-server-gateway and
	// seabass-server-text-router, which sit on the same stack.
	dockerGameImageBase = "seabass-server"
	// dockerGameNamePrefix is the container-name fallback used when no image
	// matches (retagged or privately-built images).
	dockerGameNamePrefix = "dune-server-"
)

func (c *dockerControl) Name() string { return "docker" }

// dockerPSEntry is one row of `docker ps` output.
type dockerPSEntry struct {
	name  string
	image string
	state string
}

// listDockerContainers returns every container on the host, running or not.
func listDockerContainers(exec Executor) ([]dockerPSEntry, error) {
	out, err := exec.Exec("docker ps -a --format '{{.Names}}\\t{{.Image}}\\t{{.State}}' 2>&1")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var entries []dockerPSEntry
	for _, line := range splitLines(out) {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		e := dockerPSEntry{name: parts[0]}
		if len(parts) > 1 {
			e.image = parts[1]
		}
		if len(parts) > 2 {
			e.state = parts[2]
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// imageRepoBase strips the digest, tag, and registry path from an image
// reference, leaving the bare repository name. A registry port (host:5000/img)
// must not be mistaken for a tag, so the tag is only cut after the last slash.
func imageRepoBase(image string) string {
	name := image
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	lastSlash := strings.LastIndex(name, "/")
	if colon := strings.LastIndex(name, ":"); colon > lastSlash {
		name = name[:colon]
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	return name
}

// selectGameContainers picks the game-server containers out of a full container
// listing: by image first, falling back to the name prefix.
func selectGameContainers(entries []dockerPSEntry) []dockerPSEntry {
	var byImage []dockerPSEntry
	for _, e := range entries {
		if imageRepoBase(e.image) == dockerGameImageBase {
			byImage = append(byImage, e)
		}
	}
	if len(byImage) > 0 {
		return byImage
	}
	var byName []dockerPSEntry
	for _, e := range entries {
		if strings.HasPrefix(e.name, dockerGameNamePrefix) && !isDockerSupportContainer(e.name) {
			byName = append(byName, e)
		}
	}
	return byName
}

// dockerSupportSuffixes name the containers that share the game servers' name
// prefix but are not game servers. The image match excludes them for free
// (their repos are seabass-server-gateway / -text-router); the prefix fallback
// has to do it explicitly or a retagged install picks up the gateway.
var dockerSupportSuffixes = []string{"gateway", "text-router", "router", "autoscaler", "director"}

func isDockerSupportContainer(name string) bool {
	for _, suffix := range dockerSupportSuffixes {
		if strings.HasSuffix(name, "-"+suffix) {
			return true
		}
	}
	return false
}

// configuredNames returns the explicitly configured container list, honouring
// the legacy singular key. Empty means "auto-detect".
func (c *dockerControl) configuredNames() []string {
	if len(c.gameservers) > 0 {
		return c.gameservers
	}
	if c.gameserver != "" {
		return []string{c.gameserver}
	}
	return nil
}

// discoverGameContainers resolves the game-server container set and decorates
// each with its partition index and port.
func (c *dockerControl) discoverGameContainers(exec Executor) ([]dockerGameContainer, error) {
	entries, err := listDockerContainers(exec)
	if err != nil {
		return nil, err
	}
	stateByName := make(map[string]string, len(entries))
	for _, e := range entries {
		stateByName[e.name] = e.state
	}

	var selected []dockerPSEntry
	if names := c.configuredNames(); len(names) > 0 {
		// Explicit config wins outright — a configured container that is not in
		// the listing is still reported (as not-running) rather than dropped.
		for _, n := range names {
			selected = append(selected, dockerPSEntry{name: n, state: stateByName[n]})
		}
	} else {
		selected = selectGameContainers(entries)
	}

	// Only inspect containers docker actually knows about. A stale name in
	// docker_gameservers makes `docker inspect` fail for the whole batch, which
	// would strip partition/port metadata from the valid containers too.
	known := make([]string, 0, len(selected))
	for _, e := range selected {
		if _, ok := stateByName[e.name]; ok {
			known = append(known, e.name)
		}
	}
	argsByName := inspectContainerArgs(exec, known)

	containers := make([]dockerGameContainer, 0, len(selected))
	for _, e := range selected {
		_, seen := stateByName[e.name]
		ct := dockerGameContainer{
			name:    e.name,
			state:   e.state,
			mapName: strings.TrimPrefix(e.name, dockerGameNamePrefix),
			known:   seen,
		}
		// The game server carries the same -PartitionIndex/-Port flags under
		// every control plane, so the AMP regexes and parser apply verbatim.
		// Best-effort: a container that exposes neither still yields a usable
		// row, which is what keeps the server table populated (#311).
		ct.partition = parseAMPArgInt(ampPartRe, argsByName[e.name])
		ct.port = parseAMPArgInt(ampPortRe, argsByName[e.name])
		containers = append(containers, ct)
	}
	return containers, nil
}

// inspectContainerArgs returns each container's process args keyed by name.
//
// This is deliberately ONE docker call for all containers: GetStatus is polled
// on a timer and, under an SSH executor, every Exec opens a fresh session — so a
// per-container inspect would cost N round trips per refresh.
//
// Args are best-effort: whatever docker managed to print is used even when the
// call reports an error, since docker inspect exits non-zero if any one name is
// unresolvable while still emitting the containers it did find.
func inspectContainerArgs(exec Executor, names []string) map[string]string {
	if len(names) == 0 {
		return map[string]string{}
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, shellQuote(n))
	}
	// #nosec G204 -- every container name is shell-quoted; names come from docker ps or operator config.
	out, _ := exec.Exec(fmt.Sprintf(
		"docker inspect --format '{{.Name}}\t{{json .Args}}' %s 2>/dev/null", strings.Join(quoted, " ")))
	argsByName := make(map[string]string, len(names))
	for _, line := range splitLines(out) {
		name, args, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found {
			continue
		}
		// docker renders .Name with a leading slash.
		argsByName[strings.TrimPrefix(name, "/")] = args
	}
	return argsByName
}

func (c *dockerControl) GetStatus(ctx context.Context, exec Executor) (*BattlegroupStatus, error) {
	containers, err := c.discoverGameContainers(exec)
	if err != nil {
		return nil, err
	}
	// Best-effort enrichment, exactly as ampControl does: a missing or
	// unreachable director leaves the player/dimension columns at zero rather
	// than failing the whole status call.
	dirMeta, err := fetchDirectorPartitions(ctx, exec, c.directorURL)
	if err != nil {
		componentLog("control_docker").Warn().Err(err).Msg("director enrichment unavailable")
	}

	// Director metadata is keyed by partition, so it can only be trusted where
	// the partition identifies exactly one container. Containers whose args
	// carry no -PartitionIndex all parse to 0, and enriching each of them from
	// dirMeta[0] would show every map the same players, dimension and label.
	unique := uniquePartitions(containers)

	servers := make([]ServerRow, 0, len(containers))
	running := 0
	for _, ct := range containers {
		if ct.state == "running" {
			running++
		}
		row := ServerRow{
			Map:       ct.mapName,
			Partition: ct.partition,
			Phase:     dockerPhaseLabel(ct.state),
			Ready:     ct.state == "running",
			Port:      ct.port,
		}
		if meta, ok := dirMeta[ct.partition]; ok && unique[ct.partition] {
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
		Name:     c.statusName(containers),
		Title:    "Docker Managed",
		Phase:    dockerAggregatePhase(len(containers), running),
		Database: dbPhase,
		Servers:  servers,
	}, nil
}

// statusName labels the battlegroup: the container name when there is exactly
// one, otherwise the plane name.
func (c *dockerControl) statusName(containers []dockerGameContainer) string {
	if len(containers) == 1 {
		return containers[0].name
	}
	return "docker"
}

func dockerPhaseLabel(state string) string {
	if state == "" {
		return "Unknown"
	}
	return strings.ToUpper(state[:1]) + state[1:]
}

// dockerAggregatePhase reports the battlegroup-level phase.
//
// The casing is load-bearing: handlers_servers_health.go compares
// `st.Phase == "Running"` exactly, and ampControl emits "Running". Returning
// docker's own lowercase state here made a healthy install report as down.
func dockerAggregatePhase(total, running int) string {
	switch {
	case total == 0:
		return "Unknown"
	case running == 0:
		return "Stopped"
	default:
		return "Running"
	}
}

// ExecCommand applies a lifecycle verb to EVERY game container. Acting on only
// one leaves the other partitions running (#311), so a failure on one container
// is recorded but does not abort the rest.
func (c *dockerControl) ExecCommand(_ context.Context, exec Executor, cmd string) (string, error) {
	switch cmd {
	case "start", "stop", "restart":
	default:
		return "", fmt.Errorf("docker control does not support %q", cmd)
	}
	containers, err := c.discoverGameContainers(exec)
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", errNotSupported("docker", "ExecCommand (no game server containers found)")
	}

	var out strings.Builder
	var failures []string
	for _, ct := range containers {
		// #nosec G204 -- container name is shell-quoted; it comes from docker ps or operator config.
		o, err := exec.Exec(fmt.Sprintf("docker %s %s 2>&1", cmd, shellQuote(ct.name)))
		fmt.Fprintf(&out, "%s: %s\n", ct.name, strings.TrimSpace(o))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", ct.name, err))
		}
	}
	if len(failures) > 0 {
		return out.String(), fmt.Errorf("docker %s failed for %s", cmd, strings.Join(failures, ", "))
	}
	return out.String(), nil
}

// RestartPartition cycles the single container serving a partition. Unlike AMP,
// where every partition shares one container, docker runs one container each —
// so a narrower restart unit genuinely exists here.
func (c *dockerControl) RestartPartition(_ context.Context, exec Executor, target restartTarget) (string, error) {
	containers, err := c.discoverGameContainers(exec)
	if err != nil {
		return "", err
	}
	ct, err := resolveRestartContainer(containers, target)
	if err != nil {
		return "", err
	}
	// #nosec G204 -- container name is shell-quoted; it comes from docker ps or operator config.
	out, err := exec.Exec(fmt.Sprintf("docker restart %s 2>&1", shellQuote(ct.name)))
	if err != nil {
		return out, fmt.Errorf("docker restart %s: %w — %s", ct.name, err, strings.TrimSpace(out))
	}
	return out, nil
}

// uniquePartitions reports which partition indices identify exactly one
// container. Anything claimed by two or more is not a usable key — see
// restartTarget for why the index can collapse to 0.
func uniquePartitions(containers []dockerGameContainer) map[int]bool {
	counts := make(map[int]int, len(containers))
	for _, ct := range containers {
		if !ct.known {
			continue
		}
		counts[ct.partition]++
	}
	unique := make(map[int]bool, len(counts))
	for partition, n := range counts {
		unique[partition] = n == 1
	}
	return unique
}

// resolveRestartContainer picks the one container a restart target names.
//
// The map is tried first because it is the row's own identity. Falling back to
// the partition keeps working for installs whose args carry -PartitionIndex,
// but when several containers report the same index the request is refused
// rather than guessed: restarting an arbitrary map is worse than telling the
// operator which rows collide.
func resolveRestartContainer(containers []dockerGameContainer, target restartTarget) (dockerGameContainer, error) {
	if target.Map != "" {
		for _, ct := range containers {
			if ct.mapName == target.Map || ct.name == target.Map {
				return ct, nil
			}
		}
		return dockerGameContainer{}, fmt.Errorf("docker control: no container found for map %q: %w", target.Map, errRestartTargetUnknown)
	}

	var matches []dockerGameContainer
	for _, ct := range containers {
		// A container docker has no record of would fail `docker restart`
		// anyway, and choosing it over a real one is strictly worse.
		if ct.known && ct.partition == target.Partition {
			matches = append(matches, ct)
		}
	}
	switch len(matches) {
	case 0:
		return dockerGameContainer{}, fmt.Errorf("docker control: no container found for partition %d: %w", target.Partition, errRestartTargetUnknown)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, ct := range matches {
			names = append(names, ct.name)
		}
		return dockerGameContainer{}, fmt.Errorf(
			"docker control: %d containers report partition %d (%s); "+
				"these containers expose no -PartitionIndex, so the map must be supplied: %w",
			len(matches), target.Partition, strings.Join(names, ", "), errRestartTargetAmbiguous)
	}
}

// firstContainer returns a container to read install-wide values from. The
// ServiceAuthToken and the packaged default INIs are identical across
// partitions, so any game container will do.
func (c *dockerControl) firstContainer(exec Executor) (string, error) {
	containers, err := c.discoverGameContainers(exec)
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", errNotSupported("docker", "no game server containers found")
	}
	// Discovery lists with `docker ps -a`, so the first entry can be a stopped
	// container while another map is up. Both callers docker exec into the
	// result, which fails against a stopped container for no good reason.
	for _, ct := range containers {
		if ct.state == "running" {
			return ct.name, nil
		}
	}
	// Nothing running: return the first anyway and let docker's own error
	// surface, rather than inventing a different one here.
	return containers[0].name, nil
}

func (c *dockerControl) ListProcesses(_ context.Context, exec Executor) ([]ProcessInfo, string, error) {
	entries, err := listDockerContainers(exec)
	if err != nil {
		return nil, "", err
	}
	var procs []ProcessInfo
	for _, e := range entries {
		procs = append(procs, ProcessInfo{Name: e.name, Status: e.state})
	}
	return procs, "docker", nil
}

func (c *dockerControl) ListLogSources(_ context.Context, exec Executor) ([]LogSource, error) {
	entries, err := listDockerContainers(exec)
	if err != nil {
		return nil, err
	}
	var sources []LogSource
	for _, e := range entries {
		sources = append(sources, LogSource{Namespace: "docker", Name: e.name})
	}
	return sources, nil
}

func (c *dockerControl) StreamLog(_ context.Context, exec Executor, _, name string) (<-chan string, func(), error) {
	return exec.Stream(fmt.Sprintf("docker logs -f %s 2>&1", shellQuote(name)))
}

func (c *dockerControl) CaptureJWT(_ context.Context, exec Executor) (string, string, error) {
	container, err := c.firstContainer(exec)
	if err != nil {
		return "", "", err
	}
	// #nosec G204 -- container name is shell-quoted; it comes from docker ps or operator config.
	existingToken, err := exec.Exec(fmt.Sprintf(
		"docker exec %s env 2>/dev/null | grep FuncomLiveServices__ServiceAuthToken | cut -d= -f2-",
		shellQuote(container)))
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
		shellQuote(c.brokerGame), shellQuote(expr)))
	if err != nil {
		return "", fmt.Errorf("rabbitmqctl eval: %w (output: %s)", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

func (c *dockerControl) ReadDefaultINI(_ context.Context, exec Executor, filename string) string {
	container, err := c.firstContainer(exec)
	if err != nil {
		return ""
	}
	// #nosec G204 -- container name and filename are shell-quoted.
	pathOut, err := exec.Exec(fmt.Sprintf(
		"docker exec %s find / -name %s -not -path '*/Saved/*' -not -path '*/proc/*' -not -path '*/sys/*' -not -path '*/dev/*' 2>/dev/null | head -1",
		shellQuote(container), shellQuote(filename)))
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(pathOut)
	if p == "" {
		return ""
	}
	// #nosec G204 -- container name and path are shell-quoted.
	content, err := exec.Exec(fmt.Sprintf("docker exec %s cat %s 2>/dev/null", shellQuote(container), shellQuote(p)))
	if err != nil {
		return ""
	}
	return content
}

func (c *dockerControl) DiscoverIniDir(_ context.Context, _ Executor) (string, error) {
	return "", fmt.Errorf("docker control plane requires server_ini_dir to be set in config")
}
