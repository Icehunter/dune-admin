package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// locationStore persists the admin-editable teleport/spawn location list in a
// local SQLite database. It mirrors the welcomeStore pattern: one file in
// configDir(), opened once at startup, never touched by Funcom's dune schema.
type locationStore struct {
	db *sql.DB
}

const locationStoreSchema = `
CREATE TABLE IF NOT EXISTS map_locations (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT    NOT NULL UNIQUE,
	x          REAL    NOT NULL DEFAULT 0,
	y          REAL    NOT NULL DEFAULT 0,
	z          REAL    NOT NULL DEFAULT 0,
	sort       INTEGER NOT NULL DEFAULT 0,
	created_at TEXT    NOT NULL,
	updated_at TEXT    NOT NULL
);`

// initLocationSchema creates the map_locations table on db. Safe to call
// against a shared handle (the unified store). Idempotent.
func initLocationSchema(db *sql.DB) error {
	if _, err := db.Exec(locationStoreSchema); err != nil {
		return fmt.Errorf("init location schema: %w", err)
	}
	return nil
}

// newLocationStore wraps an already-initialised shared handle (schema created by
// openUnifiedStore). Seeding must be done by the caller via seedIfEmpty().
func newLocationStore(db *sql.DB) *locationStore {
	return &locationStore{db: db}
}

// openLocationStore opens (or creates) the location database at path, ensures
// the schema exists, and seeds from cheatLocations when the table is empty.
func openLocationStore(path string) (*locationStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open location store: %w", err)
	}
	if err := initLocationSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &locationStore{db: db}
	if err := s.seedIfEmpty(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed location store: %w", err)
	}
	return s, nil
}

func (s *locationStore) close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// seedIfEmpty inserts the hardcoded cheatLocations when the table has no rows.
// This preserves existing admin-edited data on subsequent startups.
func (s *locationStore) seedIfEmpty() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM map_locations`).Scan(&count); err != nil {
		return fmt.Errorf("count locations: %w", err)
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i, loc := range cheatLocations {
		_, err := s.db.Exec(
			`INSERT INTO map_locations (name, x, y, z, sort, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			loc.Name, loc.X, loc.Y, loc.Z, i, now, now)
		if err != nil {
			return fmt.Errorf("seed location %q: %w", loc.Name, err)
		}
	}
	return nil
}

// list returns all locations ordered by sort index, then name.
func (s *locationStore) list() ([]teleportLocation, error) {
	rows, err := s.db.Query(
		`SELECT name, x, y, z FROM map_locations ORDER BY sort ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]teleportLocation, 0)
	for rows.Next() {
		var loc teleportLocation
		if err := rows.Scan(&loc.Name, &loc.X, &loc.Y, &loc.Z); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		out = append(out, loc)
	}
	return out, rows.Err()
}

// upsert inserts a new location or updates its coordinates if the name already
// exists. An empty name is rejected.
func (s *locationStore) upsert(name string, x, y, z float64) error {
	if name == "" {
		return fmt.Errorf("location name must not be empty")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO map_locations (name, x, y, z, sort, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			z = excluded.z,
			updated_at = excluded.updated_at`,
		name, x, y, z, now, now)
	if err != nil {
		return fmt.Errorf("upsert location %q: %w", name, err)
	}
	return nil
}

// rename changes the display name of a location. Returns an error if the old
// name does not exist.
func (s *locationStore) rename(oldName, newName string) error {
	if oldName == "" || newName == "" {
		return fmt.Errorf("location names must not be empty")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE map_locations SET name = ?, updated_at = ? WHERE name = ?`,
		newName, now, oldName)
	if err != nil {
		return fmt.Errorf("rename location %q → %q: %w", oldName, newName, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("location %q not found", oldName)
	}
	return nil
}

// delete removes a location by name. Returns an error if it does not exist.
func (s *locationStore) delete(name string) error {
	if name == "" {
		return fmt.Errorf("location name must not be empty")
	}
	res, err := s.db.Exec(`DELETE FROM map_locations WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete location %q: %w", name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("location %q not found", name)
	}
	return nil
}
