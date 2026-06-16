package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
)

// schedule_migration.go performs the one-way file→DB migration of the three
// legacy file-based config stores: scheduled-backups.json, scheduled-restarts.json
// (per-server, stamped to the default server id), and permissions.yaml (app-level,
// no server id). Each step is marker-gated for idempotency and leaves the source
// file on disk untouched (rollback path).

// migrateLegacyPermissions moves permissions.yaml into app_role_capabilities,
// once. No-op when the table already holds rows OR the file is absent. The file
// is left on disk.
func migrateLegacyPermissions(db *sql.DB) error {
	return runColumnMigrationOnce(db, "migrated:permissions", importLegacyPermissionsTx)
}

// importLegacyPermissionsTx reads permissions.yaml into app_role_capabilities
// when the table is empty and the file exists. No-op otherwise.
func importLegacyPermissionsTx(tx *sql.Tx) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM app_role_capabilities`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil // already populated — nothing to import
	}
	matrix, err := loadPermissionsMatrix(permissionsPath())
	if err != nil {
		return err
	}
	if len(matrix) == 0 {
		return nil // no file / empty file — fresh install seeds defaults later
	}
	for role, caps := range matrix {
		for _, cap := range caps {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO app_role_capabilities (role_id, capability) VALUES (?, ?)`,
				role, cap); err != nil {
				return err
			}
		}
	}
	return nil
}

// migrateLegacyBackupSchedule moves scheduled-backups.json into the per-server
// backup-schedule tables for serverID, once. No-op when the server already has a
// schedule row OR the file is absent. File left on disk.
func migrateLegacyBackupSchedule(db *sql.DB, serverID int) error {
	return runColumnMigrationOnce(db, "migrated:backup_schedule", func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM server_backup_schedule WHERE server_id = ?`, serverID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		cfg, ok, err := readLegacyBackupFile()
		if err != nil || !ok {
			return err
		}
		return insertBackupScheduleTx(tx, serverID, cfg)
	})
}

// migrateLegacyRestartSchedule mirrors the backup migration for restarts.
func migrateLegacyRestartSchedule(db *sql.DB, serverID int) error {
	return runColumnMigrationOnce(db, "migrated:restart_schedule", func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM server_restart_schedule WHERE server_id = ?`, serverID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		cfg, ok, err := readLegacyRestartFile()
		if err != nil || !ok {
			return err
		}
		return insertRestartScheduleTx(tx, serverID, cfg)
	})
}

// readLegacyBackupFile reads scheduled-backups.json. ok=false when absent.
func readLegacyBackupFile() (scheduledBackupConfig, bool, error) {
	data, err := os.ReadFile(scheduledBackupPath())
	if errors.Is(err, os.ErrNotExist) {
		return scheduledBackupConfig{}, false, nil
	}
	if err != nil {
		return scheduledBackupConfig{}, false, err
	}
	var c scheduledBackupConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return scheduledBackupConfig{}, false, err
	}
	return c, true, nil
}

// readLegacyRestartFile reads scheduled-restarts.json. ok=false when absent.
func readLegacyRestartFile() (scheduledRestartConfig, bool, error) {
	data, err := os.ReadFile(scheduledRestartPath())
	if errors.Is(err, os.ErrNotExist) {
		return scheduledRestartConfig{}, false, nil
	}
	if err != nil {
		return scheduledRestartConfig{}, false, err
	}
	var c scheduledRestartConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return scheduledRestartConfig{}, false, err
	}
	if c.WarnMinutes <= 0 {
		c.WarnMinutes = defaultWarnMinutes
	}
	return c, true, nil
}

// insertBackupScheduleTx writes a backup schedule + rules inside an existing tx.
func insertBackupScheduleTx(tx *sql.Tx, serverID int, cfg scheduledBackupConfig) error {
	if _, err := tx.Exec(
		`INSERT INTO server_backup_schedule (server_id, enabled, timezone, keep_n, last_fired)
		 VALUES (?, ?, ?, ?, ?)`,
		serverID, btoi(cfg.Enabled), cfg.Timezone, cfg.KeepN, cfg.LastFired); err != nil {
		return err
	}
	return replaceScheduleRules(tx, "server_backup_rule", serverID, fromBackupRules(cfg.Rules))
}

// insertRestartScheduleTx writes a restart schedule + rules inside an existing tx.
func insertRestartScheduleTx(tx *sql.Tx, serverID int, cfg scheduledRestartConfig) error {
	if _, err := tx.Exec(
		`INSERT INTO server_restart_schedule (server_id, enabled, timezone, warn_minutes, last_fired)
		 VALUES (?, ?, ?, ?, ?)`,
		serverID, btoi(cfg.Enabled), cfg.Timezone, cfg.WarnMinutes, cfg.LastFired); err != nil {
		return err
	}
	return replaceScheduleRules(tx, "server_restart_rule", serverID, fromRestartRules(cfg.Rules))
}
