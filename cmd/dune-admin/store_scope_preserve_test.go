package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// A legacy scoped table that already carries distinct numeric server ids (a
// 0.40.0 multi-server store) must keep them through the text→int rebuild; only a
// non-numeric/absent scope (0.39.5 'default') falls back to the default id. A
// regression here collapses multi-server data onto one server and aborts on the
// composite-PK duplicate.
func TestRebuildLegacyServerIDToInt_PreservesNumericScope(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1) // keep the single :memory: db across connections
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE foo (
		server_id TEXT NOT NULL, k TEXT NOT NULL, v INTEGER,
		PRIMARY KEY (server_id, k)
	)`); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	// Two distinct servers share business key 'a' (would collide if both mapped
	// to the default id); plus a legacy 'default' row.
	if _, err := db.Exec(`INSERT INTO foo (server_id, k, v) VALUES
		('1', 'a', 10), ('2', 'a', 20), ('default', 'b', 30)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	newDDL := `CREATE TABLE foo_int (
		server_id INTEGER NOT NULL, k TEXT NOT NULL, v INTEGER,
		PRIMARY KEY (server_id, k)
	)`
	if err := rebuildLegacyServerIDToInt(db, "foo", "foo_int", newDDL, []string{"k", "v"}, 1); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	rows, err := db.Query(`SELECT server_id, k, v FROM foo ORDER BY server_id, k`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	type rec struct {
		sid int
		k   string
		v   int
	}
	var got []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.sid, &r.k, &r.v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	want := []rec{{1, "a", 10}, {1, "b", 30}, {2, "a", 20}}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
