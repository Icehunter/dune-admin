package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// welcome_columns.go stores the welcome-package library and the active-version
// list as typed child tables (welcome_packages + welcome_package_items +
// welcome_active_versions) instead of the welcome_config.packages_json and
// welcome_config.active_versions_json blobs. All three tables are server-scoped
// and preserve slice order via a position column. The welcomePackage struct and
// its json tags are unchanged; only storage moves to columns. The blob columns
// are kept (packages_json written as '[]', active_versions_json as '') but no
// longer authoritative once migrated (see migrateWelcomeColumns).

const welcomeColumnsSchema = `
CREATE TABLE IF NOT EXISTS welcome_packages (
	server_id TEXT NOT NULL, version TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (server_id, version)
);
CREATE TABLE IF NOT EXISTS welcome_package_items (
	server_id TEXT NOT NULL, version TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0,
	template TEXT NOT NULL DEFAULT '', qty INTEGER NOT NULL DEFAULT 0, quality INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (server_id, version, position)
);
CREATE TABLE IF NOT EXISTS welcome_active_versions (
	server_id TEXT NOT NULL, version TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (server_id, version)
);`

// initWelcomeColumnsSchema creates the three welcome column tables. Idempotent.
func initWelcomeColumnsSchema(db *sql.DB) error {
	if _, err := db.Exec(welcomeColumnsSchema); err != nil {
		return fmt.Errorf("init welcome columns schema: %w", err)
	}
	return nil
}

// saveWelcomePackagesColumns replaces all welcome rows for serverID with the
// given packages and active versions, preserving slice order via position.
// Existing rows for serverID are deleted first so the write is a full
// replacement (matching the blobs' all-or-nothing semantics).
func saveWelcomePackagesColumns(db dbExecer, serverID string, packages []welcomePackage, activeVersions []string) error {
	if _, err := db.Exec(`DELETE FROM welcome_package_items WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear welcome_package_items %s: %w", serverID, err)
	}
	if _, err := db.Exec(`DELETE FROM welcome_packages WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear welcome_packages %s: %w", serverID, err)
	}
	if _, err := db.Exec(`DELETE FROM welcome_active_versions WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear welcome_active_versions %s: %w", serverID, err)
	}
	for pos, pkg := range packages {
		if _, err := db.Exec(`INSERT INTO welcome_packages (server_id, version, position)
			VALUES (?, ?, ?)`, serverID, pkg.Version, pos); err != nil {
			return fmt.Errorf("insert welcome_package %s/%s: %w", serverID, pkg.Version, err)
		}
		for itemPos, item := range pkg.Items {
			if _, err := db.Exec(`INSERT INTO welcome_package_items
				(server_id, version, position, template, qty, quality)
				VALUES (?, ?, ?, ?, ?, ?)`,
				serverID, pkg.Version, itemPos, item.Template, item.Qty, item.Quality); err != nil {
				return fmt.Errorf("insert welcome_package_item %s/%s[%d]: %w", serverID, pkg.Version, itemPos, err)
			}
		}
	}
	for pos, v := range activeVersions {
		if _, err := db.Exec(`INSERT INTO welcome_active_versions (server_id, version, position)
			VALUES (?, ?, ?)`, serverID, v, pos); err != nil {
			return fmt.Errorf("insert welcome_active_version %s/%s: %w", serverID, v, err)
		}
	}
	return nil
}

// loadWelcomePackagesColumns rebuilds the ordered []welcomePackage and the
// active-version list for serverID from the three child tables. Items are
// fetched once and grouped by version in Go to avoid a query-during-rows
// iteration conflict on the same connection.
func loadWelcomePackagesColumns(db dbRowQueryer, serverID string) ([]welcomePackage, []string, error) {
	q, ok := db.(welcomeColumnsQueryer)
	if !ok {
		return nil, nil, fmt.Errorf("loadWelcomePackagesColumns: db does not support Query")
	}
	packages, order, err := loadWelcomePackageRows(q, serverID)
	if err != nil {
		return nil, nil, err
	}
	if err := attachWelcomePackageItems(q, serverID, packages); err != nil {
		return nil, nil, err
	}
	out := make([]welcomePackage, 0, len(order))
	for _, v := range order {
		out = append(out, *packages[v])
	}
	active, err := loadWelcomeActiveVersions(q, serverID)
	if err != nil {
		return nil, nil, err
	}
	return out, active, nil
}

type welcomeColumnsQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// loadWelcomePackageRows reads welcome_packages for serverID ordered by
// position, returning a version→package map (items empty) plus the version
// order slice.
func loadWelcomePackageRows(db welcomeColumnsQueryer, serverID string) (map[string]*welcomePackage, []string, error) {
	rows, err := db.Query(`SELECT version FROM welcome_packages
		WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, nil, fmt.Errorf("query welcome_packages %s: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	packages := make(map[string]*welcomePackage)
	var order []string
	for rows.Next() {
		var p welcomePackage
		if err := rows.Scan(&p.Version); err != nil {
			return nil, nil, fmt.Errorf("scan welcome_package: %w", err)
		}
		p.Items = []welcomePackageItem{}
		packages[p.Version] = &p
		order = append(order, p.Version)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate welcome_packages: %w", err)
	}
	return packages, order, nil
}

// attachWelcomePackageItems reads all welcome_package_items for serverID ordered
// by position and appends each into its parent package. Items for unknown
// versions are skipped.
func attachWelcomePackageItems(db welcomeColumnsQueryer, serverID string, packages map[string]*welcomePackage) error {
	rows, err := db.Query(`SELECT version, template, qty, quality FROM welcome_package_items
		WHERE server_id = ? ORDER BY version, position`, serverID)
	if err != nil {
		return fmt.Errorf("query welcome_package_items %s: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var version string
		var item welcomePackageItem
		if err := rows.Scan(&version, &item.Template, &item.Qty, &item.Quality); err != nil {
			return fmt.Errorf("scan welcome_package_item: %w", err)
		}
		if p, ok := packages[version]; ok {
			p.Items = append(p.Items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate welcome_package_items: %w", err)
	}
	return nil
}

// loadWelcomeActiveVersions reads welcome_active_versions for serverID ordered
// by position.
func loadWelcomeActiveVersions(db welcomeColumnsQueryer, serverID string) ([]string, error) {
	rows, err := db.Query(`SELECT version FROM welcome_active_versions
		WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("query welcome_active_versions %s: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan welcome_active_version: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// legacyWelcomeBlob is one decoded welcome_config row (packages + active
// versions) keyed by server_id.
type legacyWelcomeBlob struct {
	serverID       string
	packages       []welcomePackage
	activeVersions []string
}

// decodeJSONList tolerates the empty / "null" / "[]" blob forms emitted by the
// legacy welcome_config columns, decoding any of them to an empty slice.
func decodeJSONList(blob string, out any) error {
	if blob == "" || blob == "null" || blob == "[]" {
		return nil
	}
	return json.Unmarshal([]byte(blob), out)
}

// readLegacyWelcomeBlobs decodes every welcome_config.packages_json /
// active_versions_json pair into typed slices keyed by server_id. The rows are
// fully buffered so callers can write back without a query-during-rows conflict
// on the same transaction.
func readLegacyWelcomeBlobs(tx *sql.Tx) ([]legacyWelcomeBlob, error) {
	rows, err := tx.Query(`SELECT server_id, packages_json, active_versions_json FROM welcome_config`)
	if err != nil {
		return nil, fmt.Errorf("read legacy welcome blobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []legacyWelcomeBlob
	for rows.Next() {
		var rec legacyWelcomeBlob
		var packagesJSON, activeVersionsJSON string
		if err := rows.Scan(&rec.serverID, &packagesJSON, &activeVersionsJSON); err != nil {
			return nil, fmt.Errorf("scan legacy welcome: %w", err)
		}
		if err := decodeJSONList(packagesJSON, &rec.packages); err != nil {
			return nil, fmt.Errorf("unmarshal legacy welcome packages %s: %w", rec.serverID, err)
		}
		if err := decodeJSONList(activeVersionsJSON, &rec.activeVersions); err != nil {
			return nil, fmt.Errorf("unmarshal legacy welcome active versions %s: %w", rec.serverID, err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// migrateWelcomeColumns translates each legacy welcome_config.packages_json /
// active_versions_json pair into the typed welcome child tables, once, guarded
// by the migrated:welcome_columns marker. After this runs the blobs are never
// read again.
func migrateWelcomeColumns(db *sql.DB) error {
	return runColumnMigrationOnce(db, "migrated:welcome_columns", func(tx *sql.Tx) error {
		blobs, err := readLegacyWelcomeBlobs(tx)
		if err != nil {
			return err
		}
		for _, rec := range blobs {
			if err := saveWelcomePackagesColumns(tx, rec.serverID, rec.packages, rec.activeVersions); err != nil {
				return err
			}
		}
		return nil
	})
}
