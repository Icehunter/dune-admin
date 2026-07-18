package marketbot

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// openV1Cache creates a temp SQLite cache with the pre-v2 schema (categories +
// metadata, no version key, no bot_written) — what every install had before
// the #295 fix.
func openV1Cache(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	cache, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS categories (
			template_id    TEXT     PRIMARY KEY,
			category_mask  INTEGER  NOT NULL,
			category_depth INTEGER  NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS metadata (
			key   TEXT    PRIMARY KEY,
			value INTEGER NOT NULL
		)`,
	} {
		if _, err := cache.Exec(ddl); err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return cache
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestMigrateCategoryCache_PurgesUnversionedRows: pre-v2 caches are poisoned
// with the bot's own guessed masks (learned back from its own NPC listings)
// and clean player-derived rows are indistinguishable from them — so v2 wipes
// the table (player truth re-learns within one tick) and adds bot_written.
func TestMigrateCategoryCache_PurgesUnversionedRows(t *testing.T) {
	cache := openV1Cache(t)
	if _, err := cache.Exec(
		`INSERT INTO categories (template_id, category_mask, category_depth) VALUES ('poisoned', 999, 3)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := migrateCategoryCache(cache, zerolog.Nop()); err != nil {
		t.Fatalf("migrateCategoryCache: %v", err)
	}

	if n := countRows(t, cache, "categories"); n != 0 {
		t.Errorf("categories rows after migrate = %d, want 0 (purged)", n)
	}
	if n := countRows(t, cache, "bot_written"); n != 0 {
		t.Errorf("bot_written should exist and be empty, got %d rows", n)
	}
	var version int
	if err := cache.QueryRow(`SELECT value FROM metadata WHERE key = 'category_cache_version'`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != categoryCacheVersion {
		t.Errorf("version = %d, want %d", version, categoryCacheVersion)
	}
}

// TestMigrateCategoryCache_IdempotentAtV2: a second migrate must not wipe
// post-migration rows.
func TestMigrateCategoryCache_IdempotentAtV2(t *testing.T) {
	cache := openV1Cache(t)
	if err := migrateCategoryCache(cache, zerolog.Nop()); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if _, err := cache.Exec(
		`INSERT INTO categories (template_id, category_mask, category_depth) VALUES ('learned', 123, 2)`); err != nil {
		t.Fatalf("seed post-v2 row: %v", err)
	}
	if _, err := cache.Exec(
		`INSERT INTO bot_written (template_id, category_mask, category_depth) VALUES ('written', 456, 3)`); err != nil {
		t.Fatalf("seed bot_written: %v", err)
	}

	if err := migrateCategoryCache(cache, zerolog.Nop()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if n := countRows(t, cache, "categories"); n != 1 {
		t.Errorf("categories rows after re-migrate = %d, want 1 (kept)", n)
	}
	if n := countRows(t, cache, "bot_written"); n != 1 {
		t.Errorf("bot_written rows after re-migrate = %d, want 1 (kept)", n)
	}
}

// ── mergeAuthoritative ────────────────────────────────────────────────────────

func TestMergeAuthoritative_LearnsPlayerRows(t *testing.T) {
	updates := mergeAuthoritative(
		map[string]categoryEntry{},
		[]catRow{{tmpl: "Karpov_Schematic", mask: 0x01030300, depth: 3}},
		nil, nil)
	got, ok := updates["karpov_schematic"]
	if !ok || got.mask != 0x01030300 || got.depth != 3 {
		t.Fatalf("updates = %+v, want karpov_schematic learned from player row", updates)
	}
}

// TestMergeAuthoritative_OverwritesStaleCacheWithPlayerTruth kills the old
// "only learn unknown templates" precedence bug: the first mask ever cached
// won forever, even over a later real player listing.
func TestMergeAuthoritative_OverwritesStaleCacheWithPlayerTruth(t *testing.T) {
	current := map[string]categoryEntry{
		"karpov_schematic": {mask: 999, depth: 3}, // stale/poisoned
	}
	updates := mergeAuthoritative(
		current,
		[]catRow{{tmpl: "Karpov_Schematic", mask: 0x01030300, depth: 3}},
		nil, nil)
	got, ok := updates["karpov_schematic"]
	if !ok || got.mask != 0x01030300 {
		t.Fatalf("updates = %+v, want player truth to overwrite stale cache", updates)
	}
}

func TestMergeAuthoritative_UnchangedPlayerRowNotReWritten(t *testing.T) {
	current := map[string]categoryEntry{
		"karpov_schematic": {mask: 0x01030300, depth: 3},
	}
	updates := mergeAuthoritative(
		current,
		[]catRow{{tmpl: "Karpov_Schematic", mask: 0x01030300, depth: 3}},
		nil, nil)
	if len(updates) != 0 {
		t.Fatalf("updates = %+v, want none for identical entry", updates)
	}
}

// TestMergeAuthoritative_IgnoresBotRowsWithoutWrittenRecord: without a
// bot_written record there is no provenance — the row may be a legacy guess,
// so it must never be learned as authoritative (this is the self-poisoning
// path in the old code).
func TestMergeAuthoritative_IgnoresBotRowsWithoutWrittenRecord(t *testing.T) {
	updates := mergeAuthoritative(
		map[string]categoryEntry{},
		nil,
		[]catRow{{tmpl: "GuessTemplate", mask: 777, depth: 3}},
		map[string]categoryEntry{})
	if len(updates) != 0 {
		t.Fatalf("updates = %+v, want none — bot rows without provenance are not authoritative", updates)
	}
}

// TestMergeAuthoritative_LearnsGameCorrectedBotRows: a bot row whose mask
// differs from what the bot recorded writing was rewritten by the game's
// patch-time update_sell_orders_categories — that IS authoritative.
func TestMergeAuthoritative_LearnsGameCorrectedBotRows(t *testing.T) {
	updates := mergeAuthoritative(
		map[string]categoryEntry{},
		nil,
		[]catRow{{tmpl: "Corrected", mask: 0x01050200, depth: 3}},
		map[string]categoryEntry{"corrected": {mask: 0x01030200, depth: 3}})
	got, ok := updates["corrected"]
	if !ok || got.mask != 0x01050200 {
		t.Fatalf("updates = %+v, want game-corrected mask learned", updates)
	}
}

func TestMergeAuthoritative_BotRowMatchingWrittenNotLearned(t *testing.T) {
	updates := mergeAuthoritative(
		map[string]categoryEntry{},
		nil,
		[]catRow{{tmpl: "Same", mask: 0x01030200, depth: 3}},
		map[string]categoryEntry{"same": {mask: 0x01030200, depth: 3}})
	if len(updates) != 0 {
		t.Fatalf("updates = %+v, want none — the bot's own unchanged write is not authoritative", updates)
	}
}

func TestMergeAuthoritative_PlayerTruthWinsOverGameCorrection(t *testing.T) {
	updates := mergeAuthoritative(
		map[string]categoryEntry{},
		[]catRow{{tmpl: "Both", mask: 0x01010100, depth: 3}},
		[]catRow{{tmpl: "Both", mask: 0x02020200, depth: 3}},
		map[string]categoryEntry{"both": {mask: 999, depth: 3}})
	got, ok := updates["both"]
	if !ok || got.mask != 0x01010100 {
		t.Fatalf("updates = %+v, want player row to win over game-corrected bot row", updates)
	}
}

// ── SQL guard assertions ──────────────────────────────────────────────────────

// TestPlayerLearnSQL_ExcludesNPCAndPaymentRows: the learning query must only
// see real player sell listings — is_npc_order = FALSE alone is NOT enough
// because the buy path inserts seller "Take Solari" payment rows with
// is_npc_order = FALSE and mask 0; the JOIN on dune_exchange_sell_orders
// excludes them.
func TestPlayerLearnSQL_ExcludesNPCAndPaymentRows(t *testing.T) {
	for _, want := range []string{
		"is_npc_order = FALSE",
		"JOIN dune.dune_exchange_sell_orders",
		"category_mask != 0",
	} {
		if !strings.Contains(playerCategoryLearnSQL, want) {
			t.Errorf("playerCategoryLearnSQL missing %q", want)
		}
	}
}

func TestBotScanSQL_ScopedToOwnerAndNPC(t *testing.T) {
	for _, want := range []string{
		"owner_id = $1",
		"is_npc_order = TRUE",
		"category_mask != 0",
	} {
		if !strings.Contains(botCategoryScanSQL, want) {
			t.Errorf("botCategoryScanSQL missing %q", want)
		}
	}
}

// TestRemaskSQL_ScopedToBotOwnerAndNPCOrders mirrors
// TestExpireBotOrders_NeverTouchesPlayerOrders: any UPDATE on exchange orders
// MUST carry both guards so player orders are inviolable.
func TestRemaskSQL_ScopedToBotOwnerAndNPCOrders(t *testing.T) {
	for _, want := range []string{
		"owner_id = $4",
		"is_npc_order = TRUE",
		"id = ANY($3)",
	} {
		if !strings.Contains(remaskOrdersSQL, want) {
			t.Errorf("remaskOrdersSQL missing %q", want)
		}
	}
}

// ── planRemask ────────────────────────────────────────────────────────────────

func TestPlanRemask_SkipsMatchingRows(t *testing.T) {
	got := planRemask(
		[]listingInfo{{orderID: 1, mask: 100, depth: 3}},
		categoryEntry{mask: 100, depth: 3})
	if len(got) != 0 {
		t.Fatalf("planRemask = %v, want none for matching mask+depth", got)
	}
}

func TestPlanRemask_FlagsMaskDrift(t *testing.T) {
	got := planRemask(
		[]listingInfo{
			{orderID: 1, mask: 100, depth: 3},
			{orderID: 2, mask: 999, depth: 3},
		},
		categoryEntry{mask: 100, depth: 3})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("planRemask = %v, want [2]", got)
	}
}

func TestPlanRemask_FlagsDepthOnlyDrift(t *testing.T) {
	got := planRemask(
		[]listingInfo{{orderID: 7, mask: 100, depth: 2}},
		categoryEntry{mask: 100, depth: 3})
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("planRemask = %v, want [7]", got)
	}
}

// ── bot_written recording ─────────────────────────────────────────────────────

func TestRecordBotWritten_UpsertsPerTemplate(t *testing.T) {
	cache := openV1Cache(t)
	if err := migrateCategoryCache(cache, zerolog.Nop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	e := &Exchange{cache: cache, log: zerolog.Nop()}

	if err := e.recordBotWritten(map[string]categoryEntry{
		"Tmpl_A": {mask: 100, depth: 3},
	}); err != nil {
		t.Fatalf("recordBotWritten: %v", err)
	}
	// Second write for the same template overwrites, not duplicates.
	if err := e.recordBotWritten(map[string]categoryEntry{
		"Tmpl_A": {mask: 200, depth: 3},
	}); err != nil {
		t.Fatalf("recordBotWritten update: %v", err)
	}

	var mask int32
	if err := cache.QueryRow(
		`SELECT category_mask FROM bot_written WHERE template_id = 'tmpl_a'`).Scan(&mask); err != nil {
		t.Fatalf("read bot_written: %v", err)
	}
	if mask != 200 {
		t.Errorf("bot_written mask = %d, want 200 (upserted)", mask)
	}
	if n := countRows(t, cache, "bot_written"); n != 1 {
		t.Errorf("bot_written rows = %d, want 1", n)
	}
}

// TestNewExchange_MigratesCacheToV2: construction runs the migration so legacy
// poisoned caches are purged before the first tick.
func TestNewExchange_MigratesCacheToV2(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	pre, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := pre.Exec(`CREATE TABLE categories (
		template_id TEXT PRIMARY KEY, category_mask INTEGER NOT NULL, category_depth INTEGER NOT NULL)`); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	if _, err := pre.Exec(
		`INSERT INTO categories VALUES ('poisoned', 999, 3)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = pre.Close()

	ex, err := NewExchange(nil, dbPath, nil, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewExchange: %v", err)
	}
	t.Cleanup(func() { _ = ex.cache.Close() })

	if n := countRows(t, ex.cache, "categories"); n != 0 {
		t.Errorf("categories rows after NewExchange = %d, want 0 (v2 purge)", n)
	}
	if n := countRows(t, ex.cache, "bot_written"); n != 0 {
		t.Errorf("bot_written missing or non-empty after NewExchange: %d", n)
	}
}
