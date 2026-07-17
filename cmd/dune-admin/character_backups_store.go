package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// character_backups_store.go persists metadata about full-character backups
// captured via the game's native transfer subsystem (see
// dune.character_transfer_export / dune.character_transfer_import,
// cmdCaptureCharacterBackup / cmdRestoreCharacterBackup in db.go). The
// captured data itself is the game's own transfer-format JSON, written to a
// file under the server's backups directory — this store only tracks which
// backups exist and where their file lives, so an admin can find, restore,
// download, or delete them later. Mirrors battlepassStore's server-scoped,
// global+withScope shape.

// characterBackup is one row from character_backups: metadata about a single
// captured full-character transfer backup. FilePath points at the JSON file
// holding the actual dune.character_transfer_export output for this backup.
type characterBackup struct {
	ID              int64  `json:"id"`
	AccountID       int64  `json:"account_id"`
	FLSID           string `json:"fls_id"`
	CharacterName   string `json:"character_name"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	FilePath        string `json:"file_path"`
	PatchesChecksum string `json:"patches_checksum"`
	CreatedAt       string `json:"created_at"`
}

type characterBackupsStore struct {
	db       *sql.DB
	serverID int
}

// globalCharacterBackupsStore is set once at startup (initCharacterBackupsStore,
// main.go). Nil when the unified store failed to open — callers must guard.
var globalCharacterBackupsStore *characterBackupsStore

const characterBackupsStoreSchema = `
CREATE TABLE IF NOT EXISTS character_backups (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	server_id        INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	account_id       INTEGER NOT NULL,
	fls_id           TEXT    NOT NULL DEFAULT '',
	character_name   TEXT    NOT NULL DEFAULT '',
	action           TEXT    NOT NULL,
	reason           TEXT    NOT NULL DEFAULT '',
	file_path        TEXT    NOT NULL,
	patches_checksum TEXT    NOT NULL DEFAULT '',
	created_at       TEXT    NOT NULL
);`

// initCharacterBackupsSchema creates the character_backups table on db. Safe
// to call against a shared handle (the unified store). Idempotent.
func initCharacterBackupsSchema(db *sql.DB) error {
	if _, err := db.Exec(characterBackupsStoreSchema); err != nil {
		return fmt.Errorf("init character backups schema: %w", err)
	}
	return nil
}

// newCharacterBackupsStore wraps an already-initialised shared handle (schema
// created by openUnifiedStore). Used so this store shares one SQLite file
// with every other store in production.
func newCharacterBackupsStore(db *sql.DB, serverID int) *characterBackupsStore {
	return &characterBackupsStore{db: db, serverID: serverID}
}

// openCharacterBackupsStore opens (or creates) a standalone database at path
// and ensures the schema exists. path may be ":memory:" for tests.
func openCharacterBackupsStore(path string) (*characterBackupsStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open character backups store: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := initCharacterBackupsSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &characterBackupsStore{db: db, serverID: defaultServerID}, nil
}

// withScope returns a store bound to a different server_id, sharing the same
// underlying handle — mirrors battlepassStore.withScope.
func (s *characterBackupsStore) withScope(serverID int) *characterBackupsStore {
	return &characterBackupsStore{db: s.db, serverID: serverID}
}

const characterBackupColumns = `id, account_id, fls_id, character_name, action, reason, file_path, patches_checksum, created_at`

func scanCharacterBackup(row interface{ Scan(...any) error }) (characterBackup, error) {
	var b characterBackup
	err := row.Scan(&b.ID, &b.AccountID, &b.FLSID, &b.CharacterName, &b.Action, &b.Reason, &b.FilePath, &b.PatchesChecksum, &b.CreatedAt)
	return b, err
}

// create inserts a new backup record and returns the stored row (with id and
// created_at populated).
func (s *characterBackupsStore) create(b characterBackup) (*characterBackup, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
		INSERT INTO character_backups (server_id, account_id, fls_id, character_name, action, reason, file_path, patches_checksum, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.serverID, b.AccountID, b.FLSID, b.CharacterName, b.Action, b.Reason, b.FilePath, b.PatchesChecksum, now)
	if err != nil {
		return nil, fmt.Errorf("create character backup: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.get(id)
}

// get returns one backup record by id, scoped to this store's server.
func (s *characterBackupsStore) get(id int64) (*characterBackup, error) {
	row := s.db.QueryRow(`SELECT `+characterBackupColumns+` FROM character_backups WHERE server_id = ? AND id = ?`, s.serverID, id)
	b, err := scanCharacterBackup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get character backup %d: %w", id, err)
	}
	return &b, nil
}

// list returns every backup record for this store's server, newest first.
func (s *characterBackupsStore) list() ([]characterBackup, error) {
	rows, err := s.db.Query(`SELECT `+characterBackupColumns+` FROM character_backups WHERE server_id = ? ORDER BY id DESC`, s.serverID)
	if err != nil {
		return nil, fmt.Errorf("list character backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCharacterBackups(rows)
}

// listForAccount returns every backup record for this store's server scoped
// to one account, newest first — powers the per-player backups panel.
func (s *characterBackupsStore) listForAccount(accountID int64) ([]characterBackup, error) {
	rows, err := s.db.Query(`SELECT `+characterBackupColumns+` FROM character_backups WHERE server_id = ? AND account_id = ? ORDER BY id DESC`, s.serverID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list character backups for account %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanCharacterBackups(rows)
}

func scanCharacterBackups(rows *sql.Rows) ([]characterBackup, error) {
	out := make([]characterBackup, 0)
	for rows.Next() {
		b, err := scanCharacterBackup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan character backup: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// delete removes one backup record by id, scoped to this store's server. The
// caller is responsible for removing the backing file — this only drops the
// metadata row. Returns errNotFound if no row matched (including a row that
// exists but belongs to a different server).
func (s *characterBackupsStore) delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM character_backups WHERE server_id = ? AND id = ?`, s.serverID, id)
	if err != nil {
		return fmt.Errorf("delete character backup %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete character backup %d: %w", id, err)
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}
