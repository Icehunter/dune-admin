package main

// #297 — the periodic stat-snapshot collector recorded only the wallet
// (currency_id = 0) Solari balance and returned nil on ErrNoRows, so a
// player whose only Solari sat in carried inventory or an owned base
// stash never got a history point recorded — the chart stayed empty.
// The "Solaris display" value (cmdFetchPlayerPgStats, fixed for #266)
// already sums wallet + item-form Solari; fetchSolarisBalance must use
// the same total via a shared helper so the two can never drift again.

import (
	"context"
	"testing"
)

func TestFetchItemFormSolari_NilPool(t *testing.T) {
	t.Parallel()
	_, err := fetchItemFormSolari(context.Background(), nil, 42)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestFetchSolarisBalance_NilPool(t *testing.T) {
	t.Parallel()
	bal, err := fetchSolarisBalance(context.Background(), nil, 42)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
	if bal != nil {
		t.Fatalf("expected nil balance on error, got %v", *bal)
	}
}
