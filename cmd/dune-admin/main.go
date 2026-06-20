// @title dune-admin API
// @version 1.0
// @description Admin panel API for a Dune Awakening private server.
// @description When auth_enabled is set, all /api/v1 endpoints (except /api/v1/auth/*) require the session cookie issued by /api/v1/auth/login or the Discord OAuth flow.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name dune_admin_session

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	_ "time/tzdata" // embed the IANA tz database so time.LoadLocation works on

	// minimal containers without the OS tzdata package (#204: scheduled restart
	// rejected valid zones like "Europe/London").

	_ "dune-admin/docs"

	"gopkg.in/yaml.v3"

	"dune-admin/internal/marketbot"

	"github.com/jackc/pgx/v5/pgxpool"
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
	setPasswordMode bool
	cleanMarketMode bool
	updateMode      bool
	reinstallMode   bool
	sqlQuery        string
	renderK8SOut    string
	sshHost         string
	sshUser         string
	sshKeyPath      string
	sshMode         string
	sshExtraOpts    string
	autoDiscover    bool
	discoverWrite   bool
	itemDataPath    string
	scripCurrencyID int
	dbHost          string
	dbPort          int
	dbUser          string
	dbPass          string
	dbName          string
	dbSchema        string
	listenAddr      string
	addrFlagSet     bool // true iff -addr was explicitly passed on the CLI
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
	// SSHMode selects the SSH transport: "" / "library" uses golang.org/x/crypto/ssh
	// (default); "command" shells out to the OS ssh client (honours ~/.ssh/config,
	// ProxyJump, ssh-agent; never reads a private key).
	SSHMode      string `yaml:"ssh_mode"       json:"ssh_mode"`
	SSHExtraOpts string `yaml:"ssh_extra_opts" json:"ssh_extra_opts"`

	// AutoDiscover: when true, fill empty DB/RMQ/Director fields from the
	// running game-server process args at connect time (command-mode + kubectl).
	AutoDiscover bool `yaml:"auto_discover" json:"auto_discover"`

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
	// AmpContainerRuntime selects the container CLI used for in-container ops
	// (logs/INI/rabbitmqctl) when AmpUseContainer is true: "podman" (default)
	// or "docker". Empty → podman, so existing installs are unaffected.
	AmpContainerRuntime string `yaml:"amp_container_runtime" json:"amp_container_runtime"`
	AmpDataRoot         string `yaml:"amp_data_root"     json:"amp_data_root"`
	// AMP Web API credentials — let dune-admin manage server settings under AMP
	// by writing them through AMP's own config API (Core/SetConfig), so they
	// survive AMP regenerating the game INIs. The API is the instance ADS,
	// reached in-container at 127.0.0.1:<amp_api_port> (default 8081).
	AmpAPIUser  string `yaml:"amp_api_user" json:"amp_api_user"`
	AmpAPIPass  string `yaml:"amp_api_pass" json:"amp_api_pass"`
	AmpAPIPort  int    `yaml:"amp_api_port" json:"amp_api_port"`
	DirectorURL string `yaml:"director_url" json:"director_url"`
	// WebInterfaceHostOverride is an optional per-server host that takes
	// precedence over the SSH host when building auto-discovered Web Interface
	// URLs (issue #234). Useful when the SSH jump host differs from the host
	// that serves the director/file-browser node ports.
	WebInterfaceHostOverride string `yaml:"web_interface_host_override" json:"web_interface_host_override"`

	// Timezone is the server-level IANA tz name (e.g. "America/New_York") used
	// for activity charts, scheduled restarts, and backups. Empty = host-local.
	// When set, it takes precedence over any schedule-level timezone setting.
	Timezone string `yaml:"timezone" json:"timezone"`

	// DB backup tooling (#150). AmpPgBin/AmpPgLib locate the in-container PG17
	// pg_dump/pg_restore + their shared libs; empty → validated AMP defaults.
	// AmpBackupDir is the host dir for dumps; empty → <configDir>/db-backups.
	AmpPgBin     string `yaml:"amp_pg_bin"     json:"amp_pg_bin"`
	AmpPgLib     string `yaml:"amp_pg_lib"     json:"amp_pg_lib"`
	AmpBackupDir string `yaml:"amp_backup_dir" json:"amp_backup_dir"`

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

	// ── Discord bot ────────────────────────────────────────────────────────
	// DiscordBotEnabled starts the embedded Discord gateway bot. Pointer so we
	// can distinguish "unset" (default-off) from "explicitly false".
	DiscordBotEnabled *bool  `yaml:"discord_bot_enabled"          json:"discord_bot_enabled"`
	DiscordBotToken   string `yaml:"discord_bot_token"            json:"discord_bot_token"`
	DiscordGuildID    string `yaml:"discord_guild_id"             json:"discord_guild_id"`
	// Comma-separated Discord role IDs for each capability tier.
	DiscordRolesViewer       string `yaml:"discord_roles_viewer"         json:"discord_roles_viewer"`
	DiscordRolesEconomy      string `yaml:"discord_roles_economy"        json:"discord_roles_economy"`
	DiscordRolesAdmin        string `yaml:"discord_roles_admin"          json:"discord_roles_admin"`
	DiscordAnnounceChannelID string `yaml:"discord_announce_channel_id"  json:"discord_announce_channel_id"`

	// ── Discord status embed (#188) ────────────────────────────────────────
	// A persistent, auto-updating embed posted to one channel that the bot edits
	// in place every interval. Pointer so "unset" (default-off) is distinct from
	// explicit false. The posted message ID is persisted in the unified SQLite
	// store (meta table) so restarts edit the same message instead of re-posting.
	DiscordStatusEnabled         *bool  `yaml:"discord_status_enabled"          json:"discord_status_enabled"`
	DiscordStatusChannelID       string `yaml:"discord_status_channel_id"        json:"discord_status_channel_id"`
	DiscordStatusIntervalSeconds int    `yaml:"discord_status_interval_seconds"  json:"discord_status_interval_seconds"`

	// ── Welcome package ────────────────────────────────────────────────────
	// Auto-grants a configured item package to every player once, on first
	// login. Defaults OFF — it mutates every player's inventory, so it must be
	// explicitly opted into. Bump the version to re-issue to everyone.
	WelcomePackageEnabled       *bool            `yaml:"welcome_package_enabled"            json:"welcome_package_enabled"`
	WelcomePackageScanSecs      int              `yaml:"welcome_package_scan_interval_secs" json:"welcome_package_scan_interval_secs"`
	WelcomePackageActiveVersion string           `yaml:"welcome_package_active_version"     json:"welcome_package_active_version"`
	WelcomePackages             []welcomePackage `yaml:"welcome_packages"                   json:"welcome_packages"`
	// Legacy pre-library fields, migrated into WelcomePackages on load.
	WelcomePackageVersion string               `yaml:"welcome_package_version,omitempty" json:"welcome_package_version,omitempty"`
	WelcomePackageItems   []welcomePackageItem `yaml:"welcome_package_items,omitempty"   json:"welcome_package_items,omitempty"`

	// ── Live events engine ─────────────────────────────────────────────────
	// EventsEnabled starts the background polling engine. Per-event poll_seconds
	// and jitter_seconds are configured on each event definition.
	EventsEnabled *bool `yaml:"events_enabled" json:"events_enabled"`

	// ── Dashboard authentication ──────────────────────────────────────────
	// AuthEnabled turns on login enforcement for the dashboard + API.
	// Pointer so "unset" (default-off) is distinguishable from explicit false.
	// When off, behavior is identical to releases without auth.
	AuthEnabled *bool `yaml:"auth_enabled" json:"auth_enabled"`
	// Local username/password login. The hash is bcrypt; set it via
	// `dune-admin --set-password` or the config UI (which sends the plaintext
	// in auth_local_password_new and never stores it).
	AuthLocalUsername     string `yaml:"auth_local_username"      json:"auth_local_username"`
	AuthLocalPasswordHash string `yaml:"auth_local_password_hash" json:"auth_local_password_hash"`
	// AuthLocalPasswordNew is a write-only API field: when present in a saved
	// config it is bcrypt-hashed into AuthLocalPasswordHash and discarded.
	AuthLocalPasswordNew string `yaml:"-" json:"auth_local_password_new,omitempty"`
	// Discord OAuth2 login (BYOA). Reuses discord_bot_token + discord_guild_id
	// for the membership/role lookup after the OAuth exchange.
	AuthDiscordEnabled      *bool  `yaml:"auth_discord_enabled"       json:"auth_discord_enabled"`
	AuthDiscordClientID     string `yaml:"auth_discord_client_id"     json:"auth_discord_client_id"`
	AuthDiscordClientSecret string `yaml:"auth_discord_client_secret" json:"auth_discord_client_secret"`
	// AuthDiscordRedirectURL overrides the OAuth redirect; empty derives
	// scheme://host/api/v1/auth/discord/callback from each request.
	AuthDiscordRedirectURL string `yaml:"auth_discord_redirect_url" json:"auth_discord_redirect_url"`
	// Owners get full capabilities regardless of the permissions matrix.
	// The guild owner (via Discord API) and the local account are always
	// owners; these comma-separated lists add more.
	AuthOwnerDiscordIDs string `yaml:"auth_owner_discord_ids" json:"auth_owner_discord_ids"`
	AuthOwnerRoleIDs    string `yaml:"auth_owner_role_ids"    json:"auth_owner_role_ids"`
	AuthSessionTTLHours int    `yaml:"auth_session_ttl_hours" json:"auth_session_ttl_hours"`
	// AuthGuestEnabled allows anyone to start a read-only "guest" session
	// from the login page — no credentials, default read-only capabilities.
	AuthGuestEnabled *bool `yaml:"auth_guest_enabled" json:"auth_guest_enabled"`
	// AuthCookieSameSite: "" → Lax; "none" for cross-origin CDN setups (TLS only).
	AuthCookieSameSite string `yaml:"auth_cookie_samesite" json:"auth_cookie_samesite"`

	// ── Battlepass ─────────────────────────────────────────────────────────
	// BattlepassEnabled starts the tier-evaluation loop (default off).
	// BattlepassAwardPast rewards pre-existing progress on first evaluation;
	// default off — old progress is baselined and only new unlocks earn intel.
	// IMPORTANT: set award_past=true on the FIRST scan for an account — if
	// an account is first scanned with award_past=false, its tiers are written
	// as "baseline" permanently and flipping award_past later has no effect.
	// To repopulate Pending after changing modes, clear the battlepass_* tables
	// in dune-admin.db (or delete the whole file to reset all stores).
	// BattlepassScanPaceMs is the inter-player delay (ms) during evaluation;
	// 0 disables pacing; negative → default 75ms; max 5000ms.
	// BattlepassScanStartDelayMs is the delay (ms) before the boot scan so the
	// server and DB pool finish warming; 0 is immediate; negative → 3000ms.
	// BattlepassAutoGrant (default off) records each newly-earned tier into a
	// deferred-grant ledger and delivers it automatically — to online AND
	// offline players — retrying with backoff up to deferredGrantMaxAttempts.
	// After exhaustion the tier reverts to manual-grant only.
	BattlepassEnabled          *bool `yaml:"battlepass_enabled"            json:"battlepass_enabled"`
	BattlepassAwardPast        *bool `yaml:"battlepass_award_past"         json:"battlepass_award_past"`
	BattlepassAutoGrant        *bool `yaml:"battlepass_auto_grant"         json:"battlepass_auto_grant"`
	BattlepassPollSeconds      int   `yaml:"battlepass_poll_seconds"       json:"battlepass_poll_seconds"`
	BattlepassScanPaceMs       int   `yaml:"battlepass_scan_pace_ms"       json:"battlepass_scan_pace_ms"`
	BattlepassScanStartDelayMs int   `yaml:"battlepass_scan_start_delay_ms" json:"battlepass_scan_start_delay_ms"`

	// Multi-server: when Servers is non-empty, connectAll is replaced by
	// connectMultiServer which calls connectServer for each entry. DefaultServer
	// names the initially-active server; if blank the first entry is used.
	// Existing flat-config installs (no servers: key) are unaffected.
	Servers       []ServerConfig `yaml:"servers"        json:"servers,omitempty"`
	DefaultServer string         `yaml:"default_server" json:"default_server,omitempty"`
	// DefaultServerName is the display name of the legacy flat "default" server,
	// so single-server installs can rename it. Blank → "Default".
	DefaultServerName string `yaml:"default_server_name" json:"default_server_name,omitempty"`
}

// defaultServerName returns the configured display name for the legacy flat
// "default" server, falling back to "Default".
func defaultServerName() string {
	if loadedConfig.DefaultServerName != "" {
		return loadedConfig.DefaultServerName
	}
	return "Default"
}

// marketBotEnabled returns the effective bot-enabled flag. Nil/unset → off
// (explicit opt-in required). This matches serverMarketBotEnabled semantics so
// a nil value never silently starts the bot during a config round-trip.
func marketBotEnabled(cfg appConfig) bool {
	return cfg.MarketBotEnabled != nil && *cfg.MarketBotEnabled
}

// startWelcomePackageScanner opens the ledger store, seeds the live runtime
// config, and starts the scanner goroutine. The goroutine always runs so the
// feature can be toggled on at runtime via the API; each tick is a cheap no-op
// while disabled. Returns a cancel func, or nil if the store could not open.
func startWelcomePackageScanner(_ appConfig) context.CancelFunc {
	var store *welcomeStore
	if globalStore != nil {
		store = newWelcomeStore(globalStore, defaultServerID)
	} else {
		var err error
		store, err = openWelcomeStore(filepath.Join(configDir(), "welcome-package.db"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "welcome-package: store open failed: %v\n", err)
			return nil
		}
	}
	welcomeStoreDB = store

	// Load runtime from the DB store; seeds from YAML on first boot (migration).
	// Skip on a fresh install (no server row) — the FK constraint would reject
	// the INSERT; seeding runs on the next call once a server has been added.
	if !noServerConfigured() {
		if err := applyWelcomeConfigFromStore(); err != nil {
			fmt.Fprintf(os.Stderr, "welcome-package: config load failed: %v\n", err)
		}
	}

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
			setEnvIfMissing("SSH_MODE", cfg.SSHMode)
			setEnvIfMissing("SSH_EXTRA_OPTS", cfg.SSHExtraOpts)
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
			// LISTEN_ADDR is intentionally NOT seeded from config.yaml here.
			// After the one-time import the DB is the authoritative source for the
			// listen address; resolveListenAddr() reads it at bind time so the
			// process survives config.yaml deletion without reverting to :8080.
			setEnvIfMissing("CONTROL", cfg.Control)
			setEnvIfMissing("CONTROL_NAMESPACE", cfg.ControlNamespace)
			setEnvIfMissing("BROKER_GAME_ADDR", cfg.BrokerGameAddr)
			setEnvIfMissing("BROKER_ADMIN_ADDR", cfg.BrokerAdminAddr)
			setEnvIfMissing("BROKER_USER", cfg.BrokerUser)
			setEnvIfMissing("BROKER_PASS", cfg.BrokerPass)
			setEnvIfMissing("BROKER_JWT_SECRET", cfg.BrokerJWTSecret)
			setEnvIfMissing("BACKUP_DIR", cfg.BackupDir)
			setEnvIfMissing("SERVER_INI_DIR", cfg.ServerIniDir)
			setEnvIfMissing("DISCORD_BOT_TOKEN", cfg.DiscordBotToken)
			setEnvIfMissing("DISCORD_GUILD_ID", cfg.DiscordGuildID)
			setEnvIfMissing("DISCORD_ROLES_VIEWER", cfg.DiscordRolesViewer)
			setEnvIfMissing("DISCORD_ROLES_ECONOMY", cfg.DiscordRolesEconomy)
			setEnvIfMissing("DISCORD_ROLES_ADMIN", cfg.DiscordRolesAdmin)
			setEnvIfMissing("DISCORD_ANNOUNCE_CHANNEL_ID", cfg.DiscordAnnounceChannelID)
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
	componentLog("main").Warn().
		Str("dir", workDir).
		Str("config_path", configPath()).
		Msg("stale .env file found; .env is ignored but pre-exported env vars can shadow config.yaml and silently break features (e.g. market-bot control) — delete or rename .env and restart")
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
	flag.StringVar(&sshMode, "ssh-mode", envOr("SSH_MODE", ""), "SSH transport: library (default, x/crypto/ssh) | command (OS ssh client)")
	flag.StringVar(&sshExtraOpts, "ssh-extra-opts", envOr("SSH_EXTRA_OPTS", ""), "Extra ssh -o options for command mode, space-separated")
	flag.BoolVar(&autoDiscover, "auto-discover", loadedConfig.AutoDiscover, "Discover DB/RMQ/Director from running game-server args (command-mode + kubectl)")
	flag.BoolVar(&discoverWrite, "discover-write", false, "Persist discovered values into config.yaml, then continue")
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
	flag.BoolVar(&setPasswordMode, "set-password", false, "Set the local dashboard login username/password, then exit (auth lockout recovery)")
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
	return discoverSSHKeyPath()
}

// effectiveListenAddr resolves the HTTP bind address. Precedence:
//  1. explicit -addr CLI flag or LISTEN_ADDR env var (operator override)
//  2. DB-persisted value (dbAddr) — source of truth after first-boot import;
//     survives config.yaml deletion without reverting to the :8080 default
//  3. built-in default carried in flagAddr (":8080" from the flag definition)
func effectiveListenAddr(flagAddr string, explicit bool, dbAddr string) string {
	if explicit {
		return flagAddr
	}
	if dbAddr != "" {
		return dbAddr
	}
	return flagAddr
}

// resolveListenAddr returns the address to bind the HTTP server to.
// Called at startup after hydrateConfigFromStore() has populated loadedConfig.ListenAddr.
func resolveListenAddr() string {
	_, envSet := os.LookupEnv("LISTEN_ADDR")
	return effectiveListenAddr(listenAddr, addrFlagSet || envSet, loadedConfig.ListenAddr)
}

// discoverSSHKeyPath searches the standard locations (config dir, next to the
// binary, the working directory) for an "sshKey" file and returns the first hit.
// When none exists it returns the config-dir path so callers have a stable
// default. Independent of the sshKeyPath global so it works for per-server config.
func discoverSSHKeyPath() string {
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

// firstExistingPath returns the first path from candidates where os.Stat
// succeeds, or "" if none exist. It is the shared search-order primitive
// for all data-file resolvers.
func firstExistingPath(candidates []string) string {
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// resolveDataFilePath returns the path to the named data file by searching
// the standard candidate locations in priority order:
//  1. ~/.dune-admin/<name>   — user override
//  2. <exeDir>/<name>        — next to the binary (release zip / /app/)
//  3. <exeDir>/../share/dune-admin/<name> — Homebrew pkgshare
//  4. ./<name>               — cwd (dev / make dev)
func resolveDataFilePath(name string) string {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	return firstExistingPath([]string{
		filepath.Join(home, ".dune-admin", name),
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "share", "dune-admin", name), // Homebrew pkgshare
		"./" + name,
	})
}

func resolveItemDataPath() string {
	if itemDataPath != "" {
		return itemDataPath
	}
	return resolveDataFilePath("item-data.json")
}

func resolveTagsDataPath() string {
	return resolveDataFilePath("tags-data.json")
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
		componentLog("main").Warn().Msg("item-data.json not found — using raw names for autocomplete")
		itemData = itemDataFile{
			Names: make(map[string]string),
			Items: make(map[string]itemRule),
		}
		return nil
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
	originalNames := make(map[string]string, len(parsed.Names))
	for k, v := range parsed.Names {
		lk := strings.ToLower(k)
		normalizedNames[lk] = v
		originalNames[lk] = k
	}
	parsed.Names = normalizedNames
	parsed.OriginalNames = originalNames
	itemData = parsed
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

// needsSetupConfigured reports whether a configured install still needs setup.
// auto_discover suppresses the (library-wired) wizard only when discovery is
// actually applicable — the kubectl control plane, where it fills the DB
// password from the running game server. Outside kubectl, an empty db_pass
// still requires setup, so an unreachable/failed discovery doesn't strand the
// operator without a wizard path.
func needsSetupConfigured() bool {
	// DB is the source of truth: any configured server means no setup needed.
	if globalServersStore != nil {
		if has, err := globalServersStore.hasAnyServer(); err == nil {
			return !has
		}
	}
	// Fallback (store unavailable): the legacy flat-config heuristic.
	if len(loadedConfig.Servers) > 0 {
		return false
	}
	if dbPass != "" {
		return false
	}
	return !autoDiscover || resolveControl() != "kubectl"
}

func needsSetup() bool {
	// DB is the source of truth: a server configured in the store means the
	// install is set up, regardless of whether config.yaml still exists.
	if globalServersStore != nil {
		if has, err := globalServersStore.hasAnyServer(); err == nil {
			return !has
		}
	}
	// Fallback (store unavailable): config.yaml takes priority over legacy .env.
	if _, err := os.Stat(configPath()); err == nil {
		return needsSetupConfigured()
	}
	if _, err := os.Stat(".env"); err == nil {
		return needsSetupConfigured()
	}
	return true
}

func runSQLMode(query string) error {
	if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
		return fmt.Errorf("connect: %w", msg.err)
	}
	if msg, ok := cmdRunSQL(globalDB, query)().(msgSQL); ok {
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
	itemDataForBot = usableItemDataPath(itemDataForBot)
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
	if setPasswordMode {
		return true, runSetPasswordMode()
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
	// When setup is needed, skip the connect step — the web wizard handles
	// initial configuration via POST /api/v1/config + POST /api/v1/reconnect.
	// The terminal wizard is still available via --setup / make setup.
	return needsSetup()
}

func closeGlobalConnections() {
	// Close all per-server pools (dedup by pointer so aliased pools close once).
	seen := make(map[*pgxpool.Pool]bool)
	for _, sc := range globalRegistry.All() {
		if sc.DB != nil && !seen[sc.DB] {
			seen[sc.DB] = true
			sc.DB.Close()
		}
	}
	// Fallback for the legacy path where globalDB may not be in the registry
	// (connect failed before registration, or the registry is empty).
	if globalDB != nil && !seen[globalDB] {
		globalDB.Close()
	}
	// Close the active executor so command-mode tears down its ControlMaster
	// (ssh -O exit) on shutdown. For the library executor this also closes its
	// underlying *ssh.Client (globalSSH), so drop that reference to avoid a
	// double Close on it below.
	if globalExecutor != nil {
		globalExecutor.Close()
		globalSSH = nil
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
	if len(loadedConfig.Servers) > 0 {
		// Multi-server path: connect each server from the servers: list.
		if err := connectMultiServer(loadedConfig); err != nil {
			fmt.Fprintln(os.Stderr, "connect:", err)
			fmt.Fprintln(os.Stderr, "Starting server anyway — use /api/v1/servers/{id}/reconnect to retry")
		}
	} else {
		// Legacy flat-config path.
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "connect:", msg.err)
			fmt.Fprintln(os.Stderr, "Starting server anyway — use /api/v1/reconnect to retry")
			return
		}
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

// itemDataPathResolvable reports whether path points to (or contains) a readable
// item-data.json, mirroring marketbot.loadCatalog's resolution (a directory is
// resolved to item-data.json inside it).
func itemDataPathResolvable(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "item-data.json")
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// usableItemDataPath returns configured if it resolves to a readable
// item-data.json; otherwise it falls back to the standard search locations so a
// stale or mistyped market_bot_item_data (e.g. "optional", #136) doesn't crash
// bot startup with a cryptic "open <path>" error. If item-data.json can't be
// found anywhere either, the original value is returned so loadCatalog surfaces
// a clear not-found error.
func usableItemDataPath(configured string) string {
	if itemDataPathResolvable(configured) {
		return configured
	}
	if fb := resolveItemDataPath(); itemDataPathResolvable(fb) {
		if configured != "" {
			componentLog("main").Warn().
				Str("configured_path", configured).
				Str("fallback_path", fb).
				Msg("market-bot item-data path not found; falling back")
		}
		return fb
	}
	return configured
}

// immediateModeLabel returns the error prefix for immediate-mode failures
// (render-k8s, clean-market) so the logged error is clearly attributed.
func immediateModeLabel() string {
	if renderK8SOut != "" {
		return "render-k8s: "
	}
	if cleanMarketMode {
		return "clean-market: "
	}
	return ""
}

func main() {
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrFlagSet = true
		}
	})
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run wires up and starts the server, returning an error instead of calling
// os.Exit/log.Fatal so that every deferred cleanup below unwinds on every return
// path — including the server's exit. main() translates a returned error into a
// non-zero exit code only after run() has fully returned and its defers ran.
func run(ctx context.Context) error {
	initLogging()
	handled, err := runImmediateModes()
	if handled {
		if err != nil {
			return fmt.Errorf("%s%w", immediateModeLabel(), err)
		}
		return nil
	}

	if err := loadRuntimeData(); err != nil {
		return err
	}

	// Open the unified store BEFORE connecting: servers + global settings now
	// live in the DB. hydrateConfigFromStore imports config.yaml once (first
	// boot) and loads servers/settings into loadedConfig so the connect path and
	// needsSetup() see DB-sourced state.
	closeStore := initUnifiedStoreOnce()
	defer closeStore()
	hydrateConfigFromStore()

	// Read caches must exist before the connect path / any handler. Non-fatal:
	// on error the handlers serve live data.
	if err := initGlobalCaches(); err != nil {
		componentLog("main").Warn().Err(err).Msg("init caches failed; serving live")
	}

	alreadyConnected := setupIfNeeded()
	defer closeGlobalConnections()

	connectAndPrimeTemplates(alreadyConnected)

	// One-time copy of legacy Postgres dune.discord_links registrations into the
	// SQLite discord_user_links store. Runs here (post-connect) because it needs
	// the default server's live pool; best-effort + marker-gated, so it retries on
	// a later boot if Postgres is unreachable now.
	if id, ok := firstServerID(); ok && globalStore != nil {
		var read legacyLinkReader
		if globalDB != nil {
			read = func(c context.Context) ([]legacyUserLink, error) {
				return cmdReadLegacyDiscordLinks(c, globalDB)
			}
		}
		migrateLegacyDiscordUserLinks(globalStore, id, read)
	}

	// Prewarm hot read caches so the first UI paint (dashboard health) is instant
	// on a hard refresh, then keep them warm in the background (refresh-ahead).
	warmer := newCacheWarmer(globalRegistry)
	prewarmCaches(ctx, warmer)
	go warmer.run(ctx)

	sessionCancel := startSessionTracking()
	defer sessionCancel()

	// Start one market bot per enabled server that has a live DB. Servers with
	// the toggle off, or with no DB yet (fresh/unconfigured env), don't start a
	// bot — so a brand-new install no longer spams "market-bot startup failed".
	restartAllServerMarketBots(loadedConfig)
	defer stopAllServerMarketBots()

	if loadedConfig.MarketBotRemoteURL != "" {
		remoteBotProxy = newRemoteBotClient(loadedConfig.MarketBotRemoteURL, loadedConfig.MarketBotRemoteToken)
	}

	applyDiscordConfig(loadedConfig)
	defer stopDiscordBot()
	defer stopDiscordStatusLoop()

	globalWelcomeCancel = startWelcomePackageScanner(loadedConfig)
	defer stopWelcomeScanner()

	startBackgroundServices(ctx)

	applyEventEngine(loadedConfig)
	defer stopEventEngine()

	applyBattlepassEngine(loadedConfig)
	defer stopBattlepassEngine()

	restartAllServerLandsraadBots()
	defer stopAllServerLandsraadBots()

	return startServer(resolveListenAddr())
}

// startBackgroundServices launches the process-lifetime schedulers and loads the
// operator-configured runtime data that has no teardown. The schedulers honour
// ctx.Done(); today ctx is context.Background() (the process is hard-stopped on
// signal), but threading ctx keeps this forward-compatible with graceful
// shutdown without changing current behaviour.
func startBackgroundServices(ctx context.Context) {
	// Scheduled restarts (#145): per-server schedules live in the DB; the
	// scheduler iterates the registry each tick. Runs for the process lifetime
	// (independent of the welcome scanner's lifecycle).
	go runRestartScheduler(ctx)

	// Scheduled DB backups (#150): same lifecycle as scheduled restarts.
	go runBackupScheduler(ctx)

	// Web interfaces (#155): load the operator-configured Server Health links.
	loadWebInterfaces()

	initLocationStore()
	initGivePacksStore()
	initEventStore()
	initBattlepassStore()
}

// initLocationStore opens (or creates) the persistent location store and sets
// globalLocationStore. A failure is non-fatal — the store guard in each handler
// surfaces a 503 for the affected endpoints.
func initLocationStore() {
	var s *locationStore
	if globalStore != nil {
		s = newLocationStore(globalStore)
		if err := s.seedIfEmpty(); err != nil {
			fmt.Fprintf(os.Stderr, "location store seed: %v\n", err)
		}
	} else {
		var err error
		s, err = openLocationStore(filepath.Join(configDir(), "locations.db"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "location store: %v (using empty list)\n", err)
			return
		}
	}
	globalLocationStore = s
}

// initGivePacksStore opens (or creates) the give-packs SQLite store and seeds
// it from the embedded default packs.json snapshot on first boot. A failure is
// non-fatal — handlers guard for a nil store and return 503.
func initGivePacksStore() {
	var s *givePacksStore
	if globalStore != nil {
		s = newGivePacksStore(globalStore, defaultServerID)
	} else {
		var err error
		s, err = openGivePacksStore(filepath.Join(configDir(), "give-packs.db"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "give-packs store: %v (using empty packs)\n", err)
			return
		}
	}
	givePacksStoreDB = s
	// Skip on a fresh install (no server row) — the FK constraint would reject
	// the INSERT; seeding runs on the next startup once a server has been added.
	if noServerConfigured() {
		return
	}
	loaded, _, ok, err := s.loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "give-packs store load: %v\n", err)
		return
	}
	// Seed from the embedded default snapshot on first boot or when the flag
	// was never set. Once base_packs_loaded=true, no seeding ever happens again.
	if !ok || !loaded {
		if seedErr := seedGivePacks(); seedErr != nil {
			fmt.Fprintf(os.Stderr, "give-packs seed: %v\n", seedErr)
		}
	}
}

// initEventStore opens (or creates) the events SQLite store and sets
// globalEventStore. A failure is non-fatal — handlers guard for a nil store.
func initEventStore() {
	if globalStore != nil {
		globalEventStore = newEventStore(globalStore, defaultServerID)
		return
	}
	var err error
	s, err := openEventStore(filepath.Join(configDir(), "events.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "event store: %v (events disabled)\n", err)
		return
	}
	globalEventStore = s
}

// initBattlepassStore opens (or creates) the battlepass SQLite store and sets
// globalBattlepassStore. A failure is non-fatal — handlers guard for nil.
func initBattlepassStore() {
	if globalStore != nil {
		globalBattlepassStore = newBattlepassStore(globalStore, defaultServerID)
		return
	}
	s, err := openBattlepassStore(filepath.Join(configDir(), "battlepass.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "battlepass store: %v (battlepass disabled)\n", err)
		return
	}
	globalBattlepassStore = s
}

// globalWelcomeCancel stops the welcome-package scanner goroutine on shutdown.
var globalWelcomeCancel context.CancelFunc

// stopWelcomeScanner cancels the welcome-package scanner if it is running.
func stopWelcomeScanner() {
	if globalWelcomeCancel != nil {
		globalWelcomeCancel()
	}
}

// remoteBotProxy forwards /api/v1/market-bot/* to a remote bot when set, and is
// the fallback when no per-server embedded bot is running. The embedded bots are
// per-server (ServerContext.Bot).
var remoteBotProxy *remoteBotClient
