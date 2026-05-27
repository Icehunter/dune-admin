package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

	// ── Market bot (optional) ──────────────────────────────────────────────────

	runMarketBotSetup(ask, ok, ctrl, &cfg)

	// ── Common: listen address ─────────────────────────────────────────────────

	fmt.Println("Server config:")
	cfg.ListenAddr = ask("HTTP listen address", listenAddr)
	fmt.Println()

	// ── Write config ───────────────────────────────────────────────────────────

	writeSetupConfig(ok, fail, cfg)

	fmt.Println("Setup complete.")
	fmt.Println()
	fmt.Println("  Run: dune-admin")
	fmt.Println()
}

// ── kubectl setup flow ────────────────────────────────────────────────────────

func runKubectlSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
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

	// Discover DB pod
	fmt.Println("Discovering database pod...")
	sshExecWrap := &sshExecutor{client: client}
	ns, pod, podIP, err := discoverDBPod(sshExecWrap)
	if err != nil {
		fail("Pod discovery failed: " + err.Error())
		fmt.Println()
		fmt.Println("  Make sure the SSH user can run: sudo kubectl get pods -A")
		exitSetup(1)
	}
	globalSSH = client
	globalPodNS = ns
	globalPod = pod
	globalPodIP = podIP
	ok("Database pod: " + pod)
	fmt.Println()

	// Discover DB password
	fmt.Println("Discovering database password...")
	discoveredUser := "postgres"
	discoveredPass := ""

	var battlegroups []string
	if bg := battlegroupFromPod(globalPod); bg != "" {
		battlegroups = []string{bg}
	} else {
		battlegroups = listBattlegroups(sshExecWrap)
	}

	if len(battlegroups) == 0 {
		fmt.Println("  Could not determine battlegroup name")
	} else {
		chosen := battlegroups[0]
		if len(battlegroups) > 1 {
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
		}
		yamlPath := fmt.Sprintf("~/.dune/%s.yaml", chosen)
		if u, pass := extractPasswordFromYAML(sshExecWrap, yamlPath); pass != "" {
			discoveredUser = u
			discoveredPass = pass
			ok(fmt.Sprintf("Password found in %s (user: %s)", yamlPath, u))
		} else {
			fail("No password found in " + yamlPath)
		}
		cfg.ControlNamespace = ns
		controlNS = ns
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
	globalExecutor = sshExecWrap
	globalControl = newControlPlane("kubectl", *cfg)
	ok("Database connected as: " + dbUser)
	fmt.Println()

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

// runAmpSetup configures the AMP control plane (CubeCoders AMP). AMP supports
// two deployment topologies — game server in a podman container, or running
// natively on the host as the AMP user. All paths/names are configurable.
func runAmpSetup(ask func(string, string) string, ok, fail func(string), cfg *appConfig) {
	fmt.Println("AMP instance:")
	cfg.AmpInstance = ask("Instance name (ampinstmgr instance)", "DuneAwakening01")
	cfg.AmpUser = ask("OS user that runs AMP", "amp")
	fmt.Println()

	fmt.Println("AMP topology:")
	fmt.Println("  container — game server runs inside `podman exec AMP_<instance>` (default template)")
	fmt.Println("  native    — game server runs directly on the host as the AMP user")
	topology := ask("Topology [container/native]", "container")
	useContainer := topology != "native"
	cfg.AmpUseContainer = &useContainer

	defaultLogPath := "/AMP/duneawakening/logs"
	defaultIniDir := fmt.Sprintf("/home/%s/.ampdata/instances/%s/duneawakening/server/state",
		cfg.AmpUser, cfg.AmpInstance)
	if !useContainer {
		// Native topology: game files live under /AMP/<game>/extracted/...
		defaultLogPath = "/AMP/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Saved/Logs"
		defaultIniDir = "/AMP/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Saved/Config/LinuxServer"
	}

	if useContainer {
		defaultContainer := "AMP_" + cfg.AmpInstance
		cfg.AmpContainer = ask("Podman container name", defaultContainer)
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
		defaultBrokerPrefix = fmt.Sprintf("sudo -i -u %s podman exec %s", cfg.AmpUser, cfg.AmpContainer)
	} else {
		defaultBrokerPrefix = fmt.Sprintf("sudo -i -u %s", cfg.AmpUser)
	}
	cfg.BrokerExecPrefix = ask("Broker exec prefix", defaultBrokerPrefix)
	// AMP bundles its own rabbitmqctl and doesn't put it on $PATH. The Dune
	// Awakening module ships it at the path below; other AMP game modules use
	// the same /AMP/<game>/extracted/mq/opt/rabbitmq/sbin/ layout.
	cfg.AmpRabbitmqctlPath = ask("rabbitmqctl absolute path", defaultDuneRabbitmqctl)
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
	globalExecutor = &ampExecutor{localExecutor: exec, ampUser: cfg.AmpUser}
	pool, err := connectDBDirect(context.Background(), *cfg)
	if err != nil {
		fail("DB connect failed: " + err.Error())
		exitSetup(1)
	}
	globalDB = pool
	globalControl = newControlPlane("amp", *cfg)
	ok("Database connected as: " + cfg.DBUser)
	fmt.Println()
	fmt.Println("Reminder: dune-admin needs sudoers grants to write UserGame.ini as " + cfg.AmpUser + ".")
	fmt.Println("Example /etc/sudoers.d/dune-admin entry:")
	fmt.Printf("  %s ALL=(%s) NOPASSWD: /usr/bin/tee %s/UserGame.ini, /usr/bin/tee %s/UserEngine.ini\n",
		envOr("USER", "dune-admin"), cfg.AmpUser, cfg.ServerIniDir, cfg.ServerIniDir)
	fmt.Println()

	cfg.ScripCurrency = scripCurrencyID
}

// ── Market bot setup (optional) ───────────────────────────────────────────────

// runMarketBotSetup asks optional market-bot connection details.
// Defaults: container/deployment = "market-bot"; addr derived from SSH host
// (kubectl) or localhost (docker/local). Blank responses skip the section.
func runMarketBotSetup(ask func(string, string) string, ok func(string), ctrl string, cfg *appConfig) {
	fmt.Println("Market bot (optional — press Enter to skip each field):")

	// Compute a sensible default address.
	defaultAddr := "http://localhost:8081"
	if ctrl == "kubectl" && sshHost != "" {
		host := sshHost
		if h, _, err := splitHostPort(host); err == nil {
			host = h
		}
		defaultAddr = "http://" + host + ":8081"
	}

	cfg.MarketBotAddr = ask("Bot API address", defaultAddr)

	// Only ask for the rest if an address was supplied.
	if cfg.MarketBotAddr != "" {
		cfg.MarketBotToken = ask("Bot API token (optional)", "")
		cfg.MarketBotContainer = ask("Bot deployment/container name", "market-bot")
		if ctrl == "kubectl" {
			cfg.MarketBotNamespace = ask("Bot k8s namespace", "dune-market-bot")
		}
		marketBotAddr = cfg.MarketBotAddr
		marketBotToken = cfg.MarketBotToken
		marketBotContainer = cfg.MarketBotContainer
		marketBotNamespace = cfg.MarketBotNamespace
		ok("Market bot configured at " + cfg.MarketBotAddr)
	}
	fmt.Println()
}

// splitHostPort splits host:port. Returns an error if no port is present.
func splitHostPort(hostport string) (host, port string, err error) {
	for i := len(hostport) - 1; i >= 0; i-- {
		if hostport[i] == ':' {
			return hostport[:i], hostport[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("no port")
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
