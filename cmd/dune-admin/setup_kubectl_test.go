package main

import "testing"

func TestSelectBattlegroup(t *testing.T) {
	t.Parallel()

	if got := selectBattlegroup(nil, func(string, string) string { return "" }); got != "" {
		t.Fatalf("expected empty selection for no battlegroups, got %q", got)
	}

	if got := selectBattlegroup([]string{"alpha"}, func(string, string) string { return "1" }); got != "alpha" {
		t.Fatalf("expected single battlegroup selection, got %q", got)
	}

	groups := []string{"alpha", "beta", "gamma"}
	if got := selectBattlegroup(groups, func(string, string) string { return "2" }); got != "beta" {
		t.Fatalf("expected selection index 2 => beta, got %q", got)
	}

	if got := selectBattlegroup(groups, func(string, string) string { return "99" }); got != "alpha" {
		t.Fatalf("expected invalid selection to fall back to first, got %q", got)
	}
}
