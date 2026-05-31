package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (registers "sqlite")
)

// welcomeStore is the SQLite ledger that makes welcome-package grants idempotent.
// Keyed by (fls_id, package_version, account_id): a granted OR failed row means
// "done with this account for this version". Bumping the version re-issues the
// package to everyone. Mirrors the embedded market-bot's SQLite cache pattern;
// kept in our own DB so we never touch Funcom's `dune` schema.
type welcomeStore struct {
	db *sql.DB
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
	fls_id          TEXT    NOT NULL,
	package_version TEXT    NOT NULL,
	account_id      INTEGER NOT NULL,
	character_name  TEXT    NOT NULL DEFAULT '',
	status          TEXT    NOT NULL,
	granted_at      TEXT    NOT NULL DEFAULT '',
	attempts        INTEGER NOT NULL DEFAULT 1,
	last_error      TEXT    NOT NULL DEFAULT '',
	detected_at     TEXT    NOT NULL,
	updated_at      TEXT    NOT NULL,
	PRIMARY KEY (fls_id, package_version, account_id)
);`

func openWelcomeStore(path string) (*welcomeStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open welcome store: %w", err)
	}
	if _, err := db.Exec(welcomeStoreSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init welcome schema: %w", err)
	}
	return &welcomeStore{db: db}, nil
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
		 WHERE fls_id = ? AND package_version = ? AND account_id = ? LIMIT 1`,
		flsID, version, accountID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("welcome grant exists: %w", err)
	}
	return true, nil
}

func (s *welcomeStore) insertGranted(flsID, version string, accountID int64, characterName string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO welcome_grants
			(fls_id, package_version, account_id, character_name, status, granted_at, attempts, last_error, detected_at, updated_at)
		VALUES (?, ?, ?, ?, 'granted', ?, 1, '', ?, ?)
		ON CONFLICT(fls_id, package_version, account_id) DO UPDATE SET
			status = 'granted',
			granted_at = excluded.granted_at,
			character_name = excluded.character_name,
			attempts = welcome_grants.attempts + 1,
			last_error = '',
			updated_at = excluded.updated_at`,
		flsID, version, accountID, characterName, now, now, now)
	if err != nil {
		return fmt.Errorf("insert granted: %w", err)
	}
	return nil
}

func (s *welcomeStore) insertFailed(flsID, version string, accountID int64, characterName, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO welcome_grants
			(fls_id, package_version, account_id, character_name, status, granted_at, attempts, last_error, detected_at, updated_at)
		VALUES (?, ?, ?, ?, 'failed', '', 1, ?, ?, ?)
		ON CONFLICT(fls_id, package_version, account_id) DO UPDATE SET
			status = 'failed',
			character_name = excluded.character_name,
			attempts = welcome_grants.attempts + 1,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		flsID, version, accountID, characterName, errMsg, now, now)
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
		 WHERE fls_id = ? AND package_version = ? AND account_id = ? AND status = 'failed'`,
		flsID, version, accountID)
	if err != nil {
		return 0, fmt.Errorf("delete failed grant: %w", err)
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
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
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
