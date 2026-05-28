package main

import (
	"fmt"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

// handleGetConfig returns the current config with the DB password masked.
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		// No config file yet — return defaults derived from current globals.
		jsonOK(w, buildCurrentConfig())
		return
	}
	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		jsonErr(w, fmt.Errorf("parse config: %w", err), 500)
		return
	}
	if cfg.DBPass != "" {
		cfg.DBPass = "••••••••"
	}
	jsonOK(w, cfg)
}

// handleSaveConfig writes an updated config, then reconnects.
func preserveMaskedDBPass(
	cfg *appConfig,
	readFile func(string) ([]byte, error),
	path string,
	fallback string,
) {
	if cfg.DBPass != "••••••••" {
		return
	}
	existing, err := readFile(path)
	if err == nil {
		var old appConfig
		if yaml.Unmarshal(existing, &old) == nil && old.DBPass != "" {
			cfg.DBPass = old.DBPass
		}
	}
	if cfg.DBPass == "••••••••" {
		cfg.DBPass = fallback
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

func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var cfg appConfig
	if err := decode(r, &cfg); err != nil {
		jsonErr(w, fmt.Errorf("decode: %w", err), 400)
		return
	}

	// If the client sent back the masked placeholder, keep the existing password.
	preserveMaskedDBPass(&cfg, os.ReadFile, configPath(), dbPass)

	if err := writeConfigFile(cfg); err != nil {
		jsonErr(w, err, 500)
		return
	}

	// Apply the new values to globals so connectAll picks them up.
	applyConfig(cfg)
	resetRuntimeConnections()

	if err := connectAll(); err != nil {
		jsonErr(w, fmt.Errorf("reconnect failed: %w", err), 500)
		return
	}
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
		DBPass:           "••••••••",
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
	backupDir = cfg.BackupDir
	loadedConfig = cfg
}
