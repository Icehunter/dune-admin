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
		cfg.DiscordBotToken == masked

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

// stopEmbeddedMarketBot cancels the running embedded bot (if any) and clears
// embeddedBot and globalBotCancel. Call this before resetRuntimeConnections so
// the bot releases its reference to the old (about-to-be-closed) globalDB pool.
func stopEmbeddedMarketBot() {
	if embeddedBot == nil {
		return
	}
	if globalBotCancel != nil {
		globalBotCancel()
		globalBotCancel = nil
	}
	embeddedBot = nil
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

	if err := writeConfigFile(cfg); err != nil {
		jsonErr(w, err, 500)
		return
	}

	applyConfig(cfg)

	// Stop the running bot (if any) before closing the DB pool.
	// A running bot holds a reference to globalDB; if we close the pool first
	// the bot's next tick will use a closed pool and panic or error.
	stopEmbeddedMarketBot()

	resetRuntimeConnections()

	// Reconnect is best-effort — config is already written to disk.
	// If reconnect fails (e.g. SSH not yet reachable), the file is still
	// saved and will take effect on the next restart or manual reconnect.
	if err := connectAll(); err != nil {
		log.Printf("handleSaveConfig: reconnect after save: %v", err)
	}

	// Apply the market bot config AFTER connectAll so the bot gets the
	// freshly-established globalDB rather than the old (closed) pool.
	// applyMarketBotConfig will restart the bot (if enabled) with the new pool.
	applyMarketBotConfig(cfg)
	handleStatus(w, r)
}

// buildCurrentConfig constructs an appConfig from the current global vars.
func buildCurrentConfig() appConfig {
	return appConfig{
		SSHHost:          sshHost,
		SSHUser:          sshUser,
		SSHKey:           sshKeyPath,
		DBHost:           dbHost,
		DBPort:           dbPort,
		DBUser:           dbUser,
		DBPass:           masked,
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

// applyMarketBotConfig stops or starts the embedded market bot to match the
// new config. Called after applyConfig so loadedConfig is already updated.
func applyMarketBotConfig(cfg appConfig) {
	wantEnabled := marketBotEnabled(cfg)
	botRunning := embeddedBot != nil

	if botRunning && !wantEnabled {
		log.Printf("config: market_bot_enabled set to false — stopping embedded bot")
		if globalBotCancel != nil {
			globalBotCancel()
			globalBotCancel = nil
		}
		embeddedBot = nil
	}

	if !botRunning && wantEnabled {
		log.Printf("config: market_bot_enabled set to true — starting embedded bot")
		if cancel := startEmbeddedMarketBotIfEnabled(cfg); cancel != nil {
			globalBotCancel = cancel
		}
	}

	// Update remote proxy from new config.
	if cfg.MarketBotRemoteURL != "" {
		remoteBotProxy = newRemoteBotClient(cfg.MarketBotRemoteURL, cfg.MarketBotRemoteToken)
	} else {
		remoteBotProxy = nil
	}
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
	loadedConfig = cfg
}
