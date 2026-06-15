package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (registers "sqlite")
)

// isDuplicateColumnErr returns true for the SQLite "duplicate column name" error
// that ALTER TABLE ADD COLUMN returns when the column already exists.
func isDuplicateColumnErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

// welcomeStore is the SQLite ledger that makes welcome-package grants idempotent.
// Keyed by (server_id, fls_id, package_version, account_id): a granted OR failed
// row means "done with this account for this version". Bumping the version
// re-issues the package to everyone. Mirrors the embedded market-bot's SQLite
// cache pattern; kept in our own DB so we never touch Funcom's `dune` schema.
type welcomeStore struct {
	db       *sql.DB
	serverID string
}

// welcomeGrantRecord is one ledger row, surfaced to the admin grants table.
type welcomeGrantRecord struct {
	FlsID          string `json:"fls_id"`
	PackageVersion string `json:"package_version"`
	AccountID      int64  `json:"account_id"`
	CharacterName  string `json:"character_name"`
	Status         string `json:"status"` // "granted" | "failed"
	GrantedAt      string `json:"granted_at"`
	Attempts       int64  `json:"attempts"`
	LastError      string `json:"last_error"`
	UpdatedAt      string `json:"updated_at"`
}

const welcomeStoreSchema = `
CREATE TABLE IF NOT EXISTS welcome_grants (
	server_id       TEXT    NOT NULL DEFAULT 'default',
	fls_id          TEXT    NOT NULL,
	package_version TEXT    NOT NULL,
	account_id      INTEGER NOT NULL,
	character_name  TEXT    NOT NULL DEFAULT '',
	status          TEXT    NOT NULL,
	granted_at      TEXT    NOT NULL DEFAULT '',
	attempts        INTEGER NOT NULL DEFAULT 1,
	last_error      TEXT    NOT NULL DEFAULT '',
	detected_at     TEXT    NOT NULL DEFAULT '',
	updated_at      TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (server_id, fls_id, package_version, account_id)
);
CREATE TABLE IF NOT EXISTS welcome_config (
	server_id                      TEXT    NOT NULL DEFAULT 'default',
	enabled                        INTEGER NOT NULL DEFAULT 0,
	scan_secs                      INTEGER NOT NULL DEFAULT 30,
	active_version                 TEXT    NOT NULL DEFAULT '',
	active_versions_json           TEXT    NOT NULL DEFAULT '',
	packages_json                  TEXT    NOT NULL DEFAULT '[]',
	welcome_message_enabled        INTEGER NOT NULL DEFAULT 0,
	welcome_message                TEXT    NOT NULL DEFAULT '',
	welcome_whisper_source_player  TEXT    NOT NULL DEFAULT '',
	motd_enabled                   INTEGER NOT NULL DEFAULT 0,
	motd_message                   TEXT    NOT NULL DEFAULT '',
	motd_source_player             TEXT    NOT NULL DEFAULT '',
	region_join_enabled            INTEGER NOT NULL DEFAULT 0,
	region_leave_enabled           INTEGER NOT NULL DEFAULT 0,
	region_join_template           TEXT    NOT NULL DEFAULT '',
	region_leave_template          TEXT    NOT NULL DEFAULT '',
	region_chat_channel            TEXT    NOT NULL DEFAULT 'whisper',
	updated_at                     TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (server_id)
);`

// welcomeConfigRow holds the single config row stored in welcome_config.
// PackagesJSON is the JSON-encoded []welcomePackage slice.
// ActiveVersions is the list of active package versions (new field).
// ActiveVersion is the legacy single-version field kept for backwards compat.
type welcomeConfigRow struct {
	Enabled                    bool
	ScanSecs                   int
	ActiveVersion              string
	ActiveVersions             []string
	PackagesJSON               string
	WelcomeMessageEnabled      bool
	WelcomeMessage             string
	WelcomeWhisperSourcePlayer string
	MotdEnabled                bool
	MotdMessage                string
	MotdSourcePlayer           string
	RegionJoinEnabled          bool
	RegionLeaveEnabled         bool
	RegionJoinTemplate         string
	RegionLeaveTemplate        string
	RegionChatChannel          string // "whisper" | "map"
}

// initWelcomeSchema creates the welcome tables and applies column migrations on
// db. Safe to call against a shared handle (the unified store) or a dedicated
// file. Idempotent.
func initWelcomeSchema(db *sql.DB) error {
	if _, err := db.Exec(welcomeStoreSchema); err != nil {
		return fmt.Errorf("init welcome schema: %w", err)
	}
	// Add welcome_message columns to existing DBs that predate this feature.
	// SQLite does not support IF NOT EXISTS in ALTER TABLE, so we attempt each
	// column and ignore "duplicate column" errors (1 = SQLITE_ERROR when column
	// already exists — the message contains "duplicate column name").
	for _, col := range []string{
		"ALTER TABLE welcome_config ADD COLUMN welcome_message_enabled INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE welcome_config ADD COLUMN welcome_message TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN welcome_whisper_source_player TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN active_versions_json TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN motd_enabled INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE welcome_config ADD COLUMN motd_message TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN motd_source_player TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN region_join_enabled INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE welcome_config ADD COLUMN region_leave_enabled INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE welcome_config ADD COLUMN region_join_template TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN region_leave_template TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE welcome_config ADD COLUMN region_chat_channel TEXT NOT NULL DEFAULT 'whisper'",
	} {
		if _, alterErr := db.Exec(col); alterErr != nil {
			if !isDuplicateColumnErr(alterErr) {
				return fmt.Errorf("migrate welcome_config: %w", alterErr)
			}
		}
	}
	if err := rebuildTableWithServerID(db, "welcome_grants", "welcome_grants_new",
		`CREATE TABLE welcome_grants_new (
			server_id       TEXT    NOT NULL DEFAULT 'default',
			fls_id          TEXT    NOT NULL,
			package_version TEXT    NOT NULL,
			account_id      INTEGER NOT NULL,
			character_name  TEXT    NOT NULL DEFAULT '',
			status          TEXT    NOT NULL,
			granted_at      TEXT    NOT NULL DEFAULT '',
			attempts        INTEGER NOT NULL DEFAULT 1,
			last_error      TEXT    NOT NULL DEFAULT '',
			detected_at     TEXT    NOT NULL DEFAULT '',
			updated_at      TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id, fls_id, package_version, account_id)
		)`,
		[]string{"fls_id", "package_version", "account_id", "character_name", "status",
			"granted_at", "attempts", "last_error", "detected_at", "updated_at"},
	); err != nil {
		return fmt.Errorf("migrate welcome_grants server_id: %w", err)
	}
	if err := rebuildTableWithServerID(db, "welcome_config", "welcome_config_new",
		`CREATE TABLE welcome_config_new (
			server_id                      TEXT    NOT NULL DEFAULT 'default',
			enabled                        INTEGER NOT NULL DEFAULT 0,
			scan_secs                      INTEGER NOT NULL DEFAULT 30,
			active_version                 TEXT    NOT NULL DEFAULT '',
			active_versions_json           TEXT    NOT NULL DEFAULT '',
			packages_json                  TEXT    NOT NULL DEFAULT '[]',
			welcome_message_enabled        INTEGER NOT NULL DEFAULT 0,
			welcome_message                TEXT    NOT NULL DEFAULT '',
			welcome_whisper_source_player  TEXT    NOT NULL DEFAULT '',
			motd_enabled                   INTEGER NOT NULL DEFAULT 0,
			motd_message                   TEXT    NOT NULL DEFAULT '',
			motd_source_player             TEXT    NOT NULL DEFAULT '',
			region_join_enabled            INTEGER NOT NULL DEFAULT 0,
			region_leave_enabled           INTEGER NOT NULL DEFAULT 0,
			region_join_template           TEXT    NOT NULL DEFAULT '',
			region_leave_template          TEXT    NOT NULL DEFAULT '',
			region_chat_channel            TEXT    NOT NULL DEFAULT 'whisper',
			updated_at                     TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id)
		)`,
		[]string{"enabled", "scan_secs", "active_version", "active_versions_json", "packages_json",
			"welcome_message_enabled", "welcome_message", "welcome_whisper_source_player",
			"motd_enabled", "motd_message", "motd_source_player",
			"region_join_enabled", "region_leave_enabled", "region_join_template",
			"region_leave_template", "region_chat_channel", "updated_at"},
	); err != nil {
		return fmt.Errorf("migrate welcome_config server_id: %w", err)
	}
	if err := initWelcomeColumnsSchema(db); err != nil {
		return err
	}
	return nil
}

// newWelcomeStore wraps an already-initialised shared handle (schema created by
// openUnifiedStore). Used in production so all stores share one SQLite file.
func newWelcomeStore(db *sql.DB, serverID string) *welcomeStore {
	return &welcomeStore{db: db, serverID: serverID}
}

func openWelcomeStore(path string) (*welcomeStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open welcome store: %w", err)
	}
	if err := initWelcomeSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &welcomeStore{db: db, serverID: "default"}, nil
}

func (s *welcomeStore) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// grantExists reports whether this account already has a granted OR failed row
// for the version — either way the scanner skips it.
func (s *welcomeStore) grantExists(flsID, version string, accountID int64) (bool, error) {
	var one int
	err := s.db.QueryRow(
		`SELECT 1 FROM welcome_grants
		 WHERE server_id = ? AND fls_id = ? AND package_version = ? AND account_id = ? LIMIT 1`,
		s.serverID, flsID, version, accountID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("welcome grant exists: %w", err)
	}
	return true, nil
}

// findGrant returns the ledger row for (flsID, version, accountID) if present.
func (s *welcomeStore) findGrant(flsID, version string, accountID int64) (welcomeGrantRecord, bool, error) {
	var r welcomeGrantRecord
	err := s.db.QueryRow(
		`SELECT fls_id, package_version, account_id, character_name, status,
		        granted_at, attempts, last_error, updated_at
		 FROM welcome_grants
		 WHERE server_id = ? AND fls_id = ? AND package_version = ? AND account_id = ? LIMIT 1`,
		s.serverID, flsID, version, accountID).Scan(
		&r.FlsID, &r.PackageVersion, &r.AccountID, &r.CharacterName, &r.Status,
		&r.GrantedAt, &r.Attempts, &r.LastError, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return welcomeGrantRecord{}, false, nil
	}
	if err != nil {
		return welcomeGrantRecord{}, false, fmt.Errorf("find welcome grant: %w", err)
	}
	return r, true, nil
}

func (s *welcomeStore) insertGranted(flsID, version string, accountID int64, characterName string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO welcome_grants
			(server_id, fls_id, package_version, account_id, character_name, status, granted_at, attempts, last_error, detected_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'granted', ?, 1, '', ?, ?)
		ON CONFLICT(server_id, fls_id, package_version, account_id) DO UPDATE SET
			status = 'granted',
			granted_at = excluded.granted_at,
			character_name = excluded.character_name,
			attempts = welcome_grants.attempts + 1,
			last_error = '',
			updated_at = excluded.updated_at`,
		s.serverID, flsID, version, accountID, characterName, now, now, now)
	if err != nil {
		return fmt.Errorf("insert granted: %w", err)
	}
	return nil
}

func (s *welcomeStore) insertFailed(flsID, version string, accountID int64, characterName, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO welcome_grants
			(server_id, fls_id, package_version, account_id, character_name, status, granted_at, attempts, last_error, detected_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'failed', '', 1, ?, ?, ?)
		ON CONFLICT(server_id, fls_id, package_version, account_id) DO UPDATE SET
			status = 'failed',
			character_name = excluded.character_name,
			attempts = welcome_grants.attempts + 1,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		s.serverID, flsID, version, accountID, characterName, errMsg, now, now)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}
	return nil
}

// deleteFailed clears a failed ledger row so the next scan re-attempts it. Only
// 'failed' rows are removed; 'granted' rows are left in place so a retry can
// never duplicate a successful package. Returns rows deleted.
func (s *welcomeStore) deleteFailed(flsID, version string, accountID int64) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM welcome_grants
		 WHERE server_id = ? AND fls_id = ? AND package_version = ? AND account_id = ? AND status = 'failed'`,
		s.serverID, flsID, version, accountID)
	if err != nil {
		return 0, fmt.Errorf("delete failed grant: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// deleteGrant removes a ledger row regardless of status (granted or failed). It
// backs the explicit "revoke" action: unlike deleteFailed (which retries a
// failed grant), this clears a SUCCESSFUL grant so the same package can be
// granted to that account again on the next scan (#162). Returns rows deleted.
func (s *welcomeStore) deleteGrant(flsID, version string, accountID int64) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM welcome_grants
		 WHERE server_id = ? AND fls_id = ? AND package_version = ? AND account_id = ?`,
		s.serverID, flsID, version, accountID)
	if err != nil {
		return 0, fmt.Errorf("delete grant: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *welcomeStore) listGrants(limit int) ([]welcomeGrantRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT fls_id, package_version, account_id, character_name, status,
		       granted_at, attempts, last_error, updated_at
		FROM welcome_grants
		WHERE server_id = ?
		ORDER BY updated_at DESC
		LIMIT ?`, s.serverID, limit)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]welcomeGrantRecord, 0)
	for rows.Next() {
		var r welcomeGrantRecord
		if err := rows.Scan(&r.FlsID, &r.PackageVersion, &r.AccountID, &r.CharacterName,
			&r.Status, &r.GrantedAt, &r.Attempts, &r.LastError, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan grant: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// saveConfig upserts the single welcome_config row (id=1).
func (s *welcomeStore) saveConfig(cfg welcomeConfigRow) error {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	msgEnabled := 0
	if cfg.WelcomeMessageEnabled {
		msgEnabled = 1
	}
	motdEnabled := 0
	if cfg.MotdEnabled {
		motdEnabled = 1
	}
	regionJoinEnabled := 0
	if cfg.RegionJoinEnabled {
		regionJoinEnabled = 1
	}
	regionLeaveEnabled := 0
	if cfg.RegionLeaveEnabled {
		regionLeaveEnabled = 1
	}
	// Derive compat active_version from slice (first element) or keep as-is.
	activeVersion := cfg.ActiveVersion
	if len(cfg.ActiveVersions) > 0 {
		activeVersion = cfg.ActiveVersions[0]
	}
	// Packages and active versions now live in the typed welcome child tables.
	// The legacy blob columns are written as empty placeholders ('[]' / '') so
	// the row schema is unchanged but no longer authoritative.
	packages, err := parseWelcomePackagesJSON(cfg.PackagesJSON)
	if err != nil {
		return fmt.Errorf("parse packages_json: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	regionChatChannel := cfg.RegionChatChannel
	if regionChatChannel == "" {
		regionChatChannel = "whisper"
	}
	_, err = s.db.Exec(`
		INSERT INTO welcome_config
			(server_id, enabled, scan_secs, active_version, active_versions_json, packages_json,
			 welcome_message_enabled, welcome_message, welcome_whisper_source_player,
			 motd_enabled, motd_message, motd_source_player,
			 region_join_enabled, region_leave_enabled, region_join_template, region_leave_template,
			 region_chat_channel, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET
			enabled                       = excluded.enabled,
			scan_secs                     = excluded.scan_secs,
			active_version                = excluded.active_version,
			active_versions_json          = excluded.active_versions_json,
			packages_json                 = excluded.packages_json,
			welcome_message_enabled       = excluded.welcome_message_enabled,
			welcome_message               = excluded.welcome_message,
			welcome_whisper_source_player = excluded.welcome_whisper_source_player,
			motd_enabled                  = excluded.motd_enabled,
			motd_message                  = excluded.motd_message,
			motd_source_player            = excluded.motd_source_player,
			region_join_enabled           = excluded.region_join_enabled,
			region_leave_enabled          = excluded.region_leave_enabled,
			region_join_template          = excluded.region_join_template,
			region_leave_template         = excluded.region_leave_template,
			region_chat_channel           = excluded.region_chat_channel,
			updated_at                    = excluded.updated_at`,
		s.serverID, enabled, cfg.ScanSecs, activeVersion, "", "[]",
		msgEnabled, cfg.WelcomeMessage, cfg.WelcomeWhisperSourcePlayer,
		motdEnabled, cfg.MotdMessage, cfg.MotdSourcePlayer,
		regionJoinEnabled, regionLeaveEnabled, cfg.RegionJoinTemplate, cfg.RegionLeaveTemplate,
		regionChatChannel, now)
	if err != nil {
		return fmt.Errorf("save welcome config: %w", err)
	}
	if err := saveWelcomePackagesColumns(s.db, s.serverID, packages, cfg.ActiveVersions); err != nil {
		return fmt.Errorf("save welcome config: %w", err)
	}
	return nil
}

// parseWelcomePackagesJSON decodes a packages_json blob into typed packages,
// treating "" / "[]" / "null" as an empty library.
func parseWelcomePackagesJSON(blob string) ([]welcomePackage, error) {
	var packages []welcomePackage
	if err := decodeJSONList(blob, &packages); err != nil {
		return nil, err
	}
	return packages, nil
}

// loadConfig reads the single welcome_config row. Returns (row, true, nil) if
// it exists, or (zero, false, nil) if the table is empty (first boot).
func (s *welcomeStore) loadConfig() (welcomeConfigRow, bool, error) {
	var row welcomeConfigRow
	var enabledInt, msgEnabledInt, motdEnabledInt int
	var regionJoinEnabledInt, regionLeaveEnabledInt int
	err := s.db.QueryRow(`
		SELECT enabled, scan_secs, active_version,
		       welcome_message_enabled, welcome_message, welcome_whisper_source_player,
		       motd_enabled, motd_message, motd_source_player,
		       region_join_enabled, region_leave_enabled, region_join_template, region_leave_template,
		       region_chat_channel
		FROM welcome_config WHERE server_id = ?`, s.serverID).
		Scan(&enabledInt, &row.ScanSecs, &row.ActiveVersion,
			&msgEnabledInt, &row.WelcomeMessage, &row.WelcomeWhisperSourcePlayer,
			&motdEnabledInt, &row.MotdMessage, &row.MotdSourcePlayer,
			&regionJoinEnabledInt, &regionLeaveEnabledInt, &row.RegionJoinTemplate, &row.RegionLeaveTemplate,
			&row.RegionChatChannel)
	if errors.Is(err, sql.ErrNoRows) {
		return welcomeConfigRow{}, false, nil
	}
	if err != nil {
		return welcomeConfigRow{}, false, fmt.Errorf("load welcome config: %w", err)
	}
	row.Enabled = enabledInt != 0
	row.WelcomeMessageEnabled = msgEnabledInt != 0
	row.MotdEnabled = motdEnabledInt != 0
	row.RegionJoinEnabled = regionJoinEnabledInt != 0
	row.RegionLeaveEnabled = regionLeaveEnabledInt != 0
	// Packages and active versions now come from the typed welcome child tables.
	packages, activeVersions, err := loadWelcomePackagesColumns(s.db, s.serverID)
	if err != nil {
		return welcomeConfigRow{}, false, err
	}
	packagesJSON, err := json.Marshal(packages)
	if err != nil {
		return welcomeConfigRow{}, false, fmt.Errorf("marshal welcome packages: %w", err)
	}
	row.PackagesJSON = string(packagesJSON)
	row.ActiveVersions = activeVersions
	// Compat: promote the scalar active_version for old rows with no typed list.
	if len(row.ActiveVersions) == 0 && row.ActiveVersion != "" {
		row.ActiveVersions = []string{row.ActiveVersion}
	}
	return row, true, nil
}
