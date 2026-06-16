package main

import (
	"context"
	"database/sql"
	"testing"
)

// openMemUnifiedStoreFK opens an in-memory unified store via the real
// openUnifiedStore so foreign_keys enforcement (and MaxOpenConns(1)) is active.
func openMemUnifiedStoreFK(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertTestServer(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO servers (name, position, created_at, updated_at) VALUES (?, 0, '', '')`,
		name)
	if err != nil {
		t.Fatalf("insert server: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("server id: %v", err)
	}
	return id
}

func TestForeignKeysPragmaOn(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	var v int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&v); err != nil {
		t.Fatalf("pragma read: %v", err)
	}
	if v != 1 {
		t.Fatalf("foreign_keys = %d, want 1 (cascade would not be enforced)", v)
	}
}

func TestServerDiscordStatusCascade(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	id := insertTestServer(t, db, "S")
	if _, err := db.Exec(
		`INSERT INTO server_discord_status (server_id, guild_id, channel_id, message_id)
		 VALUES (?, 'g', 'c', 'm')`, id); err != nil {
		t.Fatalf("insert status: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM servers WHERE id = ?`, id); err != nil {
		t.Fatalf("delete server: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM server_discord_status WHERE server_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("server_discord_status not cascaded: %d rows remain", n)
	}
}

func TestServerDiscordStatusRejectsOrphan(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	// No server with id 999 → FK must reject the insert.
	if _, err := db.Exec(
		`INSERT INTO server_discord_status (server_id, guild_id, channel_id, message_id)
		 VALUES (999, 'g', 'c', 'm')`); err == nil {
		t.Fatal("insert with dangling server_id succeeded; FK not enforced")
	}
}

func TestWithForeignKeysDisabled(t *testing.T) {
	db := openMemUnifiedStoreFK(t)
	ctx := context.Background()

	// Inside the helper, FK is OFF so an orphan insert is allowed; we delete it
	// before returning so foreign_key_check stays clean.
	err := withForeignKeysDisabled(ctx, db, func(conn *sql.Conn) error {
		var v int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&v); err != nil {
			return err
		}
		if v != 0 {
			t.Errorf("foreign_keys = %d inside helper, want 0", v)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO server_discord_status (server_id) VALUES (999)`); err != nil {
			t.Errorf("orphan insert rejected with FK off: %v", err)
		}
		_, err := conn.ExecContext(ctx, `DELETE FROM server_discord_status WHERE server_id = 999`)
		return err
	})
	if err != nil {
		t.Fatalf("withForeignKeysDisabled: %v", err)
	}

	// Restored afterwards on a fresh pool connection.
	var v int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&v); err != nil {
		t.Fatalf("pragma read after: %v", err)
	}
	if v != 1 {
		t.Errorf("foreign_keys = %d after helper, want 1 (restored)", v)
	}
}
