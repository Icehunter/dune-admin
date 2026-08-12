package main

import (
	"strings"
	"testing"
)

// The players list is rooted on dune.actors, and deleting a character leaves
// its three actor rows (Controller/State/Pawn) behind — measured on a live
// server against characters whose encrypted_player_state row reads
// character_state = 'Deleted'. Resolving the player_state row per ACCOUNT
// therefore emitted one row per surviving pawn and let the dead pawn borrow
// the LIVING character's name, controller id, faction and online status: 61
// rows for 51 living characters, 3 of them exact-looking name duplicates.
//
// The fix binds each pawn to ITS OWN character row (player_pawn_id) with an
// INNER lateral: dune.player_state only exposes live characters, so a pawn
// with no living character row drops out — which is precisely what "list the
// real characters" means.

func TestPlayerStateByPawnJoin_InnerLateralKeyedOnPawn(t *testing.T) {
	t.Parallel()
	got := playerStateByPawnJoinOn("sa", "sps")
	for _, want := range []string{
		"JOIN LATERAL",
		"sps2.player_pawn_id = sa.id",
		") sps ON true",
		canonicalOrderFragment,
		"LIMIT 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("playerStateByPawnJoinOn missing %q:\n%s", want, got)
		}
	}
	// INNER, not LEFT — a LEFT join would keep the ghost rows (nameless).
	if strings.Contains(got, "LEFT JOIN LATERAL") {
		t.Fatalf("playerStateByPawnJoinOn must be an INNER lateral join:\n%s", got)
	}
}

// TestFetchPlayersSQL_DropsDeletedCharacters — the list query itself must use
// the pawn-keyed join. Keyed per account it re-grows the ghost rows.
func TestFetchPlayersSQL_DropsDeletedCharacters(t *testing.T) {
	t.Parallel()
	if !strings.Contains(fetchPlayersSQL, "ps2.player_pawn_id = a.id") {
		t.Fatalf("fetchPlayersSQL must bind the pawn to its own character row:\n%s", fetchPlayersSQL)
	}
	if strings.Contains(fetchPlayersSQL, "LEFT JOIN LATERAL") {
		t.Fatal("fetchPlayersSQL must not resolve the character per account — that is what showed deleted characters")
	}
	if strings.Contains(fetchPlayersSQL, barePlayerStateJoin) {
		t.Fatal("fetchPlayersSQL still contains the bare fan-out join")
	}
	// The canonical ordering still bounds the join to one row per pawn: the
	// game's schema migration dropped the unique constraint, so even a
	// pawn-keyed join can match duplicates (#290).
	if !strings.Contains(fetchPlayersSQL, canonicalOrderFragment) {
		t.Fatalf("fetchPlayersSQL must keep the canonical most-recently-active ordering:\n%s", fetchPlayersSQL)
	}
}

// TestPlayerCharacterQueriesExcludeDeletedCharacters — every actors-rooted
// query over PlayerCharacter actors counts the same soft-deleted pawns. If
// only the list were fixed, the Players dashboard would still report the
// inflated population (measured: 61 vs 51) against a list showing 51.
// Queries that need player_state columns use the pawn-keyed join; pure
// aggregates carry the EXISTS filter instead of growing a join.
func TestPlayerCharacterQueriesExcludeDeletedCharacters(t *testing.T) {
	t.Parallel()
	joined := map[string]string{
		"fetchPlayersSQL":      fetchPlayersSQL,
		"findPlayersByNameSQL": findPlayersByNameSQL,
		"serverCountsSQL":      serverCountsSQL,
		"serverByFactionSQL":   serverByFactionSQL,
	}
	for name, sql := range joined {
		if !strings.Contains(sql, "ps2.player_pawn_id = a.id") {
			t.Errorf("%s must resolve player_state by pawn, not by account", name)
		}
		if strings.Contains(sql, "LEFT JOIN LATERAL") {
			t.Errorf("%s must use the INNER pawn-keyed lateral so deleted characters drop out", name)
		}
	}

	filtered := map[string]string{
		"serverByMapSQL":          serverByMapSQL,
		"serverCharXPSQL":         serverCharXPSQL,
		"serverFactionXPSQL":      serverFactionXPSQL,
		"serverAccountFactionSQL": serverAccountFactionSQL,
	}
	for name, sql := range filtered {
		if !strings.Contains(sql, livingCharacterFilter) {
			t.Errorf("%s must exclude pawns whose character row is gone:\n%s", name, sql)
		}
	}
}
