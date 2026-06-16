package main

import (
	"database/sql"
	"fmt"
)

// give_packs_columns.go stores the give-items pack library as surrogate-id child
// tables (give_packs + give_pack_items). give_packs is server-scoped via an
// integer FK → servers(id) ON DELETE CASCADE; give_pack_items links to its
// parent pack by the parent's surrogate id (not the text business pack id).
// Slice order is preserved via a position column.

const givePacksColumnsSchema = `
CREATE TABLE IF NOT EXISTS give_packs (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	pack_id   TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '',
	tier INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0,
	UNIQUE (server_id, pack_id)
);
CREATE INDEX IF NOT EXISTS idx_give_packs_server ON give_packs(server_id);
CREATE TABLE IF NOT EXISTS give_pack_items (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	pack_id  INTEGER NOT NULL REFERENCES give_packs(id) ON DELETE CASCADE,
	position INTEGER NOT NULL DEFAULT 0,
	template TEXT NOT NULL DEFAULT '', qty INTEGER NOT NULL DEFAULT 0, quality INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_give_pack_items_pack ON give_pack_items(pack_id);`

// initGivePacksColumnsSchema creates the give_packs and give_pack_items tables.
// Idempotent.
func initGivePacksColumnsSchema(db *sql.DB) error {
	if _, err := db.Exec(givePacksColumnsSchema); err != nil {
		return fmt.Errorf("init give-packs columns schema: %w", err)
	}
	return nil
}

// saveGivePacksColumns replaces all packs for serverID with packs, preserving
// slice order via the position column. Existing rows for serverID are deleted
// first (item cascade fires) so the write is a full replacement.
func saveGivePacksColumns(db dbExecer, serverID int, packs []givePack) error {
	if _, err := db.Exec(`DELETE FROM give_packs WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("clear give_packs %d: %w", serverID, err)
	}
	for pos, pack := range packs {
		res, err := db.Exec(`INSERT INTO give_packs (server_id, pack_id, name, category, tier, position)
			VALUES (?, ?, ?, ?, ?, ?)`,
			serverID, pack.ID, pack.Name, pack.Category, pack.Tier, pos)
		if err != nil {
			return fmt.Errorf("insert give_pack %d/%s: %w", serverID, pack.ID, err)
		}
		packID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("give_pack id %d/%s: %w", serverID, pack.ID, err)
		}
		for itemPos, item := range pack.Items {
			if _, err := db.Exec(`INSERT INTO give_pack_items
				(pack_id, position, template, qty, quality)
				VALUES (?, ?, ?, ?, ?)`,
				packID, itemPos, item.Template, item.Qty, item.Quality); err != nil {
				return fmt.Errorf("insert give_pack_item %d/%s[%d]: %w", serverID, pack.ID, itemPos, err)
			}
		}
	}
	return nil
}

// loadGivePacksColumns rebuilds the ordered []givePack for serverID from the two
// child tables. Items are fetched once and grouped by pack surrogate id in Go to
// avoid a query-during-rows-iteration conflict on the same connection.
func loadGivePacksColumns(db dbRowQueryer, serverID int) ([]givePack, error) {
	q, ok := db.(givePacksQueryer)
	if !ok {
		return nil, fmt.Errorf("loadGivePacksColumns: db does not support Query")
	}
	packs, order, byID, err := loadGivePackRows(q, serverID)
	if err != nil {
		return nil, err
	}
	if err := attachGivePackItems(q, serverID, byID); err != nil {
		return nil, err
	}
	out := make([]givePack, 0, len(order))
	for _, id := range order {
		out = append(out, *packs[id])
	}
	return out, nil
}

type givePacksQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// loadGivePackRows reads give_packs for serverID ordered by position, returning
// an id→pack map (items empty), the surrogate-id order slice, and the map alias.
func loadGivePackRows(db givePacksQueryer, serverID int) (map[int64]*givePack, []int64, map[int64]*givePack, error) {
	rows, err := db.Query(`SELECT id, pack_id, name, category, tier FROM give_packs
		WHERE server_id = ? ORDER BY position`, serverID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query give_packs %d: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	packs := make(map[int64]*givePack)
	var order []int64
	for rows.Next() {
		var id int64
		var p givePack
		if err := rows.Scan(&id, &p.ID, &p.Name, &p.Category, &p.Tier); err != nil {
			return nil, nil, nil, fmt.Errorf("scan give_pack: %w", err)
		}
		p.Items = []welcomePackageItem{}
		packs[id] = &p
		order = append(order, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate give_packs: %w", err)
	}
	return packs, order, packs, nil
}

// attachGivePackItems reads all give_pack_items for serverID's packs ordered by
// pack then position and appends each into its parent pack.
func attachGivePackItems(db givePacksQueryer, serverID int, byID map[int64]*givePack) error {
	rows, err := db.Query(`SELECT i.pack_id, i.template, i.qty, i.quality
		FROM give_pack_items i
		JOIN give_packs p ON p.id = i.pack_id
		WHERE p.server_id = ? ORDER BY i.pack_id, i.position`, serverID)
	if err != nil {
		return fmt.Errorf("query give_pack_items %d: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var packID int64
		var item welcomePackageItem
		if err := rows.Scan(&packID, &item.Template, &item.Qty, &item.Quality); err != nil {
			return fmt.Errorf("scan give_pack_item: %w", err)
		}
		if p, ok := byID[packID]; ok {
			p.Items = append(p.Items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate give_pack_items: %w", err)
	}
	return nil
}
