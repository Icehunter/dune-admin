package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (registers "sqlite")
)

// givePacksStore persists the operator-configurable give-items pack library in
// a local SQLite database. Kept in our own file so we never touch Funcom's
// dune schema. Mirrors welcomeStore / locationStore in structure and intent.
type givePacksStore struct {
	db       *sql.DB
	serverID int
}

// give_packs_config is server-scoped via an integer FK → servers(id) ON DELETE
// CASCADE. Packs live in the surrogate-id give_packs/give_pack_items tables.
const givePacksStoreSchema = `
CREATE TABLE IF NOT EXISTS give_packs_config (
	server_id         INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	base_packs_loaded INTEGER NOT NULL DEFAULT 0,
	updated_at        TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (server_id)
);`

// initGivePacksSchema creates the give_packs_config table on db. Safe to call
// against a shared handle (the unified store). Idempotent.
func initGivePacksSchema(db *sql.DB) error {
	if _, err := db.Exec(givePacksStoreSchema); err != nil {
		return fmt.Errorf("init give-packs schema: %w", err)
	}
	if err := initGivePacksColumnsSchema(db); err != nil {
		return err
	}
	return nil
}

// newGivePacksStore wraps an already-initialised shared handle (schema created
// by openUnifiedStore). Used so all stores share one SQLite file in production.
func newGivePacksStore(db *sql.DB, serverID int) *givePacksStore {
	return &givePacksStore{db: db, serverID: serverID}
}

// openGivePacksStore opens (or creates) the give-packs database at path and
// ensures the schema exists. path may be ":memory:" for tests.
func openGivePacksStore(path string) (*givePacksStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open give-packs store: %w", err)
	}
	if err := initGivePacksSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &givePacksStore{db: db, serverID: defaultServerID}, nil
}

func (s *givePacksStore) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// saveConfig persists the pack library. The give_packs_config row remains the
// canonical home for base_packs_loaded + updated_at (and presence); packs live in
// the typed give_packs/give_pack_items tables. packsJSON must be a valid JSON
// array ("" is treated as empty). basePacksLoaded=true means the default seed has
// been applied; subsequent startups skip re-seeding even when there are zero packs.
func (s *givePacksStore) saveConfig(packsJSON string, basePacksLoaded bool) error {
	loaded := 0
	if basePacksLoaded {
		loaded = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO give_packs_config (server_id, base_packs_loaded, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET
			base_packs_loaded = excluded.base_packs_loaded,
			updated_at        = excluded.updated_at`,
		s.serverID, loaded, now)
	if err != nil {
		return fmt.Errorf("save give-packs config: %w", err)
	}
	packs := []givePack{}
	if packsJSON != "" {
		if err := json.Unmarshal([]byte(packsJSON), &packs); err != nil {
			return fmt.Errorf("parse give-packs json: %w", err)
		}
	}
	if err := saveGivePacksColumns(s.db, s.serverID, packs); err != nil {
		return fmt.Errorf("save give-packs columns: %w", err)
	}
	return nil
}

// loadConfig reads the pack library. base_packs_loaded and presence come from the
// give_packs_config row (ok=false when absent — first boot, caller should seed the
// embedded default); packs come from the typed give_packs/give_pack_items tables,
// re-marshalled to a JSON array (always valid, "[]" when empty).
// Returns (basePacksLoaded, packsJSON, ok, err).
func (s *givePacksStore) loadConfig() (basePacksLoaded bool, packsJSON string, ok bool, err error) {
	var loadedInt int
	scanErr := s.db.QueryRow(`
		SELECT base_packs_loaded FROM give_packs_config WHERE server_id = ?`,
		s.serverID).Scan(&loadedInt)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return false, "", false, nil
	}
	if scanErr != nil {
		return false, "", false, fmt.Errorf("load give-packs config: %w", scanErr)
	}
	packs, loadErr := loadGivePacksColumns(s.db, s.serverID)
	if loadErr != nil {
		return false, "", false, fmt.Errorf("load give-packs columns: %w", loadErr)
	}
	blob, marshalErr := json.Marshal(packs)
	if marshalErr != nil {
		return false, "", false, fmt.Errorf("marshal give-packs: %w", marshalErr)
	}
	return loadedInt != 0, string(blob), true, nil
}
