// @title dune-admin API
// @version 1.0
// @description Admin panel API for a Dune Awakening private server.
// @host localhost:8080
// @BasePath /

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "dune-admin/docs"
	"gopkg.in/yaml.v3"

	"dune-admin/internal/marketbot"
)

// AppVersion is the release version shown to users.
// Populated at build time via -ldflags "-X main.AppVersion=$(VERSION)".
// Falls back to "<VERSION file>-dev" for source builds without ldflags.
var AppVersion = "dev"

// GitCommit and BuildTime are stamped at build time.
var GitCommit = "unknown"
var BuildTime = "unknown"

func init() {
	AppVersion = resolveAppVersion(AppVersion, ".")
}

// resolveAppVersion returns ldflagsVersion when it was set by the build pipeline.
// For plain `go build` / `go run` invocations that leave the default "dev", it
// reads the VERSION file from workDir and returns "<version>-dev" so operators
// can still tell which codebase they're running and update notifications work.
func resolveAppVersion(ldflagsVersion, workDir string) string {
	if ldflagsVersion != "dev" {
		return ldflagsVersion
	}
	data, err := os.ReadFile(filepath.Join(workDir, "VERSION"))
	if err != nil {
		return "dev"
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "dev"
	}
	return v + "-dev"
}

// ── config ────────────────────────────────────────────────────────────────────

var (
	setupMode       bool
	cleanMarketMode bool
	updateMode      bool
	reinstallMode   bool
	sqlQuery        string
	renderK8SOut    string
	sshHost         string
	sshUser         string
	sshKeyPath      string
	itemDataPath    string
	scripCurrencyID int
	dbHost          string
	dbPort          int
	dbUser          string
	dbPass          string
	dbName          string
	dbSchema        string
	listenAddr      string
	controlPlane    string
	controlNS       string
	brokerGameAddr  string
	brokerAdminAddr string
	brokerTLS       bool
	brokerUser      string
	brokerPass      string
	backupDir       string
	serverIniDir    string
)

// appConfig mirrors the fields written to ~/.dune-admin/config.yaml.
// json tags match yaml tags so the /api/v1/config endpoint speaks snake_case
// to the frontend without needing a separate DTO.
type appConfig struct {
	// Transport — SSH fields. If ssh_host is set all commands + TCP connections
	// tunnel through SSH. If omitted everything runs/connects locally.
	SSHHost string `yaml:"ssh_host" json:"ssh_host"`
	SSHUser string `yaml:"ssh_user" json:"ssh_user"`
	SSHKey  string `yaml:"ssh_key"  json:"ssh_key"`

	// Database — always required.
	DBHost   string `yaml:"db_host"   json:"db_host"`
	DBPort   int    `yaml:"db_port"   json:"db_port"`
	DBUser   string `yaml:"db_user"   json:"db_user"`
	DBPass   string `yaml:"db_pass"   json:"db_pass"`
	DBName   string `yaml:"db_name"   json:"db_name"`
	DBSchema string `yaml:"db_schema" json:"db_schema"`

	// Control plane: "kubectl" | "docker" | "local" | "amp"
	Control string `yaml:"control" json:"control"`

	// kubectl-specific
	ControlNamespace string `yaml:"control_namespace" json:"control_namespace"`

	// docker-specific — container names
	DockerGameserver  string `yaml:"docker_gameserver"  json:"docker_gameserver"`
	DockerBrokerGame  string `yaml:"docker_broker_game"  json:"docker_broker_game"`
	DockerBrokerAdmin string `yaml:"docker_broker_admin" json:"docker_broker_admin"`
	DockerDB          string `yaml:"docker_db"           json:"docker_db"`

	// local-specific — configurable shell commands
	CmdStart   string `yaml:"cmd_start"   json:"cmd_start"`
	CmdStop    string `yaml:"cmd_stop"    json:"cmd_stop"`
	CmdRestart string `yaml:"cmd_restart" json:"cmd_restart"`
	CmdStatus  string `yaml:"cmd_status"  json:"cmd_status"`

	// Broker — optional; if set, notifications and capture are available.
	BrokerGameAddr  string `yaml:"broker_game_addr"  json:"broker_game_addr"`
	BrokerAdminAddr string `yaml:"broker_admin_addr" json:"broker_admin_addr"`
	BrokerTLS       bool   `yaml:"broker_tls"        json:"broker_tls"`
	BrokerUser      string `yaml:"broker_user"       json:"broker_user"`
	BrokerPass      string `yaml:"broker_pass"       json:"broker_pass"`
	// BrokerJWTSecret is the base64-encoded HMAC key used to re-sign
	// ServiceAuthTokens for CaptureJWT. Optional override for the baked-in
	// default signing key (captureJWTSecretB64).
	BrokerJWTSecret string `yaml:"broker_jwt_secret"  json:"broker_jwt_secret"`
	// BrokerExecPrefix is prepended to all rabbitmqctl calls. Use when the
	// broker runs inside a container that isn't managed by the docker control
	// plane — e.g. "podman exec AMP_MehDune01" or "docker exec my-broker".
	BrokerExecPrefix string `yaml:"broker_exec_prefix" json:"broker_exec_prefix"`

	// Backups — optional path accessed via the executor.
	BackupDir string `yaml:"backup_dir" json:"backup_dir"`

	// ServerIniDir is the directory containing UserGame.ini and UserOverrides.ini.
	ServerIniDir string `yaml:"server_ini_dir"  json:"server_ini_dir"`
	// DefaultIniDir is a local or remote path that contains DefaultGame.ini and
	// DefaultEngine.ini — the base layer of the INI hierarchy.
	DefaultIniDir string `yaml:"default_ini_dir" json:"default_ini_dir"`

	ScripCurrency int    `yaml:"scrip_currency" json:"scrip_currency"`
	ListenAddr    string `yaml:"listen_addr"    json:"listen_addr"`

	// AMP-specific — used when Control == "amp" (CubeCoders AMP w/ podman).
	AmpInstance     string `yaml:"amp_instance"      json:"amp_instance"`
	AmpContainer    string `yaml:"amp_container"     json:"amp_container"`
	AmpUser         string `yaml:"amp_user"          json:"amp_user"`
	AmpLogPath      string `yaml:"amp_log_path"      json:"amp_log_path"`
	AmpUseContainer *bool  `yaml:"amp_use_container" json:"amp_use_container"`
	AmpDataRoot     string `yaml:"amp_data_root"     json:"amp_data_root"`
	DirectorURL     string `yaml:"director_url"      json:"director_url"`

	// ── Embedded market bot ────────────────────────────────────────────────
	// MarketBotEnabled starts the market bot as an in-process goroutine.
	// Pointer so we can distinguish "unset" (default-on) from "explicitly false".
	MarketBotEnabled     *bool   `yaml:"market_bot_enabled"   json:"market_bot_enabled"`
	MarketBotCacheDB     string  `yaml:"market_bot_cache_db"  json:"market_bot_cache_db"`
	MarketBotItemData    string  `yaml:"market_bot_item_data" json:"market_bot_item_data"`
	MarketBotState       string  `yaml:"market_bot_state"     json:"market_bot_state"`
	MarketBotBuyInt      string  `yaml:"market_bot_buy_interval"  json:"market_bot_buy_interval"`
	MarketBotListInt     string  `yaml:"market_bot_list_interval" json:"market_bot_list_interval"`
	MarketBotThresh      float64 `yaml:"market_bot_buy_threshold" json:"market_bot_buy_threshold"`
	MarketBotMaxBuys     int     `yaml:"market_bot_max_buys"      json:"market_bot_max_buys"`
	MarketBotRemoteURL   string  `yaml:"market_bot_remote_url"   json:"market_bot_remote_url"`
	MarketBotRemoteToken string  `yaml:"market_bot_remote_token" json:"market_bot_remote_token"`

	// ── Welcome package ────────────────────────────────────────────────────
	// Auto-grants a configured item package to every player once, on first
	// login. Defaults OFF — it mutates every player's inventory, so it must be
	// explicitly opted into. Bump the version to re-issue to everyone.
	WelcomePackageEnabled  *bool                `yaml:"welcome_package_enabled"            json:"welcome_package_enabled"`
	WelcomePackageVersion  string               `yaml:"welcome_package_version"            json:"welcome_package_version"`
	WelcomePackageScanSecs int                  `yaml:"welcome_package_scan_interval_secs" json:"welcome_package_scan_interval_secs"`
	WelcomePackageItems    []welcomePackageItem `yaml:"welcome_package_items"              json:"welcome_package_items"`
}

// marketBotEnabled returns the effective bot-enabled flag. Missing yaml key →
// default on (so upgrades enable the feature). Explicit false → off.
func marketBotEnabled(cfg appConfig) bool {
	if cfg.MarketBotEnabled == nil {
		return true
	}
	return *cfg.MarketBotEnabled
}

// welcomePackageEnabled returns the effective flag. Unlike the market bot,
// this defaults OFF — it grants items to every player, so it must be opt-in.
func welcomePackageEnabled(cfg appConfig) bool {
	if cfg.WelcomePackageEnabled == nil {
		return false
	}
	return *cfg.WelcomePackageEnabled
}

// startWelcomePackageScanner opens the ledger store, seeds the live runtime
// config, and starts the scanner goroutine. The goroutine always runs so the
// feature can be toggled on at runtime via the API; each tick is a cheap no-op
// while disabled. Returns a cancel func, or nil if the store could not open.
func startWelcomePackageScanner(cfg appConfig) context.CancelFunc {
	store, err := openWelcomeStore(filepath.Join(configDir(), "welcome-package.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "welcome-package: store open failed: %v\n", err)
		return nil
	}
	welcomeStoreDB = store

	setWelcomeRuntime(buildWelcomeRuntime(
		welcomePackageEnabled(cfg),
		cfg.WelcomePackageVersion,
		cfg.WelcomePackageScanSecs,
		cfg.WelcomePackageItems,
	))

	ctx, cancel := context.WithCancel(context.Background())
	go runWelcomePackageScanner(ctx)
	return cancel
}

func configDir() string {
	// DUNE_ADMIN_CONFIG_DIR allows operators to redirect config to a writable
	// path in environments where the home directory is read-only (e.g. K8s with
	// a ConfigMap-mounted home dir, or containers with a read-only root fs).
	if dir := os.Getenv("DUNE_ADMIN_CONFIG_DIR"); dir != "" {
		return dir
	}
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
			setEnvIfMissing("BROKER_USER", cfg.BrokerUser)
			setEnvIfMissing("BROKER_PASS", cfg.BrokerPass)
			setEnvIfMissing("BROKER_JWT_SECRET", cfg.BrokerJWTSecret)
			setEnvIfMissing("BACKUP_DIR", cfg.BackupDir)
			setEnvIfMissing("SERVER_INI_DIR", cfg.ServerIniDir)
			detectStaleEnvFile(".")
			return
		}
	}
	loadDotEnv()
}

// detectStaleEnvFile warns when a .env file exists in workDir alongside a
// successfully-loaded config.yaml. A stale .env is ignored by dune-admin, but
// values pre-exported into the process environment before startup (e.g. via a
// shell that sourced the old file) can shadow config.yaml and silently break
// features like market-bot control. Returns true when the file is detected.
func detectStaleEnvFile(workDir string) bool {
	if _, err := os.Stat(filepath.Join(workDir, ".env")); err != nil {
		return false
	}
	log.Printf("[WARN] stale .env file found in %s", workDir)
	log.Printf("[WARN] dune-admin is using %s — .env is ignored.", configPath())
	log.Printf("[WARN] However, env vars pre-exported from .env before startup can")
	log.Printf("[WARN] shadow config.yaml and silently break features (e.g. market-bot")
	log.Printf("[WARN] control). Delete or rename .env and restart to be sure:")
	log.Printf("[WARN]   mv %s %s.bak", filepath.Join(workDir, ".env"), filepath.Join(workDir, ".env"))
	return true
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
	flag.StringVar(&brokerUser, "broker-user", envOr("BROKER_USER", ""), "AMQP broker username (required for broker features)")
	flag.StringVar(&brokerPass, "broker-pass", envOr("BROKER_PASS", ""), "AMQP broker password (required for broker features)")
	flag.StringVar(&backupDir, "backup-dir", envOr("BACKUP_DIR", ""), "Backup directory path")
	flag.StringVar(&serverIniDir, "ini-dir", envOr("SERVER_INI_DIR", ""), "Directory containing UserGame.ini / UserOverrides.ini")
	flag.BoolVar(&setupMode, "setup", false, "Interactive setup wizard — writes ~/.dune-admin/config.yaml")
	flag.BoolVar(&cleanMarketMode, "clean-market", false, "Delete all bot listings (Revy), then exit")
	flag.StringVar(&sqlQuery, "sql", "", "Run a SQL query and print results to stdout, then exit")
	flag.StringVar(&renderK8SOut, "render-k8s", "", "Render k8s manifest with values from loaded config (path or '-' for stdout)")
	flag.BoolVar(&updateMode, "update", false, "Check for and apply the latest release")
	flag.BoolVar(&reinstallMode, "reinstall", false, "Re-download and reinstall the current latest release (useful for testing updates)")
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

func runSQLMode(query string) error {
	if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
		return fmt.Errorf("connect: %w", msg.err)
	}
	if msg, ok := cmdRunSQL(query)().(msgSQL); ok {
		if msg.err != nil {
			return msg.err
		}
		fmt.Println(msg.result)
	}
	return nil
}

// runCleanMarketMode wipes every active Revy listing then exits. Useful as a
// one-shot operation from cron, AMP, or an admin laptop without having to
// spin up the full HTTP server.
func runCleanMarketMode() error {
	if err := loadItemData(); err != nil {
		return fmt.Errorf("load item data: %w", err)
	}
	if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
		return fmt.Errorf("connect: %w", msg.err)
	}
	defer closeGlobalConnections()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cacheDB, itemDataForBot, _ := resolveEmbeddedMarketBotPaths(loadedConfig, itemDataPath)
	inst, err := marketbot.Run(ctx, marketbot.BotConfig{
		DBPool:       globalDB,
		DBHost:       dbHost,
		DBPort:       dbPort,
		DBUser:       dbUser,
		DBPass:       dbPass,
		DBName:       dbName,
		DBSchema:     dbSchema,
		CacheDB:      cacheDB,
		ItemDataPath: itemDataForBot,
	})
	if err != nil {
		return fmt.Errorf("init market bot: %w", err)
	}
	// Pause immediately so the tick loop spawned by Run does not race the
	// cleanup we are about to perform.
	inst.Pause()

	orders, items, err := inst.CleanupListings(ctx)
	if err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}
	fmt.Printf("market cleanup: deleted %d orders, %d items\n", orders, items)
	return nil
}

func runImmediateModes() (handled bool, err error) {
	// Explicit -setup flag: reconfigure and exit (don't start server).
	if setupMode {
		runSetup()
		return true, nil
	}
	if reinstallMode {
		runSelfUpdate(true)
		return true, nil
	}
	if updateMode {
		runSelfUpdate(false)
		return true, nil
	}
	if sqlQuery != "" {
		return true, runSQLMode(sqlQuery)
	}
	if cleanMarketMode {
		return true, runCleanMarketMode()
	}
	if renderK8SOut != "" {
		return true, renderK8SManifest(renderK8SOut)
	}
	return false, nil
}

func loadRuntimeData() error {
	if err := loadItemData(); err != nil {
		return err
	}
	if err := loadTagsData(); err != nil {
		return err
	}
	return nil
}

func setupIfNeeded() bool {
	// Auto-run setup wizard when no config exists — setup leaves us connected.
	if !needsSetup() {
		return false
	}
	runSetup()
	fmt.Println()
	fmt.Printf("Starting server on %s...\n", listenAddr)
	return true
}

func closeGlobalConnections() {
	if globalDB != nil {
		globalDB.Close()
	}
	if globalSSH != nil {
		_ = globalSSH.Close()
	}
}

func refreshItemTemplates() {
	if msg, ok := cmdFetchItemTemplates().(msgItemTemplates); ok {
		mergeItemTemplates(msg.templates)
	}
}

func connectAndPrimeTemplates(alreadyConnected bool) {
	if alreadyConnected {
		// Already connected by setup; just populate item templates.
		refreshItemTemplates()
		return
	}
	// Connect synchronously (SSH + DB).
	if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
		fmt.Fprintln(os.Stderr, "connect:", msg.err)
		fmt.Fprintln(os.Stderr, "Starting server anyway — use /api/v1/reconnect to retry")
		return
	}
	refreshItemTemplates()
}

func resolveEmbeddedMarketBotPaths(cfg appConfig, fallbackItemDataPath string) (cacheDB string, itemDataForBot string, statePath string) {
	cacheDB = cfg.MarketBotCacheDB
	if cacheDB == "" {
		cacheDB = filepath.Join(configDir(), "market-bot-cache.db")
	}
	itemDataForBot = cfg.MarketBotItemData
	if itemDataForBot == "" {
		if fallbackItemDataPath != "" {
			itemDataForBot = fallbackItemDataPath
		} else {
			itemDataForBot = resolveItemDataPath()
		}
	}
	statePath = cfg.MarketBotState
	if statePath == "" {
		statePath = filepath.Join(configDir(), "market-bot-state.json")
	}
	return cacheDB, itemDataForBot, statePath
}

func startEmbeddedMarketBotIfEnabled(cfg appConfig) context.CancelFunc {
	if !marketBotEnabled(cfg) {
		return nil
	}
	botCtx, botCancel := context.WithCancel(context.Background())
	cacheDB, itemDataForBot, statePath := resolveEmbeddedMarketBotPaths(cfg, itemDataPath)
	inst, err := marketbot.Run(botCtx, marketbot.BotConfig{
		DBPool:       globalDB,
		DBHost:       dbHost,
		DBPort:       dbPort,
		DBUser:       dbUser,
		DBPass:       dbPass,
		DBName:       dbName,
		DBSchema:     dbSchema,
		CacheDB:      cacheDB,
		StatePath:    statePath,
		ItemDataPath: itemDataForBot,
		BuyInterval:  parseDurString(cfg.MarketBotBuyInt, 5*time.Minute),
		ListInterval: parseDurString(cfg.MarketBotListInt, 30*time.Minute),
		BuyThreshold: cfg.MarketBotThresh,
		MaxBuys:      cfg.MarketBotMaxBuys,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "market-bot: startup failed: %v\n", err)
		botCancel()
		return nil
	}
	embeddedBot = inst
	return botCancel
}

func main() {
	flag.Parse()

	handled, err := runImmediateModes()
	if handled {
		if err != nil {
			label := ""
			if renderK8SOut != "" {
				label = "render-k8s: "
			}
			if cleanMarketMode {
				label = "clean-market: "
			}
			fmt.Fprintln(os.Stderr, label+err.Error())
			os.Exit(1)
		}
		return
	}

	if err := loadRuntimeData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	alreadyConnected := setupIfNeeded()
	defer closeGlobalConnections()

	connectAndPrimeTemplates(alreadyConnected)

	if cancel := startEmbeddedMarketBotIfEnabled(loadedConfig); cancel != nil {
		globalBotCancel = cancel
		defer func() {
			if globalBotCancel != nil {
				globalBotCancel()
			}
		}()
	}

	if loadedConfig.MarketBotRemoteURL != "" {
		remoteBotProxy = newRemoteBotClient(loadedConfig.MarketBotRemoteURL, loadedConfig.MarketBotRemoteToken)
	}

	globalWelcomeCancel = startWelcomePackageScanner(loadedConfig)
	defer stopWelcomeScanner()

	startServer(listenAddr)
}

// globalWelcomeCancel stops the welcome-package scanner goroutine on shutdown.
var globalWelcomeCancel context.CancelFunc

// stopWelcomeScanner cancels the welcome-package scanner if it is running.
func stopWelcomeScanner() {
	if globalWelcomeCancel != nil {
		globalWelcomeCancel()
	}
}

// embeddedBot holds the live market bot instance when market_bot_enabled=true.
// Nil when bot is disabled.
var embeddedBot *marketbot.Instance

// globalBotCancel cancels the embedded bot's context, stopping it cleanly.
// Set by startEmbeddedMarketBotIfEnabled; nil when no bot is running.
var globalBotCancel context.CancelFunc

// remoteBotProxy forwards /api/v1/market-bot/* to a remote bot when set.
// Takes precedence when embeddedBot is nil.
var remoteBotProxy *remoteBotClient
