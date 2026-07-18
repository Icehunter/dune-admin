package marketbot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
)

// categories.go — authoritative category learning + convergence (#295).
//
// The in-game market board routes a listing purely by (category_mask,
// category_depth). The bot's computed masks are guesses derived from
// hand-maintained menu-position maps (pricing.go) that break whenever the game
// re-orders a menu. Authoritative masks exist in two places only:
//
//  1. Real player sell listings — the game client writes the true mask.
//  2. Game patch-time corrections — dune.update_sell_orders_categories
//     rewrites mask/depth on ALL orders (including the bot's NPC rows) by
//     template_id when the category config hash changes.
//
// The old cache learned from EVERY order with a non-zero mask — including the
// bot's own guessed NPC rows — and only for templates it didn't already know,
// so the first guess ever persisted won forever (self-poisoning). This file
// replaces that with: learn player truth always (overwriting), learn bot rows
// only when they differ from what the bot recorded writing (bot_written =
// provenance ⇒ a difference means the game corrected them), and never persist
// computed guesses as authoritative.

// categoryCacheVersion gates the one-time purge of pre-#295 poisoned caches.
// Bumping it wipes learned rows; player truth re-learns within one tick.
const categoryCacheVersion = 2

// playerCategoryLearnSQL selects the authoritative masks from real player sell
// listings. is_npc_order = FALSE alone is NOT sufficient: the buy path inserts
// seller "Take Solari" payment rows with is_npc_order = FALSE and mask 0 — the
// JOIN on dune_exchange_sell_orders keeps only actual sell listings.
const playerCategoryLearnSQL = `
	SELECT DISTINCT o.template_id, o.category_mask, o.category_depth
	FROM dune.dune_exchange_orders o
	JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
	WHERE o.is_npc_order = FALSE AND o.category_mask != 0`

// botCategoryScanSQL selects the bot's own live masks, used solely to detect
// game patch-time corrections against bot_written.
const botCategoryScanSQL = `
	SELECT DISTINCT template_id, category_mask, category_depth
	FROM dune.dune_exchange_orders
	WHERE owner_id = $1 AND is_npc_order = TRUE AND category_mask != 0`

// remaskOrdersSQL converges existing bot listings onto the desired mask/depth.
// MUST NOT touch player orders — owner + is_npc_order guards are mandatory on
// every UPDATE/DELETE against exchange tables.
const remaskOrdersSQL = `
	UPDATE dune.dune_exchange_orders
	SET category_mask = $1, category_depth = $2
	WHERE id = ANY($3) AND owner_id = $4 AND is_npc_order = TRUE`

// migrateCategoryCache brings the local SQLite cache to categoryCacheVersion.
// v1→v2: purge the categories table — pre-v2 rows mix player truth with the
// bot's own re-learned guesses and the two are indistinguishable — and create
// bot_written (template → mask/depth the bot last wrote), the provenance that
// makes game corrections detectable. Idempotent at v2.
func migrateCategoryCache(cache *sql.DB, log zerolog.Logger) error {
	if _, err := cache.Exec(`
		CREATE TABLE IF NOT EXISTS bot_written (
			template_id    TEXT     PRIMARY KEY,
			category_mask  INTEGER  NOT NULL,
			category_depth INTEGER  NOT NULL
		)`); err != nil {
		return fmt.Errorf("create bot_written: %w", err)
	}

	var version int
	err := cache.QueryRow(`SELECT value FROM metadata WHERE key = 'category_cache_version'`).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read category cache version: %w", err)
	}
	if version >= categoryCacheVersion {
		return nil
	}

	res, err := cache.Exec(`DELETE FROM categories`)
	if err != nil {
		return fmt.Errorf("purge poisoned category cache: %w", err)
	}
	if _, err := cache.Exec(`
		INSERT INTO metadata (key, value) VALUES ('category_cache_version', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, categoryCacheVersion); err != nil {
		return fmt.Errorf("write category cache version: %w", err)
	}
	if purged, err := res.RowsAffected(); err == nil && purged > 0 {
		log.Info().Int64("purged", purged).Int("version", categoryCacheVersion).
			Msg("category cache migrated: purged pre-v2 rows (player truth re-learns next tick)")
	}
	return nil
}

// catRow is one (template, mask, depth) row from a learning query.
type catRow struct {
	tmpl  string
	mask  int32
	depth int16
}

// mergeAuthoritative computes the cache updates for one refresh pass. Player
// rows always win and always overwrite (a stale or poisoned cache entry must
// never outrank a real player listing). Bot rows are trusted only when a
// bot_written record exists AND differs — that difference is the game's
// patch-time correction; a bot row without provenance may be a legacy guess
// and is ignored. Pure: no IO.
func mergeAuthoritative(current map[string]categoryEntry, playerRows, botRows []catRow, written map[string]categoryEntry) map[string]categoryEntry {
	updates := make(map[string]categoryEntry)
	fromPlayer := make(map[string]bool)

	for _, r := range playerRows {
		if r.mask == 0 {
			continue
		}
		key := strings.ToLower(r.tmpl)
		entry := categoryEntry{mask: r.mask, depth: r.depth}
		fromPlayer[key] = true
		if current[key] != entry {
			updates[key] = entry
		}
	}

	for _, r := range botRows {
		key := strings.ToLower(r.tmpl)
		if r.mask == 0 || fromPlayer[key] {
			continue
		}
		wrote, hasProvenance := written[key]
		entry := categoryEntry{mask: r.mask, depth: r.depth}
		if !hasProvenance || wrote == entry {
			continue
		}
		if current[key] != entry {
			updates[key] = entry
		}
	}
	return updates
}

// planRemask returns the order IDs whose live mask/depth differ from the
// desired entry. Pure: no IO.
func planRemask(listings []listingInfo, desired categoryEntry) []int64 {
	var out []int64
	for _, l := range listings {
		if l.mask != desired.mask || l.depth != desired.depth {
			out = append(out, l.orderID)
		}
	}
	return out
}

// collectRemask records one template's drifted listings into plans, merging
// across the per-grade loop iterations.
func collectRemask(plans map[string]remaskPlan, tmpl string, listings []listingInfo, entry categoryEntry) {
	ids := planRemask(listings, entry)
	if len(ids) == 0 {
		return
	}
	plan := plans[tmpl]
	plan.entry = entry
	plan.orderIDs = append(plan.orderIDs, ids...)
	plans[tmpl] = plan
}

// loadLocalCategories seeds the in-memory map from the local SQLite cache.
func (e *Exchange) loadLocalCategories() {
	rows, err := e.cache.Query(`SELECT template_id, category_mask, category_depth FROM categories`)
	if err != nil {
		e.log.Warn().Err(err).Msg("load category cache failed")
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r catRow
		if err := rows.Scan(&r.tmpl, &r.mask, &r.depth); err != nil {
			continue
		}
		e.categories[strings.ToLower(r.tmpl)] = categoryEntry{mask: r.mask, depth: r.depth}
	}
}

// loadBotWritten returns the template → (mask, depth) records the bot last
// wrote, keyed lowercase.
func (e *Exchange) loadBotWritten() map[string]categoryEntry {
	out := make(map[string]categoryEntry)
	rows, err := e.cache.Query(`SELECT template_id, category_mask, category_depth FROM bot_written`)
	if err != nil {
		e.log.Warn().Err(err).Msg("load bot_written failed")
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r catRow
		if err := rows.Scan(&r.tmpl, &r.mask, &r.depth); err != nil {
			continue
		}
		out[strings.ToLower(r.tmpl)] = categoryEntry{mask: r.mask, depth: r.depth}
	}
	return out
}

// queryCatRows runs one of the learning queries against Postgres.
func (e *Exchange) queryCatRows(ctx context.Context, query string, args ...any) []catRow {
	rows, err := e.db.Query(ctx, query, args...)
	if err != nil {
		e.log.Warn().Err(err).Msg("category learn query failed")
		return nil
	}
	defer rows.Close()
	var out []catRow
	for rows.Next() {
		var r catRow
		if err := rows.Scan(&r.tmpl, &r.mask, &r.depth); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// persistCategoryUpdates applies merged updates to the in-memory map and the
// local SQLite cache.
func (e *Exchange) persistCategoryUpdates(updates map[string]categoryEntry) {
	for key, entry := range updates {
		e.categories[key] = entry
		if _, err := e.cache.Exec(`
			INSERT INTO categories (template_id, category_mask, category_depth)
			VALUES (?, ?, ?)
			ON CONFLICT (template_id) DO UPDATE
			  SET category_mask  = excluded.category_mask,
			      category_depth = excluded.category_depth`,
			key, entry.mask, entry.depth); err != nil {
			e.log.Warn().Str("template_id", key).Err(err).Msg("persist category failed")
		}
	}
	if len(updates) > 0 {
		e.log.Info().Int("updated", len(updates)).Int("total", len(e.categories)).Msg("category cache updated from authoritative sources")
	}
}

// recordBotWritten upserts the (mask, depth) the bot last wrote per template,
// keyed lowercase. This provenance is what lets refresh distinguish a game
// patch-time correction from the bot's own guess.
func (e *Exchange) recordBotWritten(entries map[string]categoryEntry) error {
	for tmpl, entry := range entries {
		if _, err := e.cache.Exec(`
			INSERT INTO bot_written (template_id, category_mask, category_depth)
			VALUES (?, ?, ?)
			ON CONFLICT (template_id) DO UPDATE
			  SET category_mask  = excluded.category_mask,
			      category_depth = excluded.category_depth`,
			strings.ToLower(tmpl), entry.mask, entry.depth); err != nil {
			return fmt.Errorf("record bot_written %s: %w", tmpl, err)
		}
	}
	return nil
}

// remaskPlan groups one template's drifted order IDs with the desired entry.
type remaskPlan struct {
	entry    categoryEntry
	orderIDs []int64
}

// applyRemasks converges the bot's live listings onto their desired masks and
// records the writes on bot_written. Returns how many orders were updated.
// MUST NOT touch player orders — remaskOrdersSQL carries owner + NPC guards.
func (e *Exchange) applyRemasks(ctx context.Context, plans map[string]remaskPlan) int {
	if len(plans) == 0 {
		return 0
	}
	updated := 0
	written := make(map[string]categoryEntry, len(plans))
	for tmpl, plan := range plans {
		if len(plan.orderIDs) == 0 {
			continue
		}
		tag, err := e.db.Exec(ctx, remaskOrdersSQL, plan.entry.mask, plan.entry.depth, plan.orderIDs, e.ownerID)
		if err != nil {
			e.log.Warn().Str("template_id", tmpl).Err(err).Msg("remask listings failed")
			continue
		}
		updated += int(tag.RowsAffected())
		written[tmpl] = plan.entry
	}
	if err := e.recordBotWritten(written); err != nil {
		e.log.Warn().Err(err).Msg("record bot_written after remask failed")
	}
	if updated > 0 {
		e.log.Info().Int("orders", updated).Int("templates", len(written)).Msg("re-categorized bot listings to authoritative masks")
	}
	return updated
}
