package main

import "testing"

// TestSelectPlayerKey covers the account_id -> character_id key selection for
// per-character dune tables (issue #267). On current servers these tables are
// keyed by character_id (= the actor id); on legacy servers by account_id.
func TestSelectPlayerKey(t *testing.T) {
	t.Parallel()

	const actorID, accountID = int64(54), int64(2)

	t.Run("current schema keys by character_id with the actor id", func(t *testing.T) {
		t.Parallel()
		col, val := selectPlayerKey(playerKeyCharacterID, actorID, accountID)
		if col != playerKeyCharacterID || val != actorID {
			t.Fatalf("got (%q, %d), want (%q, %d)", col, val, playerKeyCharacterID, actorID)
		}
	})

	t.Run("legacy schema keys by account_id with the account id", func(t *testing.T) {
		t.Parallel()
		col, val := selectPlayerKey(playerKeyAccountID, actorID, accountID)
		if col != playerKeyAccountID || val != accountID {
			t.Fatalf("got (%q, %d), want (%q, %d)", col, val, playerKeyAccountID, accountID)
		}
	})

	t.Run("unknown column falls back to legacy account_id key", func(t *testing.T) {
		t.Parallel()
		// Defensive: anything that isn't the character_id sentinel is treated as
		// the legacy account-keyed schema rather than mis-addressing by actor id.
		col, val := selectPlayerKey("", actorID, accountID)
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
