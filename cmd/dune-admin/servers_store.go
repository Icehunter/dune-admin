package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// serversStore persists the list of game servers in the unified SQLite store —
// the live source of truth (config.yaml is only a first-boot import seed). Each
// server's full ServerConfig is stored as a JSON blob; the numeric id is the
// DB-assigned autoincrement primary key.
type serversStore struct{ db *sql.DB }

const serversStoreSchema = `
CREATE TABLE IF NOT EXISTS servers (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT    NOT NULL DEFAULT '',
	position    INTEGER NOT NULL DEFAULT 0,
	config_json TEXT    NOT NULL DEFAULT '{}',
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
// The id inside config_json is irrelevant on read (the column is authoritative).
func (s *serversStore) insertServer(cfg ServerConfig, position int) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	blob, err := json.Marshal(cfg)
	if err != nil {
		return 0, fmt.Errorf("marshal server: %w", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO servers (name, position, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		cfg.Name, position, string(blob), now, now)
	if err != nil {
		return 0, fmt.Errorf("insert server: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("server insert id: %w", err)
	}
	return int(id), nil
}

// updateServer updates an existing server's name + config (by cfg.ID). Position
// is left unchanged.
func (s *serversStore) updateServer(cfg ServerConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	blob, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal server: %w", err)
	}
	if _, err := s.db.Exec(
		`UPDATE servers SET name = ?, config_json = ?, updated_at = ? WHERE id = ?`,
		cfg.Name, string(blob), now, cfg.ID); err != nil {
		return fmt.Errorf("update server %d: %w", cfg.ID, err)
	}
	return nil
}

func (s *serversStore) deleteServer(id int) error {
	if _, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete server %d: %w", id, err)
	}
	return nil
}

// listServers returns all servers in stable (position, id) order, with each
// row's authoritative numeric id stamped onto the returned ServerConfig.
func (s *serversStore) listServers() ([]ServerConfig, error) {
	rows, err := s.db.Query(`SELECT id, config_json FROM servers ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ServerConfig
	for rows.Next() {
		var id int
		var blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		var cfg ServerConfig
		if err := json.Unmarshal([]byte(blob), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal server %d: %w", id, err)
		}
		cfg.ID = id
		cfg.LegacyID = ""
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *serversStore) getServer(id int) (ServerConfig, bool, error) {
	var blob string
	err := s.db.QueryRow(`SELECT config_json FROM servers WHERE id = ?`, id).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerConfig{}, false, nil
	}
	if err != nil {
		return ServerConfig{}, false, fmt.Errorf("get server %d: %w", id, err)
	}
	var cfg ServerConfig
	if err := json.Unmarshal([]byte(blob), &cfg); err != nil {
		return ServerConfig{}, false, fmt.Errorf("unmarshal server %d: %w", id, err)
	}
	cfg.ID = id
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
