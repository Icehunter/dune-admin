package main

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestRemodel_PreservesEventRewardJSON proves the migration keeps event
// config_json / reward_json verbatim — including fields the backend doesn't
// model (faction_scrip) and character XP — now that events are opaque JSON
// columns rather than decomposed into typed tables.
func TestRemodel_PreservesEventRewardJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ev.db")
	const rewardJSON = `{"currency":10000,"faction_scrip":500,"xp":[{"track":"character","amount":10000}],"items":[{"template":"Ammo","qty":5,"quality":0}]}`

	// 0.39.5-shaped event_definitions: no server_id, reward in reward_json.
	seed, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE event_definitions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1,
		config_json TEXT NOT NULL DEFAULT '{}', reward_json TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := seed.Exec(
		`INSERT INTO event_definitions (name, type, enabled, version, config_json, reward_json, created_at, updated_at)
		 VALUES ('Race','milestone',1,1,'{"signal":"level","threshold":5}',?, '2024-01-01','2024-01-01')`,
		rewardJSON); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	_ = seed.Close()

	db, err := openUnifiedStore(path)
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`INSERT INTO servers (id,name,position,created_at,updated_at) VALUES (1,'D',0,'','')`); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	migrateUnifiedRemodel(db, 1)

	var gotReward string
	if err := db.QueryRow(`SELECT reward_json FROM event_definitions WHERE id = 1`).Scan(&gotReward); err != nil {
		t.Fatalf("read reward_json after migration: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(gotReward), &got); err != nil {
		t.Fatalf("reward_json not valid JSON after migration (%q): %v", gotReward, err)
	}
	if got["faction_scrip"] != float64(500) {
		t.Errorf("faction_scrip lost: %v", got["faction_scrip"])
	}
	xp, _ := got["xp"].([]any)
	if len(xp) != 1 {
		t.Fatalf("xp lost: %v", got["xp"])
	}
	x0, _ := xp[0].(map[string]any)
	if x0["track"] != "character" || x0["amount"] != float64(10000) {
		t.Errorf("character xp 10000 lost: %v", x0)
	}
}
