package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newServersParent(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE servers (id INTEGER PRIMARY KEY, name TEXT, position INTEGER)`); err != nil {
		t.Fatalf("create servers: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (1, 'srv', 0)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return db
}

// A status table left over from an earlier build (keyed by server_id only) must
// be rebuilt with the composite (server_id, guild_id) key so the upsert works.
func TestInitServerDiscordStatusSchema_RebuildsOnPKDrift(t *testing.T) {
	db := newServersParent(t)
	if _, err := db.Exec(`
		CREATE TABLE server_discord_status (
			server_id  INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
			channel_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("create old-shape table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO server_discord_status (server_id, channel_id, message_id) VALUES (1, 'old', 'old')`); err != nil {
		t.Fatalf("seed old row: %v", err)
	}

	if err := initServerDiscordStatusSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	// The composite upsert (the production write) must now succeed.
	store := newSqliteStatusStoreForGuild(db, 1, "guild-1")
	if err := store.saveStatusMessage("ch-1", "msg-1"); err != nil {
		t.Fatalf("save after migration: %v", err)
	}
	ch, msg, err := store.loadStatusMessage()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ch != "ch-1" || msg != "msg-1" {
		t.Errorf("round trip = (%q,%q), want (ch-1,msg-1)", ch, msg)
	}
}

// A table that already has the correct composite key must be left untouched —
// its stored message pointer is preserved (not dropped) so the loop keeps
// editing the same embed.
func TestInitServerDiscordStatusSchema_PreservesCorrectTable(t *testing.T) {
	db := newServersParent(t)
	if err := initServerDiscordStatusSchema(db); err != nil {
		t.Fatalf("first init: %v", err)
	}
	store := newSqliteStatusStoreForGuild(db, 1, "guild-1")
	if err := store.saveStatusMessage("keep-ch", "keep-msg"); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// Re-running schema init must be a no-op that preserves the row.
	if err := initServerDiscordStatusSchema(db); err != nil {
		t.Fatalf("second init: %v", err)
	}
	ch, msg, err := store.loadStatusMessage()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ch != "keep-ch" || msg != "keep-msg" {
		t.Errorf("row not preserved = (%q,%q), want (keep-ch,keep-msg)", ch, msg)
	}
}
