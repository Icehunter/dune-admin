package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schematicCategory returns the effective category for a template ID.
// Schematics (detected by is_schematic flag or naming conventions) are
// reclassified under "schematics/<type>" where <type> is only the first
// path segment after "items/" (e.g. "weapons", "utility").
// Using a single sub-level prevents mirroring the full items tree under schematics/.
//
// Naming patterns covered:
//   - T6_Lasgun_Schematic  (suffix _Schematic — regular schematics)
//   - Schematic_UniqueXxx  (prefix Schematic_ — unique/named schematics)
//   - ChoamHeavyLasgunSchematic (suffix Schematic, no underscore)
func schematicCategory(templateID, baseCategory string, isSchematic bool) string {
	lc := strings.ToLower(templateID)
	if !isSchematic &&
		!strings.HasSuffix(lc, "_schematic") &&
		!strings.HasPrefix(lc, "schematic_") &&
		!strings.HasSuffix(lc, "schematic") {
		return baseCategory
	}
	rest := strings.TrimPrefix(baseCategory, "items/")
	if rest == "" || rest == baseCategory {
		return "schematics"
	}
	// Take only the first segment (e.g. "utility" from "utility/gatheringtools/compactor").
	if idx := strings.Index(rest, "/"); idx != -1 {
		rest = rest[:idx]
	}
	return "schematics/" + rest
}

// itemRuleLookup returns the itemRule for a template ID, trying exact key then lowercase fallback.
func itemRuleLookup(templateID string) (itemRule, bool) {
	if r, ok := itemData.Items[templateID]; ok {
		return r, true
	}
	if r, ok := itemData.Items[strings.ToLower(templateID)]; ok {
		return r, true
	}
	return itemRule{}, false
}

// itemNameLookup returns the display name for a template ID.
func itemNameLookup(templateID string) string {
	if n := itemData.Names[templateID]; n != "" {
		return n
	}
	if n := itemData.Names[strings.ToLower(templateID)]; n != "" {
		return n
	}
	return templateID
}

// cmdFetchMarketItems returns all active exchange listings aggregated by template ID,
// enriched with catalog metadata from item-data.json.
func cmdFetchMarketItems(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgMarketItems{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT
			o.template_id,
			o.quality_level,
			MIN(o.item_price)                                              AS lowest_price,
			COALESCE(SUM(COALESCE(i.stack_size, s.initial_stack_size)), 0) AS total_stock,
			COALESCE(SUM(CASE WHEN o.is_npc_order
			    THEN COALESCE(i.stack_size, s.initial_stack_size) ELSE 0 END), 0) AS bot_stock,
			COUNT(*)                                                        AS listing_count
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.items i ON i.id = o.item_id
		GROUP BY o.template_id, o.quality_level
		ORDER BY o.template_id, o.quality_level`)
	if err != nil {
		return msgMarketItems{err: err}
	}
	defer rows.Close()

	var items []marketItem
	for rows.Next() {
		var tmpl string
		var quality, lowestPrice, totalStock, botStock, listingCount int64
		if err := rows.Scan(&tmpl, &quality, &lowestPrice, &totalStock, &botStock, &listingCount); err != nil {
			continue
		}
		rule, _ := itemRuleLookup(tmpl)
		cat := schematicCategory(tmpl, rule.Category, rule.IsSchematic)
		name := itemNameLookup(tmpl)
		if cat == "schematics" || strings.HasPrefix(cat, "schematics/") {
			name += " (Schematic)"
		}
		items = append(items, marketItem{
			TemplateID:   tmpl,
			Quality:      quality,
			DisplayName:  name,
			Category:     cat,
			Tier:         rule.Tier,
			Rarity:       rule.Rarity,
			LowestPrice:  lowestPrice,
			TotalStock:   totalStock,
			BotStock:     botStock,
			ListingCount: listingCount,
			Icon:         rule.Icon,
		})
	}
	if rows.Err() != nil {
		return msgMarketItems{err: rows.Err()}
	}
	return msgMarketItems{rows: items}
}

// cmdFetchMarketListings returns all active exchange listings, optionally filtered by template ID.
// Pass templateID="" to fetch all listings.
func cmdFetchMarketListings(pool *pgxpool.Pool, templateID string) Msg {
	if pool == nil {
		return msgMarketListings{err: fmt.Errorf("not connected")}
	}

	var args []any
	where := ""
	if templateID != "" {
		where = "WHERE o.template_id = $1"
		args = append(args, templateID)
	}

	rows, err := pool.Query(context.Background(), `
		SELECT
			o.id,
			o.template_id,
			o.is_npc_order,
			COALESCE(ps.character_name, a.class, 'Unknown') AS owner_name,
			o.item_price,
			COALESCE(i.stack_size, s.initial_stack_size)     AS stock,
			COALESCE(o.quality_level, 0)                      AS quality
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.items i ON i.id = o.item_id
		LEFT JOIN dune.actors a ON a.id = o.owner_id
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		`+where+`
		ORDER BY o.template_id, o.item_price`, args...)
	if err != nil {
		return msgMarketListings{err: err}
	}
	defer rows.Close()

	var listings []marketListing
	for rows.Next() {
		var l marketListing
		var isNPC bool
		if err := rows.Scan(&l.OrderID, &l.TemplateID, &isNPC, &l.OwnerName, &l.Price, &l.Stock, &l.Quality); err != nil {
			continue
		}
		if isNPC {
			l.OwnerType = "bot"
			l.OwnerName = "Revy"
		} else {
			l.OwnerType = "player"
		}
		listings = append(listings, l)
	}
	if rows.Err() != nil {
		return msgMarketListings{err: rows.Err()}
	}
	return msgMarketListings{rows: listings}
}

// cmdFetchMarketSales returns recent sales from bot listings (players buying from Revy).
func cmdFetchMarketSales(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgMarketSales{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT
			f.order_id,
			o.template_id,
			o.is_npc_order,
			COALESCE(ps.character_name, a.class, 'Unknown') AS seller_name,
			o.item_price,
			f.stack_size
		FROM dune.dune_exchange_fulfilled_orders f
		JOIN dune.dune_exchange_orders o ON o.id = f.order_id
		LEFT JOIN dune.actors a ON a.id = o.owner_id
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		ORDER BY f.order_id DESC
		LIMIT 200`)
	if err != nil {
		return msgMarketSales{err: err}
	}
	defer rows.Close()

	var sales []marketSale
	for rows.Next() {
		var s marketSale
		var isNPC bool
		if err := rows.Scan(&s.OrderID, &s.TemplateID, &isNPC, &s.SellerName, &s.Price, &s.Quantity); err != nil {
			continue
		}
		if isNPC {
			s.SellerType = "bot"
			s.SellerName = "Revy"
		} else {
			s.SellerType = "player"
		}
		sales = append(sales, s)
	}
	if rows.Err() != nil {
		return msgMarketSales{err: rows.Err()}
	}
	return msgMarketSales{rows: sales}
}

// cmdFetchMarketStats returns aggregate market statistics.
func cmdFetchMarketStats(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgMarketStats{err: fmt.Errorf("not connected")}
	}
	var stats marketStats
	err := pool.QueryRow(context.Background(), `
		SELECT
			COUNT(*)                                                          AS total_listings,
			COUNT(*) FILTER (WHERE o.is_npc_order)                           AS bot_listings,
			COUNT(*) FILTER (WHERE NOT o.is_npc_order)                       AS player_listings,
			COALESCE(SUM(COALESCE(i.stack_size, s.initial_stack_size)), 0)   AS total_stock,
			COALESCE(SUM(CASE WHEN o.is_npc_order
			    THEN COALESCE(i.stack_size, s.initial_stack_size) ELSE 0 END), 0) AS bot_stock,
			COALESCE(SUM(CASE WHEN NOT o.is_npc_order
			    THEN COALESCE(i.stack_size, s.initial_stack_size) ELSE 0 END), 0) AS player_stock,
			COUNT(DISTINCT o.template_id)                                     AS unique_items
		FROM dune.dune_exchange_orders o
		JOIN dune.dune_exchange_sell_orders s ON s.order_id = o.id
		LEFT JOIN dune.items i ON i.id = o.item_id`).
		Scan(&stats.TotalListings, &stats.BotListings, &stats.PlayerListings,
			&stats.TotalStock, &stats.BotStock, &stats.PlayerStock, &stats.UniqueItems)
	if err != nil {
		return msgMarketStats{err: err}
	}
	return msgMarketStats{stats: stats}
}
