package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppVersion is the conduit release version shown to users.
// Populated at build time via -ldflags "-X main.AppVersion=$(VERSION)".
// Defaults to "dev" so unreleased builds don't masquerade as a real
// release and don't trigger the update notifier.
var AppVersion = "dev"

// GitCommit and BuildTime are stamped at build time.
var GitCommit = "unknown"
var BuildTime = "unknown"

// ── config ────────────────────────────────────────────────────────────────────

var (
	captureMode        bool
	setupMode          bool
	sqlQuery           string
	sshHost            string
	sshUser            string
	sshKeyPath         string
	itemDataPath       string
	scripCurrencyID    int
	dbHost             string
	dbPort             int
	dbUser             string
	dbPass             string
	dbName             string
	dbSchema           string
	listenAddr         string
	controlPlane       string
	controlNS          string
	brokerGameAddr     string
	brokerAdminAddr    string
	brokerTLS          bool
	backupDir          string
	serverIniDir       string
	marketBotAddr      string
	marketBotToken     string
	marketBotContainer string
	marketBotNamespace string
)

// appConfig mirrors the fields written to ~/.dune-admin/config.yaml.
type appConfig struct {
	// Transport — SSH fields. If ssh_host is set all commands + TCP connections
	// tunnel through SSH. If omitted everything runs/connects locally.
	SSHHost string `yaml:"ssh_host"`
	SSHUser string `yaml:"ssh_user"`
	SSHKey  string `yaml:"ssh_key"`

	// Database — always required.
	DBHost   string `yaml:"db_host"`
	DBPort   int    `yaml:"db_port"`
	DBUser   string `yaml:"db_user"`
	DBPass   string `yaml:"db_pass"`
	DBName   string `yaml:"db_name"`
	DBSchema string `yaml:"db_schema"`

	// Control plane: "kubectl" | "docker" | "local"
	// Defaults to "kubectl" when ssh_host is set, "local" otherwise.
	Control string `yaml:"control"`

	// kubectl-specific
	ControlNamespace string `yaml:"control_namespace"`

	// docker-specific — container names
	DockerGameserver  string `yaml:"docker_gameserver"`
	DockerBrokerGame  string `yaml:"docker_broker_game"`
	DockerBrokerAdmin string `yaml:"docker_broker_admin"`
	DockerDB          string `yaml:"docker_db"`

	// local-specific — configurable shell commands
	CmdStart   string `yaml:"cmd_start"`
	CmdStop    string `yaml:"cmd_stop"`
	CmdRestart string `yaml:"cmd_restart"`
	CmdStatus  string `yaml:"cmd_status"`

	// Broker — optional; if set, notifications and capture are available.
	BrokerGameAddr  string `yaml:"broker_game_addr"`
	BrokerAdminAddr string `yaml:"broker_admin_addr"`
	BrokerTLS       bool   `yaml:"broker_tls"`
	BrokerUser      string `yaml:"broker_user"`
	BrokerPass      string `yaml:"broker_pass"`
	// BrokerExecPrefix is prepended to all rabbitmqctl calls. Use when the
	// broker runs inside a container that isn't managed by the docker control
	// plane — e.g. "podman exec AMP_MehDune01" or "docker exec my-broker".
	BrokerExecPrefix string `yaml:"broker_exec_prefix"`

	// Backups — optional path accessed via the executor.
	BackupDir string `yaml:"backup_dir"`

	// ServerIniDir is the directory containing UserGame.ini and UserOverrides.ini.
	// e.g. /home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state
	ServerIniDir string `yaml:"server_ini_dir"`

	// DefaultIniDir is a local or remote path that contains DefaultGame.ini and
	// DefaultEngine.ini — the base layer of the INI hierarchy. When set, these
	// files are read from here instead of being searched relative to ServerIniDir.
	// Example: /path/to/game/Config
	DefaultIniDir string `yaml:"default_ini_dir"`

	ScripCurrency int    `yaml:"scrip_currency"`
	ListenAddr    string `yaml:"listen_addr"`

	// Market bot — optional; enables bot control panel.
	// MarketBotAddr is the bot's HTTP API address (e.g. "http://market-bot.svc:8081").
	// MarketBotContainer is the deployment name (kubectl) or container name (docker).
	// MarketBotNamespace is only used by the kubectl control plane.
	MarketBotAddr      string `yaml:"market_bot_addr"`
	MarketBotToken     string `yaml:"market_bot_token"`
	MarketBotContainer string `yaml:"market_bot_container"`
	MarketBotNamespace string `yaml:"market_bot_namespace"`

	// AMP-specific — used when Control == "amp" (CubeCoders AMP w/ podman).
	// AmpInstance is the ampinstmgr instance name (e.g. "MehDune01").
	// AmpContainer is the podman container name (default "AMP_<instance>").
	// AmpUser is the OS user that runs AMP (default "amp"); used both for
	// sudo elevation of file writes and for podman exec invocations.
	// AmpLogPath is the in-container log directory.
	// DirectorURL is the optional Battlegroup Director URL — if set, the HTTP
	// router proxies /director/ to it (works in any control mode).
	AmpInstance  string `yaml:"amp_instance"`
	AmpContainer string `yaml:"amp_container"`
	AmpUser      string `yaml:"amp_user"`
	AmpLogPath   string `yaml:"amp_log_path"`
	// AmpUseContainer toggles between the two AMP topologies. When true (default,
	// matching CubeCoders' containerised template), commands are wrapped in
	// `podman exec`. When false, AMP runs the game server natively on the host
	// as the AMP user; the same operations run directly via sudo.
	AmpUseContainer *bool `yaml:"amp_use_container"`
	// AmpRabbitmqctlPath is the absolute path to rabbitmqctl. AMP bundles its
	// own rabbitmq under <amp_data>/<game>/extracted/mq/opt/rabbitmq/sbin and
	// it's not in $PATH inside the container. The setup wizard prefills the
	// Dune Awakening default; override here for other AMP game modules.
	AmpRabbitmqctlPath string `yaml:"amp_rabbitmqctl_path"`
	// AmpDataRoot is the per-game data root inside the AMP container (or on
	// the host in native mode). Defaults to /AMP/duneawakening — the
	// CubeCoders Dune Awakening module convention. The rabbitmqctl wrapper
	// derives loader, escript, broker-runtime, and erlang-cookie paths from
	// this. Override for other AMP game modules whose layout differs.
	AmpDataRoot string `yaml:"amp_data_root"`
	DirectorURL string `yaml:"director_url"`

	// FrontendDir overrides the auto-detected SPA directory. When unset the
	// server looks in ./dist then ./web/dist and serves the first match.
	FrontendDir string `yaml:"frontend_dir"`
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".dune-admin"
	}
	return filepath.Join(home, ".dune-admin")
}

func configPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

func setEnvIfMissing(key, val string) {
	if os.Getenv(key) == "" && val != "" {
		_ = os.Setenv(key, val)
	}
}

// loadedConfig holds the full parsed config.yaml so provider-specific fields
// (docker_*, cmd_*) remain available to connectAll() even though they have no
// corresponding env var or flag.
var loadedConfig appConfig

// loadConfig reads ~/.dune-admin/config.yaml and falls back to .env in the
// working directory for backward compatibility with existing unzipped-release
// installs.
func loadConfig() {
	data, err := os.ReadFile(configPath())
	if err == nil {
		var cfg appConfig
		if yaml.Unmarshal(data, &cfg) == nil {
			loadedConfig = cfg
			setEnvIfMissing("SSH_HOST", cfg.SSHHost)
			setEnvIfMissing("SSH_USER", cfg.SSHUser)
			setEnvIfMissing("SSH_KEY", cfg.SSHKey)
			setEnvIfMissing("DB_HOST", cfg.DBHost)
			if cfg.DBPort != 0 {
				setEnvIfMissing("DB_PORT", strconv.Itoa(cfg.DBPort))
			}
			setEnvIfMissing("DB_USER", cfg.DBUser)
			setEnvIfMissing("DB_PASS", cfg.DBPass)
			setEnvIfMissing("DB_NAME", cfg.DBName)
			setEnvIfMissing("DB_SCHEMA", cfg.DBSchema)
			if cfg.ScripCurrency != 0 {
				setEnvIfMissing("SCRIP_CURRENCY", strconv.Itoa(cfg.ScripCurrency))
			}
			setEnvIfMissing("LISTEN_ADDR", cfg.ListenAddr)
			setEnvIfMissing("CONTROL", cfg.Control)
			setEnvIfMissing("CONTROL_NAMESPACE", cfg.ControlNamespace)
			setEnvIfMissing("BROKER_GAME_ADDR", cfg.BrokerGameAddr)
			setEnvIfMissing("BROKER_ADMIN_ADDR", cfg.BrokerAdminAddr)
			setEnvIfMissing("BACKUP_DIR", cfg.BackupDir)
			setEnvIfMissing("SERVER_INI_DIR", cfg.ServerIniDir)
			setEnvIfMissing("MARKET_BOT_ADDR", cfg.MarketBotAddr)
			setEnvIfMissing("MARKET_BOT_TOKEN", cfg.MarketBotToken)
			setEnvIfMissing("MARKET_BOT_CONTAINER", cfg.MarketBotContainer)
			setEnvIfMissing("MARKET_BOT_NAMESPACE", cfg.MarketBotNamespace)
			return
		}
	}
	loadDotEnv()
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		setEnvIfMissing(k, v)
	}
}

// envOr returns the environment variable value if set, otherwise def.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func init() {
	loadConfig()
	flag.StringVar(&sshHost, "host", envOr("SSH_HOST", ""), "SSH host:port (if set, all connections tunnel through SSH)")
	flag.StringVar(&sshUser, "user", envOr("SSH_USER", "dune"), "SSH user")
	flag.StringVar(&sshKeyPath, "key", envOr("SSH_KEY", ""), "SSH private key path (auto-detected if empty)")
	flag.StringVar(&itemDataPath, "itemdata", envOr("ITEM_DATA", ""), "Item data JSON path")
	flag.IntVar(&scripCurrencyID, "scripcurrency", envIntOr("SCRIP_CURRENCY", 1), "Scrip currency id")
	flag.StringVar(&dbHost, "dbhost", envOr("DB_HOST", "127.0.0.1"), "PostgreSQL host or DNS name")
	flag.IntVar(&dbPort, "dbport", envIntOr("DB_PORT", 15432), "PostgreSQL port")
	flag.StringVar(&dbUser, "dbuser", envOr("DB_USER", "dune"), "PostgreSQL user")
	flag.StringVar(&dbPass, "dbpass", envOr("DB_PASS", ""), "PostgreSQL password")
	flag.StringVar(&dbName, "dbname", envOr("DB_NAME", "dune"), "PostgreSQL database name")
	flag.StringVar(&dbSchema, "schema", envOr("DB_SCHEMA", "dune"), "PostgreSQL schema")
	flag.StringVar(&listenAddr, "addr", envOr("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&controlPlane, "control", envOr("CONTROL", ""), "Control plane: kubectl | docker | local")
	flag.StringVar(&controlNS, "control-ns", envOr("CONTROL_NAMESPACE", ""), "Kubernetes namespace (kubectl control plane)")
	flag.StringVar(&brokerGameAddr, "broker-game", envOr("BROKER_GAME_ADDR", ""), "mq-game broker address host:port")
	flag.StringVar(&brokerAdminAddr, "broker-admin", envOr("BROKER_ADMIN_ADDR", ""), "mq-admin broker address host:port")
	flag.StringVar(&backupDir, "backup-dir", envOr("BACKUP_DIR", ""), "Backup directory path")
	flag.StringVar(&serverIniDir, "ini-dir", envOr("SERVER_INI_DIR", ""), "Directory containing UserGame.ini / UserOverrides.ini")
	flag.StringVar(&marketBotAddr, "market-bot-addr", envOr("MARKET_BOT_ADDR", ""), "Market bot HTTP API address (e.g. http://host:8081)")
	flag.StringVar(&marketBotToken, "market-bot-token", envOr("MARKET_BOT_TOKEN", ""), "Market bot bearer token")
	flag.StringVar(&marketBotContainer, "market-bot-container", envOr("MARKET_BOT_CONTAINER", "market-bot"), "Market bot container/deployment name")
	flag.StringVar(&marketBotNamespace, "market-bot-namespace", envOr("MARKET_BOT_NAMESPACE", ""), "Market bot k8s namespace")
	flag.BoolVar(&captureMode, "capture", false, "Capture RabbitMQ messages (grant + notifications) and print to stdout")
	flag.BoolVar(&setupMode, "setup", false, "Interactive setup wizard — writes ~/.dune-admin/config.yaml")
	flag.StringVar(&sqlQuery, "sql", "", "Run a SQL query and print results to stdout, then exit")
}

func resolveKeyPath() string {
	if sshKeyPath != "" {
		return sshKeyPath
	}
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(home, ".dune-admin", "sshKey"), // user config dir (package-manager installs)
		filepath.Join(exeDir, "sshKey"),              // next to the binary (drag-and-drop / unzipped release)
		"./sshKey",                                   // working directory fallback
	}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append([]string{filepath.Join(localAppData, "DuneSandboxServer", "sshKey")}, candidates...)
		}
	}
	for _, p := range candidates { // #nosec G703 -- paths are hardcoded candidates, not user input
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(home, ".dune-admin", "sshKey")
}

func resolveItemDataPath() string {
	if itemDataPath != "" {
		return itemDataPath
	}
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(home, ".dune-admin", "item-data.json"),
		filepath.Join(exeDir, "item-data.json"),
		filepath.Join(exeDir, "..", "share", "dune-admin", "item-data.json"), // Homebrew pkgshare
		"./item-data.json",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func resolveTagsDataPath() string {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(home, ".dune-admin", "tags-data.json"),
		filepath.Join(exeDir, "tags-data.json"),
		filepath.Join(exeDir, "..", "share", "dune-admin", "tags-data.json"), // Homebrew pkgshare
		"./tags-data.json",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var tagsData tagsDataFile

func loadTagsData() error {
	path := resolveTagsDataPath()
	if path == "" {
		return fmt.Errorf("tags-data.json not found — contract picker will be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tags data %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &tagsData); err != nil {
		return fmt.Errorf("parse tags data %s: %w", path, err)
	}
	return nil
}

var itemData itemDataFile

func loadItemData() error {
	path := resolveItemDataPath()
	if path == "" {
		return fmt.Errorf("item-data.json not found — item grant features will be broken")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read item data %s: %w", path, err)
	}
	var parsed itemDataFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("parse item data %s: %w", path, err)
	}
	normalizedItems := make(map[string]itemRule, len(parsed.Items))
	for k, v := range parsed.Items {
		normalizedItems[strings.ToLower(k)] = v
	}
	parsed.Items = normalizedItems
	normalizedNames := make(map[string]string, len(parsed.Names))
	for k, v := range parsed.Names {
		normalizedNames[strings.ToLower(k)] = v
	}
	parsed.Names = normalizedNames
	itemData = parsed
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func needsSetup() bool {
	// config.yaml takes priority over legacy .env.
	if _, err := os.Stat(configPath()); err == nil {
		return dbPass == ""
	}
	if _, err := os.Stat(".env"); err == nil {
		return dbPass == ""
	}
	return true
}

func main() {
	flag.Parse()

	// Explicit -setup flag: reconfigure and exit (don't start server).
	if setupMode {
		runSetup()
		return
	}

	if captureMode {
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "SSH connect:", msg.err)
			os.Exit(1)
		}
		runCapture()
		return
	}

	if sqlQuery != "" {
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "connect:", msg.err)
			os.Exit(1)
		}
		if msg, ok := cmdRunSQL(sqlQuery)().(msgSQL); ok {
			if msg.err != nil {
				fmt.Fprintln(os.Stderr, msg.err)
				os.Exit(1)
			}
			fmt.Println(msg.result)
		}
		return
	}

	if err := loadItemData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := loadTagsData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Auto-run setup wizard when no config exists — setup leaves us connected.
	alreadyConnected := false
	if needsSetup() {
		runSetup()
		alreadyConnected = true
		fmt.Println()
		fmt.Printf("Starting server on %s...\n", listenAddr)
	}

	defer func() {
		if globalDB != nil {
			globalDB.Close()
		}
		if globalSSH != nil {
			_ = globalSSH.Close()
		}
	}()

	if !alreadyConnected {
		// Connect synchronously (SSH + DB).
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "connect:", msg.err)
			fmt.Fprintln(os.Stderr, "Starting server anyway — use /api/v1/reconnect to retry")
		} else {
			if msg, ok := cmdFetchItemTemplates().(msgItemTemplates); ok {
				mergeItemTemplates(msg.templates)
			}
		}
	} else {
		// Already connected by setup; just populate item templates.
		if msg, ok := cmdFetchItemTemplates().(msgItemTemplates); ok {
			mergeItemTemplates(msg.templates)
		}
	}

	startServer(listenAddr)
}
