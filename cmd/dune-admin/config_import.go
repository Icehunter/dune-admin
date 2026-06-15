package main

import (
	"fmt"
	"log"
	"os"
)

// configImportMarker guards the one-time config.yaml → DB import.
const configImportMarker = "migrated:config_yaml"

// activeServerMetaKey persists the active server's scope id across restarts.
const activeServerMetaKey = "active_server"

// scopedServerTables lists every per-feature table whose rows are scoped by
// server_id. Used to remap legacy string scope ids to the new numeric ids on
// first-boot import.
var scopedServerTables = []string{
	"welcome_grants", "welcome_config", "give_packs_config",
	"event_definitions", "event_award_claims",
	"battlepass_tiers", "battlepass_claims", "battlepass_accounts", "battlepass_grant_ledger",
	"play_sessions", "stat_snapshots",
}

// flatConfigHasConnection reports whether the flag-globals describe a real
// legacy single-server connection (vs an empty fresh install).
func flatConfigHasConnection() bool {
	return dbPass != "" || dbHost != "" || sshHost != "" || controlPlane != ""
}

// hydrateConfigFromStore makes the DB the source of truth for servers + global
// settings. On first boot it imports config.yaml once (guarded by a meta marker)
// and never reads it again; then it loads settings + servers from the DB into
// loadedConfig. No-op (legacy YAML path) when the store failed to open.
func hydrateConfigFromStore() {
	if globalStore == nil {
		return
	}
	marker, err := metaGet(globalStore, configImportMarker)
	if err != nil {
		log.Printf("config hydrate: read marker: %v", err)
		return
	}
	if marker == "" {
		// Only import when a real config.yaml exists. Without one this is a fresh
		// install: the flag-globals (env/.env/built-in defaults) must NOT be
		// imported as a phantom server — the DB stays empty so the SPA shows the
		// setup wizard. No marker is written, so a config.yaml dropped in later
		// still imports on its first boot.
		if _, statErr := os.Stat(configPath()); statErr != nil {
			log.Printf("config import: no config.yaml at %s — fresh install, nothing to import", configPath())
		} else if err := importConfigYAMLIntoStore(loadedConfig); err != nil {
			log.Printf("config import: %v", err)
			return
		}
	}

	if cfg, ok, err := globalSettingsStore.loadSettings(); err != nil {
		log.Printf("config hydrate: load settings: %v", err)
	} else if ok {
		loadedConfig = cfg // global settings; per-server fields come from servers table
	}

	servers, err := globalServersStore.listServers()
	if err != nil {
		log.Printf("config hydrate: list servers: %v", err)
	} else {
		loadedConfig.Servers = servers
	}

	if active, err := metaGet(globalStore, activeServerMetaKey); err == nil && active != "" {
		loadedConfig.DefaultServer = active
	}

	applyConfig(loadedConfig)
}

// importConfigYAMLIntoStore performs the one-time import of the YAML-loaded
// config (servers + global settings) into the DB, remapping per-feature
// server_id data from legacy string ids to the new numeric ids. Idempotent:
// clears the servers table first (clean retry) and writes the marker last.
func importConfigYAMLIntoStore(seed appConfig) error {
	if _, err := globalStore.Exec(`DELETE FROM servers`); err != nil {
		return fmt.Errorf("clear servers: %w", err)
	}
	if err := globalSettingsStore.saveSettings(seed); err != nil {
		return err
	}

	activeScope, err := importSeedServers(seed)
	if err != nil {
		return err
	}
	if activeScope != "" {
		if err := metaSet(globalStore, activeServerMetaKey, activeScope); err != nil {
			return err
		}
	}
	log.Printf("config import: seeded DB from config.yaml (%d server(s))", max(len(seed.Servers), btoi(activeScope != "")))
	return metaSet(globalStore, configImportMarker, "done")
}

// importSeedServers inserts the seed's servers (or the legacy flat single server)
// into the store, remapping each one's per-feature data, and returns the scope
// of the server that should become active (the first one).
func importSeedServers(seed appConfig) (string, error) {
	if len(seed.Servers) > 0 {
		var activeScope string
		for i, sc := range seed.Servers {
			newScope, err := importOneServer(sc, sc.LegacyID, i)
			if err != nil {
				return "", err
			}
			if i == 0 {
				activeScope = newScope
			}
		}
		return activeScope, nil
	}
	if flatConfigHasConnection() {
		// Legacy single-server (flat config) → one "default" server row.
		return importOneServer(legacyServerFromFlat(seed), "default", 0)
	}
	return "", nil
}

// importOneServer inserts sc at position and remaps any per-feature data from
// its legacy string scope (oldScope) to the new numeric scope. Returns the new
// scope.
func importOneServer(sc ServerConfig, oldScope string, position int) (string, error) {
	newID, err := globalServersStore.insertServer(sc, position)
	if err != nil {
		return "", err
	}
	newScope := serverScope(newID)
	if oldScope != "" && oldScope != newScope {
		if err := remapServerScope(oldScope, newScope); err != nil {
			return "", err
		}
	}
	return newScope, nil
}

// remapServerScope rewrites server_id from oldScope to newScope across every
// per-feature table (and the discord-status meta key), so per-server data
// survives the legacy-string-id → numeric-id switch.
func remapServerScope(oldScope, newScope string) error {
	for _, tbl := range scopedServerTables {
		// #nosec G202 -- tbl is from the hardcoded scopedServerTables constant
		if _, err := globalStore.Exec(`UPDATE `+tbl+` SET server_id = ? WHERE server_id = ?`, newScope, oldScope); err != nil {
			return fmt.Errorf("remap %s %s->%s: %w", tbl, oldScope, newScope, err)
		}
	}
	oldKey := "discord_status_message:" + oldScope
	if v, _ := metaGet(globalStore, oldKey); v != "" {
		if err := metaSet(globalStore, "discord_status_message:"+newScope, v); err != nil {
			return err
		}
		_, _ = globalStore.Exec(`DELETE FROM meta WHERE key = ?`, oldKey)
	}
	return nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
