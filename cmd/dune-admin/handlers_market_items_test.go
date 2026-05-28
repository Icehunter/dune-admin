package main

import (
	"net/http/httptest"
	"testing"
)

func TestBuildMarketItemsFilter(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest("GET", "/api/v1/market/items?search=Spice&category=resources&tier=3&rarity=rare&owner=bot", nil)
	filter := buildMarketItemsFilter(r)

	if filter.search != "spice" || filter.category != "resources" || filter.rarity != "rare" || filter.owner != "bot" {
		t.Fatalf("unexpected filter fields: %+v", filter)
	}
	if filter.tier == nil || *filter.tier != 3 {
		t.Fatalf("expected tier=3, got %+v", filter.tier)
	}

	rInvalid := httptest.NewRequest("GET", "/api/v1/market/items?tier=not-a-number", nil)
	filter = buildMarketItemsFilter(rInvalid)
	if filter.tier != nil {
		t.Fatalf("expected invalid tier to be ignored, got %+v", filter.tier)
	}
}

func TestMarketItemMatchesFilter(t *testing.T) {
	t.Parallel()

	item := marketItem{
		TemplateID:  "Dune.Spice.Raw",
		DisplayName: "Raw Spice",
		Category:    "resources/spice",
		Tier:        3,
		Rarity:      "Rare",
		TotalStock:  12,
		BotStock:    2,
	}

	if !marketItemMatchesFilter(item, marketItemsFilter{search: "spice"}) {
		t.Fatal("expected search filter to match")
	}
	if marketItemMatchesFilter(item, marketItemsFilter{search: "water"}) {
		t.Fatal("expected non-matching search to fail")
	}
	if marketItemMatchesFilter(item, marketItemsFilter{category: "weapons"}) {
		t.Fatal("expected non-matching category to fail")
	}
	if marketItemMatchesFilter(item, marketItemsFilter{owner: "player", tier: intRef(4)}) {
		t.Fatal("expected tier mismatch to fail")
	}
	if !marketItemMatchesFilter(item, marketItemsFilter{owner: "player"}) {
		t.Fatal("expected player-owner filter to match when player stock exists")
	}
}

func TestFilterMarketItems(t *testing.T) {
	t.Parallel()

	items := []marketItem{
		{TemplateID: "A", DisplayName: "Alpha", Category: "cat/a", TotalStock: 5, BotStock: 0},
		{TemplateID: "B", DisplayName: "Bravo", Category: "cat/b", TotalStock: 5, BotStock: 5},
	}
	filtered := filterMarketItems(items, marketItemsFilter{owner: "player"})
	if len(filtered) != 1 || filtered[0].TemplateID != "A" {
		t.Fatalf("unexpected filtered items: %#v", filtered)
	}
}

func TestMarketItemsPagination(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest("GET", "/api/v1/market/items?page=2&limit=3", nil)
	start, end, page, limit := marketItemsPagination(r, 8)
	if start != 6 || end != 8 || page != 2 || limit != 3 {
		t.Fatalf("unexpected pagination: start=%d end=%d page=%d limit=%d", start, end, page, limit)
	}

	rDefault := httptest.NewRequest("GET", "/api/v1/market/items?page=-1&limit=9999", nil)
	start, end, page, limit = marketItemsPagination(rDefault, 4)
	if start != 0 || end != 4 || page != 0 || limit != 100 {
		t.Fatalf("unexpected default pagination: start=%d end=%d page=%d limit=%d", start, end, page, limit)
	}
}

func intRef(v int) *int {
	return &v
}
