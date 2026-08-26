package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// exitSetup prints a pause prompt on Windows, then exits with the given code.
func exitSetup(code int) {
	if runtime.GOOS == "windows" {
		fmt.Println()
		fmt.Println("Press Enter to close...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code)
}

func runSetup() {
	r := bufio.NewReader(os.Stdin)

	ask := func(label, def string) string {
		if def != "" {
			fmt.Printf("  %s [%s]: ", label, def)
		} else {
			fmt.Printf("  %s: ", label)
		}
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}

	ok := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	fail := func(msg string) { fmt.Printf("  ✗ %s\n", msg) }

	fmt.Println()
	fmt.Println("=== dune-admin setup ===")
	fmt.Println()

	// ── 1. Control plane ───────────────────────────────────────────────────────

	fmt.Println("Control plane:")
	fmt.Println("  kubectl — Kubernetes (k3s) via SSH (current default)")
	fmt.Println("  docker  — Docker containers (docker CLI with named containers)")
	fmt.Println("  local   — Generic local (configurable shell commands)")
	fmt.Println("  amp     — CubeCoders AMP with podman game-server container")
	fmt.Println()
	ctrl := ask("Control plane", "kubectl")
	if ctrl != "kubectl" && ctrl != "docker" && ctrl != "local" && ctrl != "amp" {
		ctrl = "kubectl"
	}
	fmt.Println()

	var cfg appConfig
	cfg.Control = ctrl

	switch ctrl {
	case "kubectl":
		runKubectlSetup(ask, ok, fail, &cfg)
	case "docker":
		runDockerSetup(ask, ok, fail, &cfg)
	case "local":
		runLocalSetup(ask, ok, fail, &cfg)
	case "amp":
		runAmpSetup(ask, ok, fail, &cfg)
	}

	// ── Embedded market bot ────────────────────────────────────────────────────

	runMarketBotSetup(ask, ok, &cfg)

	// ── Common: listen address ─────────────────────────────────────────────────

	fmt.Println("Server config:")
	defaultListenAddr := listenAddr
	if ctrl == "amp" && (defaultListenAddr == "" || defaultListenAddr == ":8080") {
		// AMP web panel commonly binds :8080 on host installs.
		defaultListenAddr = ":18080"
	}
	cfg.ListenAddr = normalizeListenAddr(ask("HTTP listen address", defaultListenAddr))
	fmt.Println()

	// ── Discord bot ────────────────────────────────────────────────────────────

	setupDiscordBot(ok, ask, &cfg)

	// ── Live events engine ─────────────────────────────────────────────────────

	setupEventsEngine(ok, ask, &cfg)

	// ── Write config ───────────────────────────────────────────────────────────

	writeSetupConfig(ok, fail, cfg)

	fmt.Println("Setup complete.")
	fmt.Println()
	fmt.Println("  Run: dune-admin")
	fmt.Println()
}

// ── kubectl setup flow ────────────────────────────────────────────────────────

// sshDefaultHost returns the best default SSH host for the setup wizard.
// loadedConfig takes priority so existing config.yaml values are honoured,
// then the SSH_HOST env var, then a hardcoded fallback.
func sshDefaultHost(cfg appConfig) string {
	if cfg.SSHHost != "" {
		return cfg.SSHHost
	}
	return envOr("SSH_HOST", "192.168.0.72:22")
}

// sshDefaultUser returns the best default SSH user for the setup wizard,
// preferring loadedConfig then SSH_USER env var then "dune".
func sshDefaultUser(cfg appConfig) string {
	if cfg.SSHUser != "" {
		return cfg.SSHUser
	}
	return envOr("SSH_USER", "dune")
}

// sshKeyNotFoundMessage returns an informative error message. When the key
// path came from config/flags, it shows that path so the user knows what was
// actually checked; otherwise it lists the auto-detection candidates.
func sshKeyNotFoundMessage(configuredKeyPath string) string {
	if configuredKeyPath != "" {
		return "SSH key not found at configured path: " + configuredKeyPath
	}
	return "SSH key not found (checked ~/.dune-admin/sshKey, next to binary, ./sshKey)"
}

func setupKubectlSSHKey(ask func(string, string) string, ok, fail func(string)) string {
	// SSH key
	fmt.Println("Checking for SSH key...")
	keyPath := resolveKeyPath()
	if _, err := os.Stat(keyPath); err != nil {
		fail(sshKeyNotFoundMessage(sshKeyPath))
		fmt.Println()
		sshKeyPath = ask("Path to SSH private key", "")
		if sshKeyPath == "" {
			fmt.Fprintln(os.Stderr, "SSH key is required. Aborting.")
			exitSetup(1)
		}
		if _, err := os.Stat(sshKeyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Key not found at %s. Aborting.\n", sshKeyPath)
			exitSetup(1)
		}
		keyPath = sshKeyPath
	} else {
		ok("SSH key: " + keyPath)
		sshKeyPath = keyPath
	}
	fmt.Println()
	return keyPath
}

func setupKubectlSSHConnection(
	ask func(string, string) string,
	ok, fail func(string),
	keyPath string,
) *sshExecutor {
	// SSH connection details
	fmt.Println("SSH connection:")
	sshHost = ask("VM host:port", sshDefaultHost(loadedConfig))
	sshUser = ask("SSH user", sshDefaultUser(loadedConfig))
	fmt.Println()

	// Dial
	fmt.Printf("Connecting via SSH to %s...\n", sshHost)
	client, err := dialSSH(sshHost, sshUser, keyPath)
	if err != nil {
		fail("SSH failed: " + err.Error())
		fmt.Println()
		fmt.Printf("  Attempted:  user=%s  host=%s  key=%s\n", sshUser, sshHost, keyPath)
		fmt.Println()
		fmt.Println("  Make sure:")
		fmt.Println("    - The VM is reachable at the given host:port")
		fmt.Println("    - The correct SSH private key is specified")
		fmt.Println("    - That key's public key is in ~/.ssh/authorized_keys on the VM")
		fmt.Println("    - The SSH user matches the account on the VM (default: dune)")
		fmt.Println("    - The SSH user has passwordless sudo for kubectl")
		exitSetup(1)
	}
	ok("SSH connected")
	fmt.Println()
	globalSSH = client
	return &sshExecutor{client: client}
}

func setupKubectlDiscoverDBPod(exec *sshExecutor, ok, fail func(string), cfg *appConfig) {
	// Discover DB pod
	fmt.Println("Discovering database pod...")
	ns, pod, podIP, err := discoverDBPod(exec)
	if err != nil {
		fail("Pod discovery failed: " + err.Error())
		fmt.Println()
		fmt.Println("  Make sure the SSH user can run: sudo kubectl get pods -A")
		exitSetup(1)
	}
	globalPodNS = ns
	globalPod = pod
	globalPodIP = podIP
	cfg.ControlNamespace = ns
	controlNS = ns
	ok("Database pod: " + pod)
	fmt.Println()
}

func selectBattlegroup(battlegroups []string, ask func(string, string) string) string {
	if len(battlegroups) == 0 {
		return ""
	}
	chosen := battlegroups[0]
	if len(battlegroups) == 1 {
		return chosen
	}

	fmt.Println("  Available battlegroups:")
	for i, bg := range battlegroups {
		fmt.Printf("    [%d] %s\n", i+1, bg)
	}
	fmt.Println()
	idxStr := ask(fmt.Sprintf("Which battlegroup? [1-%d]", len(battlegroups)), "1")
	idx := 1
	_, _ = fmt.Sscanf(idxStr, "%d", &idx)
	if idx >= 1 && idx <= len(battlegroups) {
		chosen = battlegroups[idx-1]
	}
	return chosen
}

func setupKubectlDiscoverDBCredentials(
	exec *sshExecutor,
	ask func(string, string) string,
	ok, fail func(string),
	cfg *appConfig,
) (string, string) {
	// Discover DB password
	fmt.Println("Discovering database password...")
	discoveredUser := "postgres"
	discoveredPass := ""

	var battlegroups []string
	if bg := battlegroupFromPod(globalPod); bg != "" {
		battlegroups = []string{bg}
	} else {
		battlegroups = listBattlegroups(exec)
	}

	if len(battlegroups) == 0 {
		fmt.Println("  Could not determine battlegroup name")
	} else {
		chosen := selectBattlegroup(battlegroups, ask)
		yamlPath := fmt.Sprintf("~/.dune/%s.yaml", chosen)
		if u, pass := extractPasswordFromYAML(exec, yamlPath); pass != "" {
			discoveredUser = u
			discoveredPass = pass
			ok(fmt.Sprintf("Password found in %s (user: %s)", yamlPath, u))
		} else {
			fail("No password found in " + yamlPath)
		}
	}

	if discoveredPass == "" {
		fmt.Println()
		fmt.Println("  Could not auto-discover the database password.")
		discoveredUser = ask("Database user", "postgres")
		discoveredPass = ask("Database password", "")
		if discoveredPass == "" {
			fmt.Fprintln(os.Stderr, "Database password is required. Aborting.")
			exitSetup(1)
		}
	}
	fmt.Println()
	return discoveredUser, discoveredPass
}

func setupKubectlConnectDB(
	discoveredUser, discoveredPass string,
	exec *sshExecutor,
	cfg *appConfig,
	ok, fail func(string),
) {
	// Connect to DB
	fmt.Println("Connecting to database...")
	dbUser = discoveredUser
	dbPass = discoveredPass
	// connectDB (kubectl path) tunnels through globalExecutor and aborts if it is
	// nil — so it MUST be set before the connect, not after. Otherwise SSH + pod
	// discovery succeed but the DB connect fails with "globalExecutor is nil".
	globalExecutor = exec
	pool, err := connectDB(context.Background(), discoveredUser, discoveredPass)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		fmt.Println()
		fmt.Printf("  The password may be wrong. Delete %s and re-run to try again.\n", configPath())
		exitSetup(1)
	}
	globalDB = pool
	globalControl = newControlPlane("kubectl", *cfg)
	ok("Database connected as: " + dbUser)
	fmt.Println()
}

func runKubectlSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	keyPath := setupKubectlSSHKey(ask, ok, fail)
	exec := setupKubectlSSHConnection(ask, ok, fail, keyPath)
	setupKubectlDiscoverDBPod(exec, ok, fail, cfg)
	discoveredUser, discoveredPass := setupKubectlDiscoverDBCredentials(exec, ask, ok, fail, cfg)
	setupKubectlConnectDB(discoveredUser, discoveredPass, exec, cfg, ok, fail)

	cfg.ControlNamespace = globalPodNS
	controlNS = globalPodNS
	if abs, err := filepath.Abs(keyPath); err == nil {
		keyPath = abs
	}
	cfg.SSHHost = sshHost
	cfg.SSHUser = sshUser
	cfg.SSHKey = keyPath
	cfg.DBHost = "127.0.0.1"
	cfg.DBPort = dbPort
	cfg.DBUser = dbUser
	cfg.DBPass = dbPass
	cfg.DBName = dbName
	cfg.DBSchema = dbSchema
	cfg.ScripCurrency = scripCurrencyID
}

// ── docker setup flow ─────────────────────────────────────────────────────────

// parseIndexSelection maps a comma-separated 1-based index list onto container
// names. Out-of-range entries, junk, and duplicates are dropped rather than
// aborting setup.
func parseIndexSelection(input string, entries []dockerPSEntry) []string {
	var names []string
	seen := map[string]bool{}
	for part := range strings.SplitSeq(input, ",") {
		i, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || i < 1 || i > len(entries) {
			continue
		}
		if name := entries[i-1].name; !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// dockerGameserverChoice is the outcome of the wizard's game-server prompt.
//
// Pinning is deliberately opt-in. The wizard used to write the auto-detected
// names straight into docker_gameservers, which froze the layout at setup time:
// an explicit list short-circuits runtime discovery, so a map server started
// afterwards never appeared on the panel (#311). Leaving the list empty keeps
// image-based discovery in charge on every poll.
type dockerGameserverChoice struct {
	pinned   []string // written to docker_gameservers; empty means auto-detect
	detected []string // what discovery finds today, for the confirmation line
}

// autoDetect reports whether discovery stays in charge at runtime.
func (c dockerGameserverChoice) autoDetect() bool { return len(c.pinned) == 0 }

// usable reports whether the panel will find game servers. Nothing pinned and
// nothing detectable means status and restart have nothing to act on.
func (c dockerGameserverChoice) usable() bool {
	return len(c.pinned) > 0 || len(c.detected) > 0
}

// resolveDockerGameservers turns the operator's answer into that choice. Junk
// and out-of-range indices resolve to auto-detect rather than to an empty pin,
// so a typo can't leave the panel permanently blank.
func resolveDockerGameservers(answer string, entries []dockerPSEntry) dockerGameserverChoice {
	var detected []string
	for _, e := range selectGameContainers(entries) {
		detected = append(detected, e.name)
	}
	return dockerGameserverChoice{
		pinned:   parseIndexSelection(answer, entries),
		detected: detected,
	}
}

// detectBrokerContainers picks the RabbitMQ game/admin brokers out of a listing.
func detectBrokerContainers(entries []dockerPSEntry) (game, admin string) {
	for _, e := range entries {
		if !strings.Contains(imageRepoBase(e.image), "rabbitmq") {
			continue
		}
		switch {
		case strings.Contains(e.name, "game") && game == "":
			game = e.name
		case strings.Contains(e.name, "admin") && admin == "":
			admin = e.name
		}
	}
	return game, admin
}

// detectDirectorContainer finds the Battlegroup Director container, whose API
// supplies per-partition player counts and dimension labels.
func detectDirectorContainer(entries []dockerPSEntry) string {
	for _, e := range entries {
		if strings.Contains(imageRepoBase(e.image), "director") {
			return e.name
		}
	}
	return ""
}

// askGameservers offers the detected game servers and lets the operator pin a
// different set. Accepting the default pins nothing, so discovery re-runs on
// every poll and map servers started after setup show up on their own (#311).
func askGameservers(ask func(string, string) string, ok, fail func(string), cfg *appConfig, entries []dockerPSEntry) {
	detected := resolveDockerGameservers("", entries).detected
	if len(detected) == 0 {
		fmt.Println("  Could not auto-detect the game-server containers.")
	} else {
		fmt.Println("  Auto-detected game servers: " + strings.Join(detected, ", "))
		fmt.Println("  Leave blank to re-detect on every poll — new map servers appear automatically.")
	}
	// Default is blank: pinning is the exception, not the norm.
	choice := resolveDockerGameservers(
		ask("Pin specific game-server containers? (comma-separated numbers, blank = auto-detect)", ""),
		entries)
	cfg.DockerGameservers = choice.pinned

	switch {
	case !choice.usable():
		fail("No game-server containers detected or pinned — status and restart will not work")
	case choice.autoDetect():
		ok(fmt.Sprintf("Game servers: auto-detect (%s)", strings.Join(choice.detected, ", ")))
	default:
		ok(fmt.Sprintf("Game servers pinned: %s", strings.Join(choice.pinned, ", ")))
	}
}

// setupDockerContainers discovers the container layout and lets the operator
// confirm which containers are game servers. A Dune install runs one game-server
// container per map/partition, so asking for a single name (as this wizard used
// to) leaves the install half-working (#311).
func setupDockerContainers(ask func(string, string) string, ok, fail func(string), cfg *appConfig, exec Executor) {
	entries, err := listDockerContainers(exec)
	if err != nil || len(entries) == 0 {
		fail("Could not list Docker containers")
		fmt.Println("  Make sure Docker is running and this user can run `docker ps`.")
		fmt.Println("  Falling back to manual entry.")
		cfg.DockerGameservers = splitContainerList(
			ask("Game server container names (comma-separated)", "dune-gameserver"))
		cfg.DockerBrokerGame = ask("mq-game broker container name (optional)", "")
		cfg.DockerBrokerAdmin = ask("mq-admin broker container name (optional)", "")
		return
	}

	fmt.Println("Containers found:")
	for i, e := range entries {
		fmt.Printf("    [%d] %-32s %s\n", i+1, e.name, e.state)
	}
	fmt.Println()

	askGameservers(ask, ok, fail, cfg, entries)

	brokerGame, brokerAdmin := detectBrokerContainers(entries)
	cfg.DockerBrokerGame = ask("mq-game broker container name (optional)", brokerGame)
	cfg.DockerBrokerAdmin = ask("mq-admin broker container name (optional)", brokerAdmin)

	// The Director supplies player counts, queue depth, and dimension labels for
	// each partition — the columns that stay blank without it.
	directorDefault := ""
	if detectDirectorContainer(entries) != "" {
		directorDefault = "http://127.0.0.1:11717"
	}
	cfg.DirectorURL = ask("Battlegroup Director URL (optional)", directorDefault)
}

// runDockerSetupDBPrompts collects the DB connection fields for a docker
// install. DBHost keeps its existing default: "database" fits a dune-admin
// instance running inside the same compose network, and an operator on the
// host (dune-admin's normal placement, per SETUP_DOCKER.md) overrides it with
// an IP either way, same as @wofnull did in #325.
//
// DBPass defaults to "dune" — dune-docker itself hardens Postgres to
// 127.0.0.1-only and issues that one base password, confirmed stable since
// the project's start (#325). It stays a confirmable default like the
// others: ask() shows it in brackets and only applies it on an empty answer.
func runDockerSetupDBPrompts(ask func(string, string) string, cfg *appConfig) {
	cfg.DBHost = ask("DB host (Docker DNS or IP)", "database")
	cfg.DBPort = dbPort
	// The label is plain — ask() already renders the default in brackets, and
	// a "[%d]" baked into the label too used to print "DB port [15432] [15432]:".
	portStr := ask("DB port", fmt.Sprintf("%d", dbPort))
	_, _ = fmt.Sscanf(portStr, "%d", &cfg.DBPort)
	cfg.DBUser = ask("DB user", envOr("DB_USER", "dune"))
	cfg.DBPass = ask("DB password", "dune")
	cfg.DBName = ask("DB name", envOr("DB_NAME", "dune"))
	cfg.DBSchema = ask("DB schema", envOr("DB_SCHEMA", "dune"))
}

func runDockerSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	exec := &localExecutor{}
	setupDockerContainers(ask, ok, fail, cfg, exec)
	fmt.Println()

	fmt.Println("Database connection:")
	runDockerSetupDBPrompts(ask, cfg)
	if cfg.DBPass == "" {
		fmt.Fprintln(os.Stderr, "Database password is required. Aborting.")
		exitSetup(1)
	}
	fmt.Println()

	fmt.Println("Connecting to database...")
	dbHost = cfg.DBHost
	dbPort = cfg.DBPort
	dbUser = cfg.DBUser
	dbPass = cfg.DBPass
	dbName = cfg.DBName
	dbSchema = cfg.DBSchema
	globalExecutor = exec
	pool, err := connectDBDirect(context.Background(), *cfg)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		exitSetup(1)
	}
	globalDB = pool
	globalControl = newControlPlane("docker", *cfg)
	ok("Database connected as: " + cfg.DBUser)
	fmt.Println()

	cfg.ScripCurrency = scripCurrencyID
}

// ── local setup flow ──────────────────────────────────────────────────────────

func runLocalSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	fmt.Println("Database connection:")
	cfg.DBHost = ask("DB host", "127.0.0.1")
	cfg.DBPort = dbPort
	portStr := ask(fmt.Sprintf("DB port [%d]", dbPort), fmt.Sprintf("%d", dbPort))
	_, _ = fmt.Sscanf(portStr, "%d", &cfg.DBPort)
	cfg.DBUser = ask("DB user", envOr("DB_USER", "dune"))
	cfg.DBPass = ask("DB password", "")
	if cfg.DBPass == "" {
		fmt.Fprintln(os.Stderr, "Database password is required. Aborting.")
		exitSetup(1)
	}
	cfg.DBName = ask("DB name", envOr("DB_NAME", "dune"))
	cfg.DBSchema = ask("DB schema", envOr("DB_SCHEMA", "dune"))
	fmt.Println()

	fmt.Println("Server control commands (optional — leave blank to skip):")
	cfg.CmdStart = ask("Start command (e.g. 'amp start dune')", "")
	cfg.CmdStop = ask("Stop command", "")
	cfg.CmdRestart = ask("Restart command", "")
	cfg.CmdStatus = ask("Status command", "")
	fmt.Println()

	fmt.Println("Connecting to database...")
	dbHost = cfg.DBHost
	dbPort = cfg.DBPort
	dbUser = cfg.DBUser
	dbPass = cfg.DBPass
	dbName = cfg.DBName
	dbSchema = cfg.DBSchema
	exec := &localExecutor{}
	globalExecutor = exec
	pool, err := connectDBDirect(context.Background(), *cfg)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		exitSetup(1)
	}
	globalDB = pool
	globalControl = newControlPlane("local", *cfg)
	ok("Database connected as: " + cfg.DBUser)
	fmt.Println()

	cfg.ScripCurrency = scripCurrencyID
}

// ── amp setup flow ────────────────────────────────────────────────────────────

type ampSetupDefaults struct {
	instance string
	user     string
	topology string
	selected *ampInstance
}

func detectAmpSetupDefaults(ask func(string, string) string) ampSetupDefaults {
	d := ampSetupDefaults{instance: "DuneAwakening01", user: "amp", topology: "container"}
	instances, detectedUser, _ := detectAmpInstances()
	if len(instances) == 0 {
		return d
	}
	fmt.Printf("Detected %d AMP instance(s):\n", len(instances))
	for i, inst := range instances {
		fmt.Printf("  %d) %s\n", i+1, summarizeInstance(inst))
	}
	choice := 1
	if len(instances) > 1 {
		pickStr := ask("Pick instance [1]", "1")
		if n, err := strconv.Atoi(strings.TrimSpace(pickStr)); err == nil && n >= 1 && n <= len(instances) {
			choice = n
		}
	}
	d.selected = &instances[choice-1]
	d.instance = d.selected.Name
	if d.selected.InContainer {
		d.topology = "container"
	} else {
		d.topology = "native"
	}
	if detectedUser != "" {
		d.user = detectedUser
	}
	fmt.Println()
	return d
}

// runAmpSetup configures the AMP control plane (CubeCoders AMP). AMP supports
// two deployment topologies — game server in a podman container, or running
// natively on the host as the AMP user. All paths/names are configurable.
//
// The wizard auto-detects AMP instances via `ampinstmgr -l` when available
// and pre-fills prompts with discovered values. Falls back to historical
// hardcoded defaults (DuneAwakening01, /AMP/duneawakening/...) when
// detection isn't possible — every prompt is still overridable.
func runAmpSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	d := detectAmpSetupDefaults(ask)
	defaultInstance := d.instance
	defaultUser := d.user
	defaultTopology := d.topology
	selected := d.selected

	fmt.Println("AMP instance:")
	cfg.AmpInstance = ask("Instance name (ampinstmgr instance)", defaultInstance)
	cfg.AmpUser = ask("OS user that runs AMP", defaultUser)
	fmt.Println()

	fmt.Println("AMP topology:")
	fmt.Println("  container — game server runs inside the AMP container (podman or docker)")
	fmt.Println("  native    — game server runs directly on the host as the AMP user")
	topology := ask("Topology [container/native]", defaultTopology)
	useContainer := topology != "native"
	cfg.AmpUseContainer = &useContainer

	if useContainer {
		defaultContainer := "AMP_" + cfg.AmpInstance
		cfg.AmpContainer = ask("Container name", defaultContainer)
		cfg.AmpContainerRuntime = ask("Container runtime [podman/docker]", "podman")
	}

	// ── Game install root: try to probe the container instead of hardcoding
	//     `/AMP/duneawakening`. Falls back to the historical default if the
	//     probe can't answer (container not running, non-standard layout, etc.)
	//     Asked after the runtime question above so the probe uses the
	//     operator's chosen container CLI instead of assuming podman (#278).
	gameRoot := "/AMP/duneawakening"
	if useContainer && selected != nil && selected.InContainer {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		container := "AMP_" + cfg.AmpInstance
		if root, _ := probeGameRoot(probeCtx, cfg.AmpUser, cfg.AmpContainerRuntime, container); root != "" {
			gameRoot = root
			fmt.Printf("Detected game install root: %s\n", gameRoot)
		}
		cancel()
	}
	gameFolder := filepath.Base(gameRoot) // e.g. "duneawakening"

	defaultLogPath := gameRoot + "/logs"
	defaultIniDir := fmt.Sprintf("/home/%s/.ampdata/instances/%s/%s/server/state",
		cfg.AmpUser, cfg.AmpInstance, gameFolder)
	// The game's stock Default*.ini ship under the extracted game-server tree,
	// the same in container and native topologies.
	defaultDefaultIniDir := gameRoot + "/extracted/game-server/home/dune/server/DuneSandbox/Config"
	if !useContainer {
		// Native topology: game files live under <game-root>/extracted/...
		defaultLogPath = gameRoot + "/extracted/game-server/home/dune/server/DuneSandbox/Saved/Logs"
		defaultIniDir = gameRoot + "/extracted/game-server/home/dune/server/DuneSandbox/Saved/Config/LinuxServer"
	}

	cfg.AmpLogPath = ask("Log directory", defaultLogPath)
	cfg.DirectorURL = ask("Battlegroup Director URL (optional)", "http://127.0.0.1:11717")
	fmt.Println()

	// INI paths are fully determined by the AMP instance layout, so we derive
	// them instead of asking. The instance state dir holds UserOverrides.ini
	// (where dune-admin writes game settings) and ue5-saved/UserSettings (where
	// runtime auto-discovery finds UserGame.ini / UserEngine.ini).
	cfg.ServerIniDir = defaultIniDir
	cfg.DefaultIniDir = defaultDefaultIniDir
	fmt.Println("INI directories (auto-derived from the AMP instance):")
	fmt.Printf("  instance state: %s\n", cfg.ServerIniDir)
	fmt.Printf("  game defaults:  %s\n", cfg.DefaultIniDir)
	fmt.Println()

	fmt.Println("RabbitMQ broker (used by capture mode AND live RMQ commands):")
	var defaultBrokerPrefix string
	if useContainer {
		runtime := cfg.AmpContainerRuntime
		if runtime == "" {
			runtime = "podman"
		}
		defaultBrokerPrefix = fmt.Sprintf("sudo -i -u %s %s exec %s", cfg.AmpUser, runtime, cfg.AmpContainer)
	} else {
		defaultBrokerPrefix = fmt.Sprintf("sudo -i -u %s", cfg.AmpUser)
	}
	cfg.BrokerExecPrefix = ask("Broker exec prefix", defaultBrokerPrefix)
	fmt.Println()

	fmt.Println("Database connection:")
	cfg.DBHost = ask("DB host", "127.0.0.1")
	cfg.DBPort = dbPort
	portStr := ask(fmt.Sprintf("DB port [%d]", dbPort), fmt.Sprintf("%d", dbPort))
	_, _ = fmt.Sscanf(portStr, "%d", &cfg.DBPort)
	cfg.DBUser = ask("DB user", envOr("DB_USER", "dune"))
	cfg.DBPass = ask("DB password", "")
	if cfg.DBPass == "" {
		fmt.Fprintln(os.Stderr, "Database password is required. Aborting.")
		exitSetup(1)
	}
	cfg.DBName = ask("DB name", envOr("DB_NAME", "dune"))
	cfg.DBSchema = ask("DB schema", envOr("DB_SCHEMA", "dune"))
	fmt.Println()

	fmt.Println("Connecting to database...")
	dbHost = cfg.DBHost
	dbPort = cfg.DBPort
	dbUser = cfg.DBUser
	dbPass = cfg.DBPass
	dbName = cfg.DBName
	dbSchema = cfg.DBSchema
	exec := &localExecutor{}
	globalExecutor = &ampExecutor{Executor: exec, ampUser: cfg.AmpUser}
	pool, err := connectDBDirect(context.Background(), *cfg)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		exitSetup(1)
	}
	globalDB = pool
	globalControl = newControlPlane("amp", *cfg)
	ok("Database connected as: " + cfg.DBUser)
	fmt.Println()
	fmt.Println("Reminder: dune-admin writes game settings to UserOverrides.ini and engine")
	fmt.Println("settings to UserEngine.ini as " + cfg.AmpUser + " (AMP owns UserGame.ini itself).")
	fmt.Println("Example /etc/sudoers.d/dune-admin entry:")
	fmt.Printf("  %s ALL=(%s) NOPASSWD: /usr/bin/tee %s/UserOverrides.ini, /usr/bin/tee %s/ue5-saved/UserSettings/UserEngine.ini\n",
		envOr("USER", "dune-admin"), cfg.AmpUser, cfg.ServerIniDir, cfg.ServerIniDir)
	fmt.Println("  (If the service user does not own the INI files, also add: /usr/bin/cat)")
	fmt.Println()

	cfg.ScripCurrency = scripCurrencyID
}

// ── Market bot setup ───────────────────────────────────────────────────────────

func runMarketBotSetup(ask func(string, string) string, ok func(string), cfg *appConfig) {
	fmt.Println("Embedded market bot:")
	enabled := strings.ToLower(strings.TrimSpace(ask("Enable embedded market bot [yes/no]", "yes")))
	enabledBool := enabled != "n" && enabled != "no" && enabled != "false" && enabled != "0"
	cfg.MarketBotEnabled = &enabledBool
	if !enabledBool {
		ok("Embedded market bot disabled")

		// Offer remote proxy config.
		remoteURL := strings.TrimSpace(ask("Remote bot URL (leave blank to skip)", ""))
		if remoteURL != "" {
			cfg.MarketBotRemoteURL = remoteURL
			cfg.MarketBotRemoteToken = strings.TrimSpace(ask("Remote bot API token", ""))
			ok("Remote market bot proxy configured")
		}
		fmt.Println()
		return
	}

	cfg.MarketBotCacheDB = ask("Bot cache database path", filepath.Join(configDir(), "market-bot-cache.db"))
	cfg.MarketBotItemData = ask("Bot item-data.json path (optional)", "")

	parseFloat := func(input string, fallback float64) float64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
		if err != nil || v <= 0 {
			return fallback
		}
		return v
	}
	parseInt := func(input string, fallback int) int {
		v, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || v <= 0 {
			return fallback
		}
		return v
	}

	cfg.MarketBotBuyInt = ask("Buy tick interval", "5m")
	cfg.MarketBotListInt = ask("List tick interval", "30m")
	cfg.MarketBotThresh = parseFloat(ask("Buy threshold multiplier", "1.05"), 1.05)
	cfg.MarketBotMaxBuys = parseInt(ask("Max buys per tick", "50"), 50)
	ok("Embedded market bot configured")
	fmt.Println()
}

// ── Discord bot setup ──────────────────────────────────────────────────────────

// setupDiscordBot prompts for Discord bot configuration. Each owner creates
// their own Discord application at https://discord.com/developers/applications,
// copies the bot token, and pastes it here.
func setupDiscordBot(ok func(string), ask func(prompt, def string) string, cfg *appConfig) {
	fmt.Println("── Discord Bot (optional) ──────────────────────────────────────────────")
	fmt.Println("Each dune-admin instance uses its own Discord application (bring-your-own-bot).")
	fmt.Println("Create one at https://discord.com/developers/applications, copy the Bot token,")
	fmt.Println("then invite the bot to your guild with the 'applications.commands' scope.")
	fmt.Println()

	enableStr := strings.TrimSpace(ask("Enable Discord bot? (y/n)", "n"))
	if enableStr != "y" && enableStr != "Y" {
		ok("Discord bot disabled")
		fmt.Println()
		return
	}

	t := true
	cfg.DiscordBotEnabled = &t
	cfg.DiscordBotToken = strings.TrimSpace(ask("Bot token", ""))
	cfg.DiscordGuildID = strings.TrimSpace(ask("Guild (server) ID", ""))
	cfg.DiscordRolesViewer = strings.TrimSpace(ask("Viewer role IDs (comma-separated, leave blank to skip)", ""))
	cfg.DiscordRolesEconomy = strings.TrimSpace(ask("Economy role IDs (comma-separated, leave blank to skip)", ""))
	cfg.DiscordRolesAdmin = strings.TrimSpace(ask("Admin role IDs (comma-separated, leave blank to skip)", ""))
	cfg.DiscordAnnounceChannelID = strings.TrimSpace(ask("Announce channel ID (for reward/event announcements, leave blank to skip)", ""))

	ok("Discord bot configured")
	fmt.Println()
}

// ── Events engine setup ───────────────────────────────────────────────────────

func setupEventsEngine(ok func(string), ask func(string, string) string, cfg *appConfig) {
	fmt.Println("── Live Events Engine (optional) ───────────────────────────────────────────")
	enableStr := strings.TrimSpace(ask("Enable live events engine? (y/n)", "n"))
	if !strings.EqualFold(enableStr, "y") && !strings.EqualFold(enableStr, "yes") {
		f := false
		cfg.EventsEnabled = &f
		ok("Live events engine disabled")
		fmt.Println()
		return
	}
	t := true
	cfg.EventsEnabled = &t
	ok("Live events engine configured (poll interval set per event)")
	fmt.Println()
}

// ── Write config ──────────────────────────────────────────────────────────────

func writeSetupConfig(ok, fail func(string), cfg appConfig) {
	cfgDir := configDir()
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		fail("Failed to create config directory: " + err.Error())
		exitSetup(1)
	}
	cfgData, err := yaml.Marshal(cfg)
	if err != nil {
		fail("Failed to marshal config: " + err.Error())
		exitSetup(1)
	}
	cfgFile := configPath()
	if err := os.WriteFile(cfgFile, cfgData, 0600); err != nil {
		fail("Failed to write config: " + err.Error())
		exitSetup(1)
	}
	// Sync the in-memory config with what we just wrote. On a fresh install
	// loadConfig() found no file, so loadedConfig is still the zero value — and
	// MarketBotEnabled==nil defaults the embedded bot ON. Without this, the bot
	// (and other wizard choices) would ignore the operator's answers until the
	// next restart re-reads the file.
	loadedConfig = cfg
	ok("Config written to " + cfgFile)
	fmt.Println()
}
