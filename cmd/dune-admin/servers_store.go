package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// serversStore persists the list of game servers in the unified SQLite store —
// the live source of truth (config.yaml is only a first-boot import seed). Each
// server's ServerConfig is stored across typed columns (see servers_columns.go);
// the legacy config_json blob is retained as '{}' but no longer authoritative.
// The numeric id is the DB-assigned autoincrement primary key.
type serversStore struct{ db *sql.DB }

const serversStoreSchema = `
CREATE TABLE IF NOT EXISTS servers (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT    NOT NULL DEFAULT '',
	position    INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT    NOT NULL DEFAULT '',
	updated_at  TEXT    NOT NULL DEFAULT ''
);`

func initServersSchema(db *sql.DB) error {
	if _, err := db.Exec(serversStoreSchema); err != nil {
		return fmt.Errorf("init servers schema: %w", err)
	}
	return nil
}

func newServersStore(db *sql.DB) *serversStore { return &serversStore{db: db} }

// insertServer inserts cfg as a new server row and returns the DB-assigned id.
// The row is created first, then the typed columns are populated via
// writeServerColumns.
func (s *serversStore) insertServer(cfg ServerConfig, position int) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO servers (name, position, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		cfg.Name, position, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert server: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("server insert id: %w", err)
	}
	id := int(id64)
	if err := writeServerColumns(s.db, id, cfg); err != nil {
		return 0, err
	}
	return id, nil
}

// updateServer updates an existing server's name + typed columns (by cfg.ID).
// Position is left unchanged.
func (s *serversStore) updateServer(cfg ServerConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE servers SET name = ?, updated_at = ? WHERE id = ?`,
		cfg.Name, now, cfg.ID); err != nil {
		return fmt.Errorf("update server %d: %w", cfg.ID, err)
	}
	return writeServerColumns(s.db, cfg.ID, cfg)
}

func (s *serversStore) deleteServer(id int) error {
	if _, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete server %d: %w", id, err)
	}
	return nil
}

// listServers returns all servers in stable (position, id) order, reading each
// row's typed columns and stamping name + the authoritative numeric id.
func (s *serversStore) listServers() ([]ServerConfig, error) {
	rows, err := s.db.Query(`SELECT id, name FROM servers ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type idName struct {
		id   int
		name string
	}
	var ids []idName
	for rows.Next() {
		var rec idName
		if err := rows.Scan(&rec.id, &rec.name); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		ids = append(ids, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ServerConfig
	for _, rec := range ids {
		cfg, err := readServerColumns(s.db, rec.id)
		if err != nil {
			return nil, fmt.Errorf("read server %d: %w", rec.id, err)
		}
		cfg.ID = rec.id
		cfg.Name = rec.name
		cfg.LegacyID = ""
		out = append(out, cfg)
	}
	return out, nil
}

func (s *serversStore) getServer(id int) (ServerConfig, bool, error) {
	var name string
	err := s.db.QueryRow(`SELECT name FROM servers WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerConfig{}, false, nil
	}
	if err != nil {
		return ServerConfig{}, false, fmt.Errorf("get server %d: %w", id, err)
	}
	cfg, err := readServerColumns(s.db, id)
	if err != nil {
		return ServerConfig{}, false, fmt.Errorf("read server %d: %w", id, err)
	}
	cfg.ID = id
	cfg.Name = name
	cfg.LegacyID = ""
	return cfg, true, nil
}

func (s *serversStore) hasAnyServer() (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM servers)`).Scan(&n); err != nil {
		return false, fmt.Errorf("count servers: %w", err)
	}
	return n != 0, nil
}

func (s *serversStore) nextPosition() (int, error) {
	var pos int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM servers`).Scan(&pos); err != nil {
		return 0, fmt.Errorf("next position: %w", err)
	}
	return pos, nil
}
