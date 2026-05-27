package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// handleMarketItems returns all active exchange listings aggregated by template ID.
// Query params: search, category, tier, rarity, owner (bot|player|all), page, limit.
func handleMarketItems(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketItems().(msgMarketItems)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}

	items := msg.rows
	if items == nil {
		items = []marketItem{}
	}

	q := r.URL.Query()
	search := strings.ToLower(q.Get("search"))
	category := q.Get("category")
	tierStr := q.Get("tier")
	rarity := strings.ToLower(q.Get("rarity"))
	owner := q.Get("owner") // "bot", "player", or "" for all

	// Apply filters in Go — simpler than parameterised SQL for this aggregation query.
	filtered := items[:0]
	for _, it := range items {
		if search != "" {
			if !strings.Contains(strings.ToLower(it.DisplayName), search) &&
				!strings.Contains(strings.ToLower(it.TemplateID), search) {
				continue
			}
		}
		if category != "" && !strings.HasPrefix(it.Category, category) {
			continue
		}
		if tierStr != "" {
			if t, err := strconv.Atoi(tierStr); err == nil && it.Tier != t {
				continue
			}
		}
		if rarity != "" && !strings.EqualFold(it.Rarity, rarity) {
			continue
		}
		if owner == "bot" && it.BotStock == 0 {
			continue
		}
		if owner == "player" && (it.TotalStock-it.BotStock) == 0 {
			continue
		}
		filtered = append(filtered, it)
	}

	// Pagination.
	limit := 100
	page := 0
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 500 {
		limit = l
	}
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 0 {
		page = p
	}
	start := page * limit
	end := start + limit
	if start >= len(filtered) {
		start = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	jsonOK(w, map[string]any{
		"items": filtered[start:end],
		"total": len(filtered),
		"page":  page,
		"limit": limit,
	})
}

// handleMarketListings returns all active individual listings, optionally for one template.
// Query param: template_id, owner (bot|player|all), sort (price|quality).
func handleMarketListings(w http.ResponseWriter, r *http.Request) {
	templateID := r.URL.Query().Get("template_id")
	msg, ok := cmdFetchMarketListings(templateID).(msgMarketListings)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}

	listings := msg.rows
	if listings == nil {
		listings = []marketListing{}
	}

	if owner := r.URL.Query().Get("owner"); owner == "bot" || owner == "player" {
		filtered := listings[:0]
		for _, l := range listings {
			if l.OwnerType == owner {
				filtered = append(filtered, l)
			}
		}
		listings = filtered
	}

	jsonOK(w, listings)
}

// handleMarketSales returns recent fulfilled sales (players buying from the bot).
func handleMarketSales(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketSales().(msgMarketSales)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	sales := msg.rows
	if sales == nil {
		sales = []marketSale{}
	}
	jsonOK(w, sales)
}

// handleMarketStats returns aggregate market statistics (admin-only by convention).
func handleMarketStats(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketStats().(msgMarketStats)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, msg.stats)
}

// handleMarketCategories returns the category tree derived from item-data.json.
// Schematic items are reclassified under "schematics/" to surface as their own group.
func handleMarketCategories(w http.ResponseWriter, r *http.Request) {
	seen := map[string]bool{}
	var categories []string
	for templateID, rule := range itemData.Items {
		if rule.Category == "" {
			continue
		}
		cat := schematicCategory(templateID, rule.Category)
		if !seen[cat] {
			seen[cat] = true
			categories = append(categories, cat)
		}
	}
	jsonOK(w, categories)
}

// handleMarketCatalog returns a flat list of all known items (template_id + display_name)
// for use in autocomplete UIs such as the disabled-items manager.
func handleMarketCatalog(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		TemplateID  string `json:"template_id"`
		DisplayName string `json:"display_name"`
	}
	seen := map[string]bool{}
	var items []entry
	for tmpl, rule := range itemData.Items {
		name := rule.Name
		if name == "" {
			name = tmpl
		}
		seen[strings.ToLower(tmpl)] = true
		items = append(items, entry{TemplateID: tmpl, DisplayName: name})
	}
	for tmpl, name := range itemData.Names {
		if !seen[strings.ToLower(tmpl)] {
			items = append(items, entry{TemplateID: tmpl, DisplayName: name})
		}
	}
	jsonOK(w, items)
}
