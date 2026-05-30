package marketbot

import (
	"errors"
	"testing"
)

func TestExpireBotOrders_SkipsWhenGameNowZero(t *testing.T) {
	called := false
	err := expireBotOrders(0, 42, func(_, _ int64) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("deleteFn should not be called when gameNow is 0 (epoch not learned)")
	}
}

func TestExpireBotOrders_SkipsWhenGameNowNegative(t *testing.T) {
	called := false
	err := expireBotOrders(-1, 42, func(_, _ int64) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("deleteFn should not be called when gameNow is negative")
	}
}

func TestExpireBotOrders_DeletesWithCorrectArgs(t *testing.T) {
	const (
		gameNow int64 = 3000000
		ownerID int64 = 344
	)

	var gotOwner, gotCutoff int64
	err := expireBotOrders(gameNow, ownerID, func(owner, cutoff int64) error {
		gotOwner = owner
		gotCutoff = cutoff
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOwner != ownerID {
		t.Errorf("deleteFn ownerID = %d, want %d", gotOwner, ownerID)
	}
	if gotCutoff != gameNow {
		t.Errorf("deleteFn cutoff = %d, want %d (gameNow)", gotCutoff, gameNow)
	}
}

func TestExpireBotOrders_PropagatesDeleteError(t *testing.T) {
	dbErr := errors.New("connection lost")
	err := expireBotOrders(1000, 1, func(_, _ int64) error {
		return dbErr
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected wrapped db error, got %v", err)
	}
}

func TestExpireBotOrders_NeverTouchesPlayerOrders(t *testing.T) {
	// The deleteFn must only be called once — with the bot's ownerID.
	// If it were called with a different owner or without an owner filter,
	// player listings could be expired. The delete SQL (in the caller) must
	// include owner_id = ownerID; this test validates the ownerID is passed.
	const botOwnerID int64 = 344

	var calls []int64
	_ = expireBotOrders(3000000, botOwnerID, func(owner, _ int64) error {
		calls = append(calls, owner)
		return nil
	})
	if len(calls) != 1 {
		t.Fatalf("deleteFn called %d times, want 1", len(calls))
	}
	if calls[0] != botOwnerID {
		t.Errorf("deleteFn received ownerID=%d, want %d — player orders could be affected",
			calls[0], botOwnerID)
	}
}
