package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

const masked = "••••••••"

// handleGetConfig returns the current config with all secret fields masked.
//
// @Summary Get current runtime configuration (secrets masked)
// @Tags config
// @Produce json
// @Success 200 {object} appConfig
// @Router /api/v1/config [get]
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		jsonOK(w, buildCurrentConfig())
		return
	}
	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		jsonErr(w, fmt.Errorf("parse config: %w", err), 500)
		return
	}
	maskSecrets(&cfg)
	jsonOK(w, cfg)
}

// maskSecrets replaces all secret fields with the display placeholder.
func maskSecrets(cfg *appConfig) {
	if cfg.DBPass != "" {
		cfg.DBPass = masked
	}
	if cfg.BrokerPass != "" {
		cfg.BrokerPass = masked
	}
	if cfg.BrokerJWTSecret != "" {
		cfg.BrokerJWTSecret = masked
	}
	if cfg.MarketBotRemoteToken != "" {
		cfg.MarketBotRemoteToken = masked
	}
	if cfg.AmpAPIPass != "" {
		cfg.AmpAPIPass = masked
	}
	if cfg.DiscordBotToken != "" {
		cfg.DiscordBotToken = masked
	}
	if cfg.AuthDiscordClientSecret != "" {
		cfg.AuthDiscordClientSecret = masked
	}
	if cfg.AuthLocalPasswordHash != "" {
		cfg.AuthLocalPasswordHash = masked
	}
	// Per-server entries carry their own secrets — mask them too so plaintext
	// passwords never reach the client.
	for i := range cfg.Servers {
		maskServerSecrets(&cfg.Servers[i])
	}
}

// preserveMaskedSecrets restores real secret values when the client sent back
// the display placeholder. Falls back to loadedConfig when the file is
// unreadable so in-memory secrets survive a mid-session config file move.
func preserveMaskedSecrets(
	cfg *appConfig,
	readFile func(string) ([]byte, error),
	path string,
) {
	needsRestore := cfg.DBPass == masked ||
		cfg.BrokerPass == masked ||
		cfg.BrokerJWTSecret == masked ||
		cfg.MarketBotRemoteToken == masked ||
		cfg.AmpAPIPass == masked ||
		cfg.DiscordBotToken == masked ||
		cfg.AuthDiscordClientSecret == masked ||
		cfg.AuthLocalPasswordHash == masked

	if !needsRestore {
		return
	}

	old := loadedConfig
	if data, err := readFile(path); err == nil {
		_ = yaml.Unmarshal(data, &old)
	}
	// dbPass global may differ from loadedConfig when set from env var
	if old.DBPass == "" {
		old.DBPass = dbPass
	}

	if cfg.DBPass == masked {
		cfg.DBPass = old.DBPass
	}
	if cfg.BrokerPass == masked {
		cfg.BrokerPass = old.BrokerPass
	}
	if cfg.BrokerJWTSecret == masked {
		cfg.BrokerJWTSecret = old.BrokerJWTSecret
	}
	if cfg.MarketBotRemoteToken == masked {
		cfg.MarketBotRemoteToken = old.MarketBotRemoteToken
	}
	if cfg.AmpAPIPass == masked {
		cfg.AmpAPIPass = old.AmpAPIPass
	}
	if cfg.DiscordBotToken == masked {
		cfg.DiscordBotToken = old.DiscordBotToken
	}
	if cfg.AuthDiscordClientSecret == masked {
		cfg.AuthDiscordClientSecret = old.AuthDiscordClientSecret
	}
	if cfg.AuthLocalPasswordHash == masked {
		cfg.AuthLocalPasswordHash = old.AuthLocalPasswordHash
	}
}

func writeConfigFile(cfg appConfig) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(configPath(), data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func resetRuntimeConnections() {
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}
	if globalExecutor != nil {
		globalExecutor.Close()
		globalExecutor = nil
	}
	globalSSH = nil
	globalControl = nil
}

// @Summary Save configuration and reconnect
// @Tags config
// @Accept json
// @Produce json
// @Param config body appConfig true "Updated configuration"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/config [post]
func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var cfg appConfig
	if err := decode(r, &cfg); err != nil {
		jsonErr(w, fmt.Errorf("decode: %w", err), 400)
		return
	}

	preserveMaskedSecrets(&cfg, os.ReadFile, configPath())

	// Per-server config (Servers[] / default_server) is owned exclusively by the
	// /api/v1/servers endpoints. A global-settings save must never wipe or mutate
	// it — the POST /config payload carries only global + flat fields.
	cfg.Servers = loadedConfig.Servers
	cfg.DefaultServer = loadedConfig.DefaultServer

	// Fresh-setup / legacy single-server path: gap-fill blank connection fields
	// with control-plane defaults so leaving a field empty in the wizard uses the
	// default (mirroring the console wizard) instead of wiping it via applyConfig.
	// Multi-server installs don't use the flat connection fields, so skip them.
	if len(cfg.Servers) == 0 {
		applyFlatConnectionDefaults(&cfg)
	}

	if err := applyNewLocalPassword(&cfg); err != nil {
		jsonErr(w, err, 500)
		return
	}

	if err := writeConfigFile(cfg); err != nil {
		jsonErr(w, err, 500)
		return
	}

	// Capture the auth-enabled state before applyConfig overwrites loadedConfig,
	// so we can force a clean slate when it is toggled.
	wasAuthEnabled := authEnabled(loadedConfig)

	// Multi-server installs keep their per-server DB/control connections in the
	// registry (managed by the /servers endpoints). Reconnecting here would close
	// the active server's pool — which is aliased to globalDB — leaving the
	// registry holding a dead pool, and would register a spurious flat "default"
	// server. So only the legacy flat install reconnects on a global save.
	multiServer := len(cfg.Servers) > 0

	applyConfig(cfg)

	// Re-initialize auth state so enabling auth (or adding Discord login)
	// via the UI takes effect without a process restart. Disabling auth is
	// honored immediately too — the middleware checks loadedConfig per request.
	initAuthRuntime(cfg)

	if !multiServer {
		// Stop the running bots before closing the DB pool. A running bot holds a
		// reference to the pool; if we close it first the bot's next tick would
		// use a closed pool.
		stopAllServerMarketBots()

		resetRuntimeConnections()

		// Reconnect is best-effort — config is already written to disk.
		// If reconnect fails (e.g. SSH not yet reachable), the file is still
		// saved and will take effect on the next restart or manual reconnect.
		if err := connectAll(); err != nil {
			log.Printf("handleSaveConfig: reconnect after save: %v", err)
		}
	}

	// Restart every server's market bot AFTER reconnect so each picks up the new
	// global tuning (intervals/thresholds) against its live pool. Then refresh
	// the remote-bot proxy from the new config.
	restartAllServerMarketBots(loadedConfig)
	applyMarketBotConfig(cfg)
	// Discord connects outbound (no dependency on globalDB) so restart it last
	// to pick up any token/guild/role changes without requiring a process restart.
	applyDiscordConfig(cfg)

	// When auth is toggled on or off, drop the caller's session cookie so the
	// SPA re-evaluates from a clean slate (login page when enabling, normal app
	// when disabling) rather than acting on a stale session.
	clearSessionOnAuthToggle(w, r, wasAuthEnabled, authEnabled(cfg))

	handleStatus(w, r)
}

// clearSessionOnAuthToggle expires the session cookie when the auth-enabled
// flag changes state during a config save. No-op when the flag is unchanged.
func clearSessionOnAuthToggle(w http.ResponseWriter, r *http.Request, wasEnabled, nowEnabled bool) {
	if wasEnabled != nowEnabled {
		clearSessionCookie(w, r)
	}
}

// buildCurrentConfig constructs an appConfig from the current global vars.
func buildCurrentConfig() appConfig {
	dbPassOut := ""
	if dbPass != "" {
		dbPassOut = masked
	}
	return appConfig{
		SSHHost:          sshHost,
		SSHUser:          sshUser,
		SSHKey:           sshKeyPath,
		DBHost:           dbHost,
		DBPort:           dbPort,
		DBUser:           dbUser,
		DBPass:           dbPassOut,
		DBName:           dbName,
		DBSchema:         dbSchema,
		Control:          controlPlane,
		ControlNamespace: controlNS,
		BrokerGameAddr:   brokerGameAddr,
		BrokerAdminAddr:  brokerAdminAddr,
		BrokerTLS:        brokerTLS,
		BackupDir:        backupDir,
		ListenAddr:       listenAddr,
		ScripCurrency:    scripCurrencyID,
	}
}

// applyMarketBotConfig refreshes the remote-bot proxy from the new global config.
// The embedded bots themselves are per-server and (re)started via
// restartAllServerMarketBots / the /servers endpoints, not here.
func applyMarketBotConfig(cfg appConfig) {
	if cfg.MarketBotRemoteURL != "" {
		remoteBotProxy = newRemoteBotClient(cfg.MarketBotRemoteURL, cfg.MarketBotRemoteToken)
	} else {
		remoteBotProxy = nil
	}
}

// handleDiscover runs auto-discovery on demand and, when persist=true, writes
// the gap-filled values into config.yaml (then applies them). Requires an
// active executor (command-mode/kubectl).
func handleDiscover(w http.ResponseWriter, r *http.Request) {
	exec := executorFromCtx(r)
	if exec == nil {
		jsonErr(w, fmt.Errorf("no executor connected"), http.StatusServiceUnavailable)
		return
	}
	g, err := discoverGameConfig(exec)
	if err != nil {
		jsonErr(w, err, http.StatusBadGateway)
		return
	}
	var gameIP, adminIP, directorIP string
	ctrl := controlFromCtx(r)
	if ctrl != nil && ctrl.Name() == "kubectl" {
		pods := fetchClusterPodIPs(exec)
		gameIP = podIPByPattern(pods, "mq-game")
		adminIP = podIPByPattern(pods, "mq-admin")
		directorIP = podIPByPattern(pods, "bgd")
	}
	cfg := persistDiscoveredConfig(loadedConfig, g, gameIP, adminIP, directorIP)
	if r.URL.Query().Get("persist") == "true" {
		if err := writeConfigFile(cfg); err != nil {
			jsonErr(w, fmt.Errorf("write config: %w", err), http.StatusInternalServerError)
			return
		}
		loadedConfig = cfg
		applyConfig(cfg)
	}
	jsonOK(w, map[string]any{
		"db_user": cfg.DBUser, "db_name": cfg.DBName,
		"db_pass":     maskSecret(cfg.DBPass),
		"broker_game": cfg.BrokerGameAddr, "broker_admin": cfg.BrokerAdminAddr,
		"director_url": cfg.DirectorURL,
		"persisted":    r.URL.Query().Get("persist") == "true",
	})
}

// applyConfig pushes a saved appConfig back into the runtime globals so that
// connectAll() picks up the new values without requiring a process restart.
func applyConfig(cfg appConfig) {
	sshHost = cfg.SSHHost
	sshUser = cfg.SSHUser
	if cfg.SSHKey != "" {
		sshKeyPath = cfg.SSHKey
	}
	dbHost = cfg.DBHost
	if cfg.DBPort != 0 {
		dbPort = cfg.DBPort
	}
	dbUser = cfg.DBUser
	dbPass = cfg.DBPass
	dbName = cfg.DBName
	dbSchema = cfg.DBSchema
	controlPlane = cfg.Control
	controlNS = cfg.ControlNamespace
	brokerGameAddr = cfg.BrokerGameAddr
	brokerAdminAddr = cfg.BrokerAdminAddr
	brokerTLS = cfg.BrokerTLS
	brokerUser = cfg.BrokerUser
	brokerPass = cfg.BrokerPass
	backupDir = cfg.BackupDir
	sshMode = cfg.SSHMode
	sshExtraOpts = cfg.SSHExtraOpts
	autoDiscover = cfg.AutoDiscover
	loadedConfig = cfg
}
