package main

import (
	"database/sql"
	"fmt"
)

// welcome_columns.go stores the welcome-package library and the active-version
// list as surrogate-id child tables (welcome_packages + welcome_package_items +
// welcome_active_versions). Packages and active versions are server-scoped via
// an integer FK → servers(id) ON DELETE CASCADE; items link to their parent
// package by its surrogate id. Slice order is preserved via a position column.

const welcomeColumnsSchema = `
CREATE TABLE IF NOT EXISTS welcome_packages (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	version   TEXT NOT NULL,
	position  INTEGER NOT NULL DEFAULT 0,
	UNIQUE (server_id, version)
);
CREATE INDEX IF NOT EXISTS idx_welcome_packages_server ON welcome_packages(server_id);
CREATE TABLE IF NOT EXISTS welcome_package_items (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	package_id INTEGER NOT NULL REFERENCES welcome_packages(id) ON DELETE CASCADE,
	position   INTEGER NOT NULL DEFAULT 0,
	template   TEXT NOT NULL DEFAULT '', qty INTEGER NOT NULL DEFAULT 0, quality INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_welcome_package_items_pkg ON welcome_package_items(package_id);
CREATE TABLE IF NOT EXISTS welcome_active_versions (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	version   TEXT NOT NULL,
	position  INTEGER NOT NULL DEFAULT 0,
	UNIQUE (server_id, version)
);
CREATE INDEX IF NOT EXISTS idx_welcome_active_versions_server ON welcome_active_versions(server_id);`

// initWelcomeColumnsSchema creates the three welcome child tables. Idempotent.
func initWelcomeColumnsSchema(db *sql.DB) error {
	if _, err := db.Exec(welcomeColumnsSchema); err != nil {
		return fmt.Errorf("init welcome columns schema: %w", err)
	}
	return nil
}

// saveWelcomePackagesColumns replaces all welcome rows for serverID with the
// given packages and active versions, preserving slice order via position.
// Existing rows for serverID are deleted first (item cascade fires) so the write
// is a full replacement (matching the blobs' all-or-nothing semantics).
func saveWelcomePackagesColumns(db dbExecer, serverID int, packages []welcomePackage, activeVersions []string) error {
	if _, err := db.Exec(`DELETE FROM welcome_packages WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear welcome_packages %d: %w", serverID, err)
	}
	if _, err := db.Exec(`DELETE FROM welcome_active_versions WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear welcome_active_versions %d: %w", serverID, err)
	}
	for pos, pkg := range packages {
		res, err := db.Exec(`INSERT INTO welcome_packages (server_id, version, position)
			VALUES (?, ?, ?)`, serverID, pkg.Version, pos)
		if err != nil {
			return fmt.Errorf("insert welcome_package %d/%s: %w", serverID, pkg.Version, err)
		}
		pkgID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("welcome_package id %d/%s: %w", serverID, pkg.Version, err)
		}
		for itemPos, item := range pkg.Items {
			if _, err := db.Exec(`INSERT INTO welcome_package_items
				(package_id, position, template, qty, quality)
				VALUES (?, ?, ?, ?, ?)`,
				pkgID, itemPos, item.Template, item.Qty, item.Quality); err != nil {
				return fmt.Errorf("insert welcome_package_item %d/%s[%d]: %w", serverID, pkg.Version, itemPos, err)
			}
		}
	}
	for pos, v := range activeVersions {
		if _, err := db.Exec(`INSERT INTO welcome_active_versions (server_id, version, position)
			VALUES (?, ?, ?)`, serverID, v, pos); err != nil {
			return fmt.Errorf("insert welcome_active_version %d/%s: %w", serverID, v, err)
		}
	}
	return nil
}

// loadWelcomePackagesColumns rebuilds the ordered []welcomePackage and the
// active-version list for serverID from the three child tables. Items are
// fetched once and grouped by package surrogate id in Go to avoid a
// query-during-rows iteration conflict on the same connection.
func loadWelcomePackagesColumns(db dbRowQueryer, serverID int) ([]welcomePackage, []string, error) {
	q, ok := db.(welcomeColumnsQueryer)
	if !ok {
		return nil, nil, fmt.Errorf("loadWelcomePackagesColumns: db does not support Query")
	}
	packages, order, byID, err := loadWelcomePackageRows(q, serverID)
	if err != nil {
		return nil, nil, err
	}
	if err := attachWelcomePackageItems(q, serverID, byID); err != nil {
		return nil, nil, err
	}
	out := make([]welcomePackage, 0, len(order))
	for _, id := range order {
		out = append(out, *packages[id])
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
// position, returning an id→package map (items empty), the surrogate-id order
// slice, and the same map (alias) for item attachment.
func loadWelcomePackageRows(db welcomeColumnsQueryer, serverID int) (map[int64]*welcomePackage, []int64, map[int64]*welcomePackage, error) {
	rows, err := db.Query(`SELECT id, version FROM welcome_packages
		WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query welcome_packages %d: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	packages := make(map[int64]*welcomePackage)
	var order []int64
	for rows.Next() {
		var id int64
		var p welcomePackage
		if err := rows.Scan(&id, &p.Version); err != nil {
			return nil, nil, nil, fmt.Errorf("scan welcome_package: %w", err)
		}
		p.Items = []welcomePackageItem{}
		packages[id] = &p
		order = append(order, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate welcome_packages: %w", err)
	}
	return packages, order, packages, nil
}

// attachWelcomePackageItems reads all welcome_package_items for serverID's
// packages ordered by package then position and appends each into its parent.
func attachWelcomePackageItems(db welcomeColumnsQueryer, serverID int, byID map[int64]*welcomePackage) error {
	rows, err := db.Query(`SELECT i.package_id, i.template, i.qty, i.quality
		FROM welcome_package_items i
		JOIN welcome_packages p ON p.id = i.package_id
		WHERE p.server_id = ? ORDER BY i.package_id, i.position`, serverID)
	if err != nil {
		return fmt.Errorf("query welcome_package_items %d: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pkgID int64
		var item welcomePackageItem
		if err := rows.Scan(&pkgID, &item.Template, &item.Qty, &item.Quality); err != nil {
			return fmt.Errorf("scan welcome_package_item: %w", err)
		}
		if p, ok := byID[pkgID]; ok {
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
func loadWelcomeActiveVersions(db welcomeColumnsQueryer, serverID int) ([]string, error) {
	rows, err := db.Query(`SELECT version FROM welcome_active_versions
		WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("query welcome_active_versions %d: %w", serverID, err)
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
