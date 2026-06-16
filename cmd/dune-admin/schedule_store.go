package main

import (
	"database/sql"
	"errors"
	"fmt"
)

// errStoreUnavailable is returned by the schedule save funcs when the unified
// SQLite store is not open.
var errStoreUnavailable = errors.New("unified store unavailable")

// schedule_store.go holds the per-server SQLite schema + store funcs for the
// scheduled-backup and scheduled-restart configs (moved off the legacy
// scheduled-backups.json / scheduled-restarts.json files). Both schedules are
// owned by servers.id via an integer FK with ON DELETE CASCADE, so deleting a
// server purges its schedule + rules. Rule weekday sets are stored as an INTEGER
// bitmask (bit n set ⇔ weekday n present, 0=Sun..6=Sat) — no JSON/CSV.

// initServerBackupScheduleSchema creates the per-server backup-schedule tables.
// Both FK-reference servers(id) so a server delete cascades them. Idempotent.
func initServerBackupScheduleSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS server_backup_schedule (
			server_id  INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
			enabled    INTEGER NOT NULL DEFAULT 0,
			timezone   TEXT    NOT NULL DEFAULT '',
			keep_n     INTEGER NOT NULL DEFAULT 0,
			last_fired INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS server_backup_rule (
			server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			position  INTEGER NOT NULL DEFAULT 0,
			time      TEXT    NOT NULL DEFAULT '',
			days_mask INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (server_id, position)
		)`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_backup_rule_server ON server_backup_rule(server_id)`)
	return err
}

// initServerRestartScheduleSchema mirrors the backup schedule for restarts.
func initServerRestartScheduleSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS server_restart_schedule (
			server_id    INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
			enabled      INTEGER NOT NULL DEFAULT 0,
			timezone     TEXT    NOT NULL DEFAULT '',
			warn_minutes INTEGER NOT NULL DEFAULT 0,
			last_fired   INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS server_restart_rule (
			server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			position  INTEGER NOT NULL DEFAULT 0,
			time      TEXT    NOT NULL DEFAULT '',
			days_mask INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (server_id, position)
		)`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_restart_rule_server ON server_restart_rule(server_id)`)
	return err
}

// ── days ↔ bitmask helpers ──────────────────────────────────────────────────

// daysToMask encodes a []int weekday set (0=Sun..6=Sat) as a bitmask. Out-of-
// range days are ignored; duplicates collapse.
func daysToMask(days []int) int {
	mask := 0
	for _, d := range days {
		if d >= 0 && d <= 6 {
			mask |= 1 << uint(d)
		}
	}
	return mask
}

// maskToDays is the inverse of daysToMask, returning days in ascending order.
func maskToDays(mask int) []int {
	var days []int
	for d := 0; d <= 6; d++ {
		if mask&(1<<uint(d)) != 0 {
			days = append(days, d)
		}
	}
	return days
}

// ── backup schedule store funcs ─────────────────────────────────────────────

// loadBackupSchedule reads the backup schedule for serverID. ok=false when no
// schedule row exists for that server (caller applies defaults).
func loadBackupSchedule(db *sql.DB, serverID int) (scheduledBackupConfig, bool, error) {
	var cfg scheduledBackupConfig
	var enabled int
	err := db.QueryRow(
		`SELECT enabled, timezone, keep_n, last_fired FROM server_backup_schedule WHERE server_id = ?`,
		serverID).Scan(&enabled, &cfg.Timezone, &cfg.KeepN, &cfg.LastFired)
	if err == sql.ErrNoRows {
		return scheduledBackupConfig{}, false, nil
	}
	if err != nil {
		return scheduledBackupConfig{}, false, fmt.Errorf("load backup schedule %d: %w", serverID, err)
	}
	cfg.Enabled = enabled != 0
	rules, err := loadScheduleRules(db, "server_backup_rule", serverID)
	if err != nil {
		return scheduledBackupConfig{}, false, err
	}
	cfg.Rules = toBackupRules(rules)
	return cfg, true, nil
}

// saveBackupSchedule replaces the backup schedule + rules for serverID in a tx.
func saveBackupSchedule(db *sql.DB, serverID int, cfg scheduledBackupConfig) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO server_backup_schedule (server_id, enabled, timezone, keep_n, last_fired)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(server_id) DO UPDATE SET
		   enabled=excluded.enabled, timezone=excluded.timezone,
		   keep_n=excluded.keep_n, last_fired=excluded.last_fired`,
		serverID, btoi(cfg.Enabled), cfg.Timezone, cfg.KeepN, cfg.LastFired); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("save backup schedule %d: %w", serverID, err)
	}
	if err := replaceScheduleRules(tx, "server_backup_rule", serverID, fromBackupRules(cfg.Rules)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ── restart schedule store funcs ────────────────────────────────────────────

// loadRestartSchedule reads the restart schedule for serverID. ok=false when no
// schedule row exists for that server.
func loadRestartSchedule(db *sql.DB, serverID int) (scheduledRestartConfig, bool, error) {
	var cfg scheduledRestartConfig
	var enabled int
	err := db.QueryRow(
		`SELECT enabled, timezone, warn_minutes, last_fired FROM server_restart_schedule WHERE server_id = ?`,
		serverID).Scan(&enabled, &cfg.Timezone, &cfg.WarnMinutes, &cfg.LastFired)
	if err == sql.ErrNoRows {
		return scheduledRestartConfig{}, false, nil
	}
	if err != nil {
		return scheduledRestartConfig{}, false, fmt.Errorf("load restart schedule %d: %w", serverID, err)
	}
	cfg.Enabled = enabled != 0
	rules, err := loadScheduleRules(db, "server_restart_rule", serverID)
	if err != nil {
		return scheduledRestartConfig{}, false, err
	}
	cfg.Rules = toRestartRules(rules)
	return cfg, true, nil
}

// saveRestartSchedule replaces the restart schedule + rules for serverID in a tx.
func saveRestartSchedule(db *sql.DB, serverID int, cfg scheduledRestartConfig) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO server_restart_schedule (server_id, enabled, timezone, warn_minutes, last_fired)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(server_id) DO UPDATE SET
		   enabled=excluded.enabled, timezone=excluded.timezone,
		   warn_minutes=excluded.warn_minutes, last_fired=excluded.last_fired`,
		serverID, btoi(cfg.Enabled), cfg.Timezone, cfg.WarnMinutes, cfg.LastFired); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("save restart schedule %d: %w", serverID, err)
	}
	if err := replaceScheduleRules(tx, "server_restart_rule", serverID, fromRestartRules(cfg.Rules)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ── shared rule encoding ────────────────────────────────────────────────────

// scheduleRule is the storage-neutral form of a backup/restart rule.
type scheduleRule struct {
	Time string
	Mask int
}

// loadScheduleRules reads the ordered (time, days_mask) rules for serverID from
// the given rule table. table is a trusted internal literal.
func loadScheduleRules(db *sql.DB, table string, serverID int) ([]scheduleRule, error) {
	// #nosec G201 -- table is a trusted internal literal, never request input.
	rows, err := db.Query(`SELECT time, days_mask FROM `+table+` WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("load %s %d: %w", table, serverID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []scheduleRule
	for rows.Next() {
		var r scheduleRule
		if err := rows.Scan(&r.Time, &r.Mask); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// replaceScheduleRules deletes and re-inserts the ordered rules for serverID.
// table is a trusted internal literal.
func replaceScheduleRules(tx *sql.Tx, table string, serverID int, rules []scheduleRule) error {
	// #nosec G201 -- table is a trusted internal literal, never request input.
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear %s %d: %w", table, serverID, err)
	}
	for i, r := range rules {
		// #nosec G201 -- table is a trusted internal literal, never request input.
		if _, err := tx.Exec(
			`INSERT INTO `+table+` (server_id, position, time, days_mask) VALUES (?, ?, ?, ?)`,
			serverID, i, r.Time, r.Mask); err != nil {
			return fmt.Errorf("insert %s row %d: %w", table, i, err)
		}
	}
	return nil
}

func toBackupRules(rules []scheduleRule) []backupRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]backupRule, len(rules))
	for i, r := range rules {
		out[i] = backupRule{Days: maskToDays(r.Mask), Time: r.Time}
	}
	return out
}

func fromBackupRules(rules []backupRule) []scheduleRule {
	out := make([]scheduleRule, len(rules))
	for i, r := range rules {
		out[i] = scheduleRule{Time: r.Time, Mask: daysToMask(r.Days)}
	}
	return out
}

func toRestartRules(rules []scheduleRule) []restartRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]restartRule, len(rules))
	for i, r := range rules {
		out[i] = restartRule{Days: maskToDays(r.Mask), Time: r.Time}
	}
	return out
}

func fromRestartRules(rules []restartRule) []scheduleRule {
	out := make([]scheduleRule, len(rules))
	for i, r := range rules {
		out[i] = scheduleRule{Time: r.Time, Mask: daysToMask(r.Days)}
	}
	return out
}
