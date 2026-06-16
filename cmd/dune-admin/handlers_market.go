package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type marketItemsFilter struct {
	search   string
	category string
	tier     *int
	rarity   string
	owner    string
}

func buildMarketItemsFilter(r *http.Request) marketItemsFilter {
	q := r.URL.Query()
	filter := marketItemsFilter{
		search:   strings.ToLower(q.Get("search")),
		category: q.Get("category"),
		rarity:   strings.ToLower(q.Get("rarity")),
		owner:    q.Get("owner"),
	}
	if tierStr := q.Get("tier"); tierStr != "" {
		if tier, err := strconv.Atoi(tierStr); err == nil {
			filter.tier = &tier
		}
	}
	return filter
}

func marketItemMatchesFilter(it marketItem, filter marketItemsFilter) bool {
	if filter.search != "" {
		if !strings.Contains(strings.ToLower(it.DisplayName), filter.search) &&
			!strings.Contains(strings.ToLower(it.TemplateID), filter.search) {
			return false
		}
	}
	if filter.category != "" && !strings.HasPrefix(it.Category, filter.category) {
		return false
	}
	if filter.tier != nil && it.Tier != *filter.tier {
		return false
	}
	if filter.rarity != "" && !strings.EqualFold(it.Rarity, filter.rarity) {
		return false
	}
	if filter.owner == "bot" && it.BotStock == 0 {
		return false
	}
	if filter.owner == "player" && (it.TotalStock-it.BotStock) == 0 {
		return false
	}
	return true
}

func filterMarketItems(items []marketItem, filter marketItemsFilter) []marketItem {
	filtered := make([]marketItem, 0, len(items))
	for _, it := range items {
		if marketItemMatchesFilter(it, filter) {
			filtered = append(filtered, it)
		}
	}
	return filtered
}

func marketItemsPagination(r *http.Request, total int) (start, end, page, limit int) {
	q := r.URL.Query()
	limit = 100
	page = 0
	if parsedLimit, err := strconv.Atoi(q.Get("limit")); err == nil && parsedLimit > 0 && parsedLimit <= 500 {
		limit = parsedLimit
	}
	if parsedPage, err := strconv.Atoi(q.Get("page")); err == nil && parsedPage > 0 {
		page = parsedPage
	}
	start = page * limit
	end = start + limit
	if start >= total {
		start = total
	}
	if end > total {
		end = total
	}
	return start, end, page, limit
}

// handleMarketItems returns all active exchange listings aggregated by template ID.
// Query params: search, category, tier, rarity, owner (bot|player|all), page, limit.
// @Summary List market items aggregated by template ID
// @Tags market
// @Produce json
// @Param search query string false "Filter by display name or template ID"
// @Param category query string false "Filter by category prefix"
// @Param tier query int false "Filter by item tier"
// @Param rarity query string false "Filter by rarity"
// @Param owner query string false "Filter by owner type (bot|player|all)"
// @Param page query int false "Page number (0-based)"
// @Param limit query int false "Page size (default 100, max 500)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/market/items [get]
func handleMarketItems(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketItems(dbFromCtx(r)).(msgMarketItems)
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

	filter := buildMarketItemsFilter(r)
	filtered := filterMarketItems(items, filter)
	start, end, page, limit := marketItemsPagination(r, len(filtered))

	jsonOK(w, map[string]any{
		"items": filtered[start:end],
		"total": len(filtered),
		"page":  page,
		"limit": limit,
	})
}

// handleMarketListings returns all active individual listings, optionally for one template.
// Query param: template_id, owner (bot|player|all), sort (price|quality).
// @Summary List individual active market listings
// @Tags market
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/market/listings [get]
func handleMarketListings(w http.ResponseWriter, r *http.Request) {
	templateID := r.URL.Query().Get("template_id")
	msg, ok := cmdFetchMarketListings(dbFromCtx(r), templateID).(msgMarketListings)
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
// @Summary List recent fulfilled market sales
// @Tags market
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/market/sales [get]
func handleMarketSales(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketSales(dbFromCtx(r)).(msgMarketSales)
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
// @Summary Return aggregate market statistics
// @Tags market
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/market/stats [get]
func handleMarketStats(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchMarketStats(dbFromCtx(r)).(msgMarketStats)
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
// @Summary List distinct item categories from the item catalog
// @Tags market
// @Produce json
// @Success 200 {array} string
// @Router /api/v1/market/categories [get]
func handleMarketCategories(w http.ResponseWriter, r *http.Request) {
	seen := map[string]bool{}
	var categories []string
	for templateID, rule := range itemData.Items {
		if rule.Category == "" {
			continue
		}
		cat := schematicCategory(templateID, rule.Category, rule.IsSchematic)
		if !seen[cat] {
			seen[cat] = true
			categories = append(categories, cat)
		}
	}
	jsonOK(w, categories)
}

// handleMarketCatalog returns a flat list of all known items (template_id + display_name)
// for use in autocomplete UIs such as the disabled-items manager.
// @Summary List all known item templates with display names
// @Tags market
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Router /api/v1/market/catalog [get]
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
	for tmpl, origTmpl := range itemData.OriginalNames {
		if !seen[tmpl] { // tmpl is already lowercased here
			items = append(items, entry{TemplateID: origTmpl, DisplayName: itemData.Names[tmpl]})
		}
	}
	jsonOK(w, items)
}
