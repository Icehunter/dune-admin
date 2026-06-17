package main

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"strings"
	"testing"
)

// setGlobalStoreForTest wires db as globalStore and restores on cleanup.
func setGlobalStoreForTest(t *testing.T, db *sql.DB) {
	t.Helper()
	orig := globalStore
	globalStore = db
	t.Cleanup(func() { globalStore = orig })
}

// ── noServerConfigured ────────────────────────────────────────────────────────

func TestNoServerConfigured_EmptyUnifiedStore(t *testing.T) {
	db := openMemUnifiedStore(t)
	setGlobalStoreForTest(t, db)
	if !noServerConfigured() {
		t.Error("noServerConfigured() = false, want true for empty servers table")
	}
}

func TestNoServerConfigured_WithServerRow(t *testing.T) {
	db := openMemUnifiedStore(t)
	seedDefaultServer(t, db)
	setGlobalStoreForTest(t, db)
	if noServerConfigured() {
		t.Error("noServerConfigured() = true, want false when server row exists")
	}
}

func TestNoServerConfigured_NilGlobalStore(t *testing.T) {
	orig := globalStore
	globalStore = nil
	t.Cleanup(func() { globalStore = orig })
	if noServerConfigured() {
		t.Error("noServerConfigured() = true, want false for nil global store (standalone path)")
	}
}

// ── applyWelcomeConfigFromStore ───────────────────────────────────────────────

// TestApplyWelcomeConfigFromStore_FreshInstall verifies that calling
// applyWelcomeConfigFromStore on a unified store with no server row (the
// Windows fresh-install case) returns nil rather than a FK constraint error.
func TestApplyWelcomeConfigFromStore_FreshInstall(t *testing.T) {
	db := openMemUnifiedStore(t)
	setGlobalStoreForTest(t, db)

	origWelcome := welcomeStoreDB
	welcomeStoreDB = newWelcomeStore(db, defaultServerID)
	t.Cleanup(func() { welcomeStoreDB = origWelcome })

	if err := applyWelcomeConfigFromStore(); err != nil {
		t.Errorf("applyWelcomeConfigFromStore() = %v, want nil on fresh install (no server row)", err)
	}
}

// ── initGivePacksStore ────────────────────────────────────────────────────────

// captureStderr redirects os.Stderr and returns a function that stops the
// capture and returns whatever was written.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	return func() string {
		_ = w.Close()
		os.Stderr = old
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}
}

// TestInitGivePacksStore_FreshInstall verifies that initGivePacksStore on a
// unified store with no server row does not print a give-packs FK error to
// stderr (the Windows fresh-install case).
func TestInitGivePacksStore_FreshInstall(t *testing.T) {
	db := openMemUnifiedStore(t)
	setGlobalStoreForTest(t, db)

	origGP := givePacksStoreDB
	givePacksStoreDB = nil
	t.Cleanup(func() { givePacksStoreDB = origGP })

	stop := captureStderr(t)
	initGivePacksStore()
	captured := stop()

	if strings.Contains(captured, "give-packs seed") {
		t.Errorf("seedGivePacks was called with no server row (FK error):\n%s", captured)
	}
	if givePacksStoreDB == nil {
		t.Error("givePacksStoreDB is nil after initGivePacksStore, want non-nil")
	}
}

// ── applyBattlepassEngine ─────────────────────────────────────────────────────

// TestApplyBattlepassEngine_FreshInstall verifies that applyBattlepassEngine
// on a unified store with no server row does not panic and leaves the store
// empty (seed skipped). After a server row is added, a second call seeds the
// default catalog.
func TestApplyBattlepassEngine_FreshInstall(t *testing.T) {
	db := openMemUnifiedStore(t)
	setGlobalStoreForTest(t, db)

	origBP := globalBattlepassStore
	globalBattlepassStore = newBattlepassStore(db, defaultServerID)
	t.Cleanup(func() { globalBattlepassStore = origBP })

	// Fresh install: no server row — must not panic.
	applyBattlepassEngine(appConfig{})

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM battlepass_tiers`).Scan(&count); err != nil {
		t.Fatalf("count battlepass_tiers: %v", err)
	}
	if count != 0 {
		t.Errorf("fresh install: got %d tiers, want 0 (seed should be skipped)", count)
	}

	// Add server row (as happens when the user first configures their server).
	seedDefaultServer(t, db)

	// Second call should seed the default catalog now that a server row exists.
	applyBattlepassEngine(appConfig{})

	if err := db.QueryRow(`SELECT COUNT(*) FROM battlepass_tiers WHERE server_id = ?`, defaultServerID).Scan(&count); err != nil {
		t.Fatalf("count battlepass_tiers after server add: %v", err)
	}
	if count == 0 {
		t.Error("after adding server row: expected default catalog to be seeded, got 0 tiers")
	}
}
