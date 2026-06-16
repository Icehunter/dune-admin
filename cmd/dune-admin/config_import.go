package main

import (
	"fmt"
	"os"
)

// configImportMarker guards the one-time config.yaml → DB import.
const configImportMarker = "migrated:config_yaml"

// activeServerMetaKey persists the active server's scope id across restarts.
const activeServerMetaKey = "active_server"

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
		componentLog("config_import").Error().Err(err).Msg("read marker")
		return
	}
	if marker == "" {
		// Only import when a real config.yaml exists. Without one this is a fresh
		// install: the flag-globals (env/.env/built-in defaults) must NOT be
		// imported as a phantom server — the DB stays empty so the SPA shows the
		// setup wizard. No marker is written, so a config.yaml dropped in later
		// still imports on its first boot.
		if _, statErr := os.Stat(configPath()); statErr != nil {
			componentLog("config_import").Info().Str("config_path", configPath()).Msg("no config.yaml — fresh install, nothing to import")
		} else if err := importConfigYAMLIntoStore(loadedConfig); err != nil {
			componentLog("config_import").Error().Err(err).Msg("import config.yaml failed")
			return
		}
	}

	// Phase B of the remodel: now that the server row(s) exist with numeric ids,
	// convert any legacy 0.39.5-shaped scoped tables (TEXT/absent server_id +
	// JSON blobs) to the int-FK + surrogate-id schema. No-op on a fresh install.
	if id, ok := firstServerID(); ok {
		migrateUnifiedRemodel(globalStore, id)
	}

	if cfg, ok, err := globalSettingsStore.loadSettings(); err != nil {
		componentLog("config_import").Error().Err(err).Msg("load settings")
	} else if ok {
		loadedConfig = cfg // global settings; per-server fields come from servers table
	}

	servers, err := globalServersStore.listServers()
	if err != nil {
		componentLog("config_import").Error().Err(err).Msg("list servers")
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
	componentLog("config_import").Info().
		Int("server_count", max(len(seed.Servers), btoi(activeScope != ""))).
		Msg("seeded DB from config.yaml")
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

// importOneServer inserts sc at position and returns the new numeric scope. The
// 0.39.5 single-server text→int conversion of per-feature data is handled by the
// migration phase (migrateUnifiedRemodel), not here.
func importOneServer(sc ServerConfig, _ string, position int) (string, error) {
	newID, err := globalServersStore.insertServer(sc, position)
	if err != nil {
		return "", err
	}
	return serverScope(newID), nil
}

// firstServerID returns the lowest server id (the default/first server) and
// whether any server exists. Used to scope the 0.39.5 single-server data.
func firstServerID() (int, bool) {
	if globalStore == nil {
		return 0, false
	}
	var id int
	err := globalStore.QueryRow(`SELECT id FROM servers ORDER BY position, id LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
