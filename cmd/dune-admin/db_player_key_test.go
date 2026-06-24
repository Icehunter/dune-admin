package main

import "testing"

// TestSelectPlayerKey covers the account_id -> character_id key selection for
// per-character dune tables (issue #267). On current servers these tables are
// keyed by character_id (= encrypted_player_state.id, resolved from the account
// id); on legacy servers by account_id.
func TestSelectPlayerKey(t *testing.T) {
	t.Parallel()

	const characterID, accountID = int64(2), int64(51)

	t.Run("current schema keys by character_id", func(t *testing.T) {
		t.Parallel()
		col, val := selectPlayerKey(playerKeyCharacterID, characterID, accountID)
		if col != playerKeyCharacterID || val != characterID {
			t.Fatalf("got (%q, %d), want (%q, %d)", col, val, playerKeyCharacterID, characterID)
		}
	})

	t.Run("legacy schema keys by account_id", func(t *testing.T) {
		t.Parallel()
		col, val := selectPlayerKey(playerKeyAccountID, characterID, accountID)
		if col != playerKeyAccountID || val != accountID {
			t.Fatalf("got (%q, %d), want (%q, %d)", col, val, playerKeyAccountID, accountID)
		}
	})

	t.Run("unknown column falls back to legacy account_id key", func(t *testing.T) {
		t.Parallel()
		// Defensive: anything that isn't the character_id sentinel is treated as
		// the legacy account-keyed schema rather than mis-addressing by character id.
		col, val := selectPlayerKey("", characterID, accountID)
		if col != playerKeyAccountID || val != accountID {
			t.Fatalf("got (%q, %d), want (%q, %d)", col, val, playerKeyAccountID, accountID)
		}
	})
}

// TestPlayerKeyColumnFromProbe verifies the information_schema probe result is
// mapped to the right key column.
func TestPlayerKeyColumnFromProbe(t *testing.T) {
	t.Parallel()

	if got := playerKeyColumnFromProbe(true); got != playerKeyCharacterID {
		t.Fatalf("character_id present: got %q, want %q", got, playerKeyCharacterID)
	}
	if got := playerKeyColumnFromProbe(false); got != playerKeyAccountID {
		t.Fatalf("character_id absent: got %q, want %q", got, playerKeyAccountID)
	}
}
