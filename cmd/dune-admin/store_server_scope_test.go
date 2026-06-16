package main

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// scope ids used across the isolation tests. Both have a parent servers row
// inserted by openSharedScopeDB so the int FK on every scoped table is satisfied.
const (
	scopeA = 1
	scopeB = 2
)

// openSharedScopeDB opens a shared in-memory unified store (FK enforcement ON)
// and inserts two parent server rows (ids scopeA/scopeB) so scoped child rows
// satisfy their server_id FK. Used by server-scope isolation tests.
func openSharedScopeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, id := range []int{scopeA, scopeB} {
		if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (?, ?, ?)`, id, "srv", id); err != nil {
			t.Fatalf("insert server %d: %v", id, err)
		}
	}
	return db
}

// ── welcome_grants ─────────────────────────────────────────────────────────────

// TestWelcomeStore_GrantsServerScope verifies that two welcomeStore instances
// sharing the same DB cannot see each other's welcome_grants rows.
func TestWelcomeStore_GrantsServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newWelcomeStore(db, scopeA)
	sB := newWelcomeStore(db, scopeB)

	if err := sA.insertGranted("FLS1", "v1", 1, "Paul"); err != nil {
		t.Fatalf("sA.insertGranted: %v", err)
	}

	if ex, err := sB.grantExists("FLS1", "v1", 1); err != nil {
		t.Fatalf("sB.grantExists: %v", err)
	} else if ex {
		t.Error("server B should not see server A's grant")
	}

	if ex, err := sA.grantExists("FLS1", "v1", 1); err != nil {
		t.Fatalf("sA.grantExists: %v", err)
	} else if !ex {
		t.Error("server A should see its own grant")
	}

	recs, err := sB.listGrants(10)
	if err != nil {
		t.Fatalf("sB.listGrants: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("sB.listGrants: got %d rows, want 0", len(recs))
	}
}

// ── welcome_config ─────────────────────────────────────────────────────────────

// TestWelcomeStore_ConfigServerScope verifies that saveConfig on server A is
// invisible to server B's loadConfig.
func TestWelcomeStore_ConfigServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newWelcomeStore(db, scopeA)
	sB := newWelcomeStore(db, scopeB)

	cfg := welcomeConfigRow{Enabled: true, ScanSecs: 99}
	if err := sA.saveConfig(cfg); err != nil {
		t.Fatalf("sA.saveConfig: %v", err)
	}

	_, ok, err := sB.loadConfig()
	if err != nil {
		t.Fatalf("sB.loadConfig: %v", err)
	}
	if ok {
		t.Error("server B should not see server A's config (ok must be false)")
	}

	row, ok, err := sA.loadConfig()
	if err != nil {
		t.Fatalf("sA.loadConfig: %v", err)
	}
	if !ok {
		t.Error("server A should see its own config")
	}
	if row.ScanSecs != 99 {
		t.Errorf("sA.loadConfig ScanSecs = %d, want 99", row.ScanSecs)
	}
}

// ── give_packs_config ──────────────────────────────────────────────────────────

// TestGivePacksStore_ServerScope verifies that saveConfig on server A is
// invisible to server B's loadConfig.
func TestGivePacksStore_ServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newGivePacksStore(db, scopeA)
	sB := newGivePacksStore(db, scopeB)

	const packsJSON = `[{"id":"p1"}]`
	if err := sA.saveConfig(packsJSON, true); err != nil {
		t.Fatalf("sA.saveConfig: %v", err)
	}

	_, _, ok, err := sB.loadConfig()
	if err != nil {
		t.Fatalf("sB.loadConfig: %v", err)
	}
	if ok {
		t.Error("server B should not see server A's config (ok must be false)")
	}

	_, got, ok, err := sA.loadConfig()
	if err != nil {
		t.Fatalf("sA.loadConfig: %v", err)
	}
	if !ok {
		t.Error("server A should see its own config")
	}
	var gotPacks []givePack
	if err := json.Unmarshal([]byte(got), &gotPacks); err != nil {
		t.Fatalf("unmarshal sA packs %q: %v", got, err)
	}
	if len(gotPacks) != 1 || gotPacks[0].ID != "p1" {
		t.Errorf("sA.loadConfig packs = %q, want one pack with id p1", got)
	}
}

// ── event_award_claims ─────────────────────────────────────────────────────────

// TestEventStore_ClaimsServerScope verifies that a claim recorded on server A
// is not visible from server B.
func TestEventStore_ClaimsServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newEventStore(db, scopeA)
	sB := newEventStore(db, scopeB)

	if err := sA.recordGranted(1, 1, 42); err != nil {
		t.Fatalf("sA.recordGranted: %v", err)
	}

	ex, err := sB.claimExists(1, 1, 42)
	if err != nil {
		t.Fatalf("sB.claimExists: %v", err)
	}
	if ex {
		t.Error("server B should not see server A's claim")
	}

	ex, err = sA.claimExists(1, 1, 42)
	if err != nil {
		t.Fatalf("sA.claimExists: %v", err)
	}
	if !ex {
		t.Error("server A should see its own claim")
	}
}

// ── battlepass_claims ──────────────────────────────────────────────────────────

// TestBattlepassStore_ClaimsServerScope verifies that a claim recorded on
// server A is invisible to server B.
func TestBattlepassStore_ClaimsServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newBattlepassStore(db, scopeA)
	sB := newBattlepassStore(db, scopeB)

	if err := sA.recordClaim("level:5", 42, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("sA.recordClaim: %v", err)
	}

	keys, err := sB.claimedKeys(42)
	if err != nil {
		t.Fatalf("sB.claimedKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("server B should not see server A's claims, got %v", keys)
	}

	keys, err = sA.claimedKeys(42)
	if err != nil {
		t.Fatalf("sA.claimedKeys: %v", err)
	}
	if _, ok := keys["level:5"]; !ok {
		t.Error("server A should see its own claim")
	}
}

// ── battlepass_grant_ledger ────────────────────────────────────────────────────

// TestBattlepassStore_GrantLedgerServerScope verifies that a pending grant
// recorded on server A is not visible from server B's retry list.
func TestBattlepassStore_GrantLedgerServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	sA := newBattlepassStore(db, scopeA)
	sB := newBattlepassStore(db, scopeB)

	if err := sA.recordPendingGrant("level:5", 42); err != nil {
		t.Fatalf("sA.recordPendingGrant: %v", err)
	}

	rows, err := sB.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("sB.listRetryableGrantLedger: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("server B should not see server A's grant ledger, got %d rows", len(rows))
	}

	rows, err = sA.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("sA.listRetryableGrantLedger: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("server A should see 1 pending grant, got %d", len(rows))
	}
}

// ── discord status ─────────────────────────────────────────────────────────────

// TestSqliteStatusStore_ServerScope verifies that status message saved by
// server A is not visible to server B.
func TestSqliteStatusStore_ServerScope(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	ssA := newSqliteStatusStore(db, scopeA)
	ssB := newSqliteStatusStore(db, scopeB)

	if err := ssA.saveStatusMessage("ch1", "msg1"); err != nil {
		t.Fatalf("ssA.saveStatusMessage: %v", err)
	}

	ch, msg, err := ssB.loadStatusMessage()
	if err != nil {
		t.Fatalf("ssB.loadStatusMessage: %v", err)
	}
	if ch != "" || msg != "" {
		t.Errorf("server B should not see server A's status message, got ch=%q msg=%q", ch, msg)
	}

	ch, msg, err = ssA.loadStatusMessage()
	if err != nil {
		t.Fatalf("ssA.loadStatusMessage: %v", err)
	}
	if ch != "ch1" || msg != "msg1" {
		t.Errorf("server A status message = (%q, %q), want (ch1, msg1)", ch, msg)
	}
}

// ── cascade delete ─────────────────────────────────────────────────────────────

// TestServerDeleteCascade proves that deleting a server removes every scoped
// child row for that server while leaving another server's rows untouched —
// the FK ON DELETE CASCADE that replaces the old manual purge.
func TestServerDeleteCascade(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	wA, wB := newWelcomeStore(db, scopeA), newWelcomeStore(db, scopeB)
	gA, gB := newGivePacksStore(db, scopeA), newGivePacksStore(db, scopeB)
	eA, eB := newEventStore(db, scopeA), newEventStore(db, scopeB)
	bA, bB := newBattlepassStore(db, scopeA), newBattlepassStore(db, scopeB)
	stA := newSqliteStatusStore(db, scopeA)

	// Seed both servers across every scoped table.
	for _, s := range []*welcomeStore{wA, wB} {
		if err := s.insertGranted("FLS", "v1", 7, "Leto"); err != nil {
			t.Fatalf("insertGranted: %v", err)
		}
		if err := s.saveConfig(welcomeConfigRow{Enabled: true, ScanSecs: 30,
			PackagesJSON:   `[{"version":"v1","items":[{"template":"t","qty":1}]}]`,
			ActiveVersions: []string{"v1"}}); err != nil {
			t.Fatalf("welcome saveConfig: %v", err)
		}
	}
	for _, s := range []*givePacksStore{gA, gB} {
		if err := s.saveConfig(`[{"id":"p1","items":[{"template":"t","qty":1}]}]`, true); err != nil {
			t.Fatalf("give saveConfig: %v", err)
		}
	}
	for _, s := range []*eventStore{eA, eB} {
		if err := s.recordGranted(1, 1, 42); err != nil {
			t.Fatalf("recordGranted: %v", err)
		}
	}
	for _, s := range []*battlepassStore{bA, bB} {
		if err := s.recordClaim("level:5", 42, 100, battlepassClaimEarned); err != nil {
			t.Fatalf("recordClaim: %v", err)
		}
		if err := s.recordPendingGrant("level:5", 42); err != nil {
			t.Fatalf("recordPendingGrant: %v", err)
		}
	}
	if err := stA.saveStatusMessage("ch1", "msg1"); err != nil {
		t.Fatalf("saveStatusMessage: %v", err)
	}

	// Delete server A.
	if err := newServersStore(db).deleteServer(scopeA); err != nil {
		t.Fatalf("deleteServer: %v", err)
	}

	// Every scoped table must have zero rows for scopeA.
	assertCount(t, db, "welcome_grants", scopeA, 0)
	assertCount(t, db, "welcome_config", scopeA, 0)
	assertCount(t, db, "welcome_packages", scopeA, 0)
	assertCount(t, db, "give_packs_config", scopeA, 0)
	assertCount(t, db, "give_packs", scopeA, 0)
	assertCount(t, db, "event_award_claims", scopeA, 0)
	assertCount(t, db, "battlepass_claims", scopeA, 0)
	assertCount(t, db, "battlepass_grant_ledger", scopeA, 0)
	assertCount(t, db, "server_discord_status", scopeA, 0)
	// Child item rows cascade through their parent.
	assertTotalCount(t, db, "welcome_package_items", 1)
	assertTotalCount(t, db, "give_pack_items", 1)

	// Server B's rows remain.
	assertCount(t, db, "welcome_grants", scopeB, 1)
	assertCount(t, db, "give_packs_config", scopeB, 1)
	assertCount(t, db, "event_award_claims", scopeB, 1)
	assertCount(t, db, "battlepass_claims", scopeB, 1)
}

func assertCount(t *testing.T, db *sql.DB, table string, serverID, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE server_id = ?`, serverID).Scan(&n); err != nil { // #nosec G202 -- test table literal
		t.Fatalf("count %s: %v", table, err)
	}
	if n != want {
		t.Errorf("%s rows for server %d = %d, want %d", table, serverID, n, want)
	}
}

func assertTotalCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil { // #nosec G202 -- test table literal
		t.Fatalf("count %s: %v", table, err)
	}
	if n != want {
		t.Errorf("%s total rows = %d, want %d", table, n, want)
	}
}
