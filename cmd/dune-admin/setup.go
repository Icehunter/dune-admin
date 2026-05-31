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
	cfg.ListenAddr = ask("HTTP listen address", defaultListenAddr)
	fmt.Println()

	// ── Write config ───────────────────────────────────────────────────────────

	writeSetupConfig(ok, fail, cfg)

	fmt.Println("Setup complete.")
	fmt.Println()
	fmt.Println("  Run: dune-admin")
	fmt.Println()
}

// ── kubectl setup flow ────────────────────────────────────────────────────────

func setupKubectlSSHKey(ask func(string, string) string, ok, fail func(string)) string {
	// SSH key
	fmt.Println("Checking for SSH key...")
	keyPath := resolveKeyPath()
	if _, err := os.Stat(keyPath); err != nil {
		fail("SSH key not found (checked ~/.dune-admin/sshKey, next to binary, ./sshKey)")
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
	sshHost = ask("VM host:port", envOr("SSH_HOST", "192.168.0.72:22"))
	sshUser = ask("SSH user", envOr("SSH_USER", "dune"))
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
	pool, err := connectDB(context.Background(), discoveredUser, discoveredPass)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		fmt.Println()
		fmt.Printf("  The password may be wrong. Delete %s and re-run to try again.\n", configPath())
		exitSetup(1)
	}
	globalDB = pool
	globalExecutor = exec
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

func runDockerSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	fmt.Println("Docker container names:")
	cfg.DockerGameserver = ask("Game server container name", "dune-gameserver")
	cfg.DockerBrokerGame = ask("mq-game broker container name (optional)", "")
	cfg.DockerBrokerAdmin = ask("mq-admin broker container name (optional)", "")
	fmt.Println()

	// Test docker access
	exec := &localExecutor{}
	out, err := exec.Exec(fmt.Sprintf("docker inspect --format '{{.State.Status}}' %s 2>&1", cfg.DockerGameserver))
	if err != nil {
		fail(fmt.Sprintf("docker inspect failed: %s", out))
		fmt.Println("  Make sure Docker is running and the container name is correct.")
		fmt.Println("  Continuing anyway...")
	} else {
		ok(fmt.Sprintf("Container %s is %s", cfg.DockerGameserver, strings.TrimSpace(out)))
	}
	fmt.Println()

	fmt.Println("Database connection:")
	cfg.DBHost = ask("DB host (Docker DNS or IP)", "database")
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

	// ── Game install root: try to probe the container instead of hardcoding
	//     `/AMP/duneawakening`. Falls back to the historical default if the
	//     probe can't answer (container not running, non-standard layout, etc.)
	gameRoot := "/AMP/duneawakening"
	if useContainer && selected != nil && selected.InContainer {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		container := "AMP_" + cfg.AmpInstance
		if root, _ := probeGameRoot(probeCtx, cfg.AmpUser, container); root != "" {
			gameRoot = root
			fmt.Printf("Detected game install root: %s\n", gameRoot)
		}
		cancel()
	}
	gameFolder := filepath.Base(gameRoot) // e.g. "duneawakening"

	defaultLogPath := gameRoot + "/logs"
	defaultIniDir := fmt.Sprintf("/home/%s/.ampdata/instances/%s/%s/server/state",
		cfg.AmpUser, cfg.AmpInstance, gameFolder)
	if !useContainer {
		// Native topology: game files live under <game-root>/extracted/...
		defaultLogPath = gameRoot + "/extracted/game-server/home/dune/server/DuneSandbox/Saved/Logs"
		defaultIniDir = gameRoot + "/extracted/game-server/home/dune/server/DuneSandbox/Saved/Config/LinuxServer"
	}

	if useContainer {
		defaultContainer := "AMP_" + cfg.AmpInstance
		cfg.AmpContainer = ask("Container name", defaultContainer)
		cfg.AmpContainerRuntime = ask("Container runtime [podman/docker]", "podman")
	}
	cfg.AmpLogPath = ask("Log directory", defaultLogPath)
	cfg.DirectorURL = ask("Battlegroup Director URL (optional)", "http://127.0.0.1:11717")
	fmt.Println()

	fmt.Println("INI directories:")
	cfg.ServerIniDir = ask("UserGame.ini directory (host path)", defaultIniDir)
	cfg.DefaultIniDir = ask("DefaultGame.ini directory (optional)", "")
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
	fmt.Println("Reminder: dune-admin needs sudoers grants to read and write UserGame.ini as " + cfg.AmpUser + ".")
	fmt.Println("Example /etc/sudoers.d/dune-admin entry:")
	fmt.Printf("  %s ALL=(%s) NOPASSWD: /usr/bin/tee %s/UserGame.ini, /usr/bin/tee %s/UserEngine.ini\n",
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
	ok("Config written to " + cfgFile)
	fmt.Println()
}
