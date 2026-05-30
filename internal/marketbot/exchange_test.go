package marketbot

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// newTestExchange returns an Exchange with a temporary file-based SQLite cache
// suitable for unit tests that exercise epoch learning (which writes to e.cache).
func newTestExchange(t *testing.T) *Exchange {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-cache.db")
	cache, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test sqlite: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if _, err := cache.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		key   TEXT    PRIMARY KEY,
		value INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("init metadata table: %v", err)
	}
	return &Exchange{cache: cache}
}

func TestEnsureCachePathCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "nested", "cache", "market-bot.db")

	resolved, err := ensureCachePath(cachePath)
	if err != nil {
		t.Fatalf("ensureCachePath returned error: %v", err)
	}
	if resolved != cachePath {
		t.Fatalf("ensureCachePath returned %q, want %q", resolved, cachePath)
	}
	if _, err := os.Stat(filepath.Dir(cachePath)); err != nil {
		t.Fatalf("expected cache directory to exist: %v", err)
	}
}

func TestEnsureCachePathRejectsEmptyPath(t *testing.T) {
	if _, err := ensureCachePath("   "); err == nil {
		t.Fatalf("ensureCachePath should reject empty path")
	}
}

func TestConfigValuesIsDisabled(t *testing.T) {
	snap := configValues{
		DisabledItems: []string{"item.sword", "item.shield"},
	}
	if !snap.isDisabled("item.sword") {
		t.Error("item.sword should be disabled")
	}
	if !snap.isDisabled("item.shield") {
		t.Error("item.shield should be disabled")
	}
	if snap.isDisabled("item.axe") {
		t.Error("item.axe should NOT be disabled")
	}
	if snap.isDisabled("") {
		t.Error("empty string should NOT be disabled")
	}

	// Case-insensitive match.
	if !snap.isDisabled("ITEM.SWORD") {
		t.Error("isDisabled should be case-insensitive")
	}
}

func TestConfigValuesIsDisabledEmpty(t *testing.T) {
	snap := configValues{}
	if snap.isDisabled("anything") {
		t.Error("empty DisabledItems list should never block any item")
	}
}

// TestEpochSentinelCutoffExcludesSentinel verifies that the sentinel
// expiration_time value (999_999_999) is excluded from epoch detection.
// The SQL query uses WHERE expiration_time < epochSentinelCutoff, so the
// sentinel itself must equal epochSentinelCutoff to be excluded by the strict
// less-than comparison.
func TestEpochSentinelCutoffExcludesSentinel(t *testing.T) {
	t.Parallel()

	// The sentinel must equal the cutoff so that "< epochSentinelCutoff" in
	// the SQL excludes it. If the cutoff were > 999_999_999, sentinel listings
	// would pass the filter and corrupt the epoch calculation.
	if epochSentinelCutoff != 999_999_999 {
		t.Errorf("epochSentinelCutoff = %d, want 999_999_999 "+
			"(SQL uses < epochSentinelCutoff to exclude sentinel listings)",
			epochSentinelCutoff)
	}
}

// TestApplyLearnedEpoch_SkipsOnError verifies that applyLearnedEpoch does not
// modify the Exchange when fetchRef returns an error.
func TestApplyLearnedEpoch_SkipsOnError(t *testing.T) {
	t.Parallel()

	ex := &Exchange{gameEpochUnix: 12345}
	applyLearnedEpoch(ex, func() (int64, error) {
		return 0, errors.New("no rows")
	})
	if ex.gameEpochUnix != 12345 {
		t.Errorf("gameEpochUnix changed on error: got %d, want 12345", ex.gameEpochUnix)
	}
}

// TestApplyLearnedEpoch_SkipsOnZeroRef verifies that applyLearnedEpoch does not
// modify the Exchange when fetchRef returns 0 (no matching row).
func TestApplyLearnedEpoch_SkipsOnZeroRef(t *testing.T) {
	t.Parallel()

	ex := &Exchange{gameEpochUnix: 99}
	applyLearnedEpoch(ex, func() (int64, error) {
		return 0, nil
	})
	if ex.gameEpochUnix != 99 {
		t.Errorf("gameEpochUnix changed on zero ref: got %d, want 99", ex.gameEpochUnix)
	}
}

// TestLearnGameEpoch_FallsBackToPlayerListingsOnBootstrap verifies the two-tier
// epoch detection strategy:
//  1. First tries bot's own non-sentinel listings (fast, accurate path).
//  2. Falls back to player listings when no qualifying bot listings exist.
//
// This covers the bootstrap case: on a fresh install or after cache clearing,
// the only bot listings are sentinel listings (expiration_time = 999_999_999).
// The first tier returns no rows, so the second tier (player listings) must be
// tried so the bot can bootstrap the epoch from real player expirations.
func TestLearnGameEpoch_FallsBackToPlayerListingsOnBootstrap(t *testing.T) {
	t.Parallel()

	// On bootstrap: bot has only sentinel listings — tier 1 returns no rows.
	// Player listing provides a real expiration_time.
	const playerExpiry = int64(1_800_000)

	ex := newTestExchange(t)
	ex.gameEpochUnix = 0
	calls := 0
	applyLearnedEpochTwoTier(ex,
		func() (int64, error) {
			calls++
			return 0, pgx.ErrNoRows // no non-sentinel bot listings
		},
		func() (int64, error) {
			calls++
			return playerExpiry, nil // player listing found
		},
	)

	if calls != 2 {
		t.Errorf("expected 2 fetch calls (tier1 miss + tier2 hit), got %d", calls)
	}
	if ex.gameEpochUnix == 0 {
		t.Error("gameEpochUnix should have been updated from player listing fallback")
	}
}

// TestLearnGameEpoch_UsesBotListingsWhenAvailable verifies that when the bot has
// non-sentinel listings, tier 1 succeeds and tier 2 is never called.
func TestLearnGameEpoch_UsesBotListingsWhenAvailable(t *testing.T) {
	t.Parallel()

	const botExpiry = int64(1_900_000)

	ex := newTestExchange(t)
	ex.gameEpochUnix = 0
	tier2Called := false
	applyLearnedEpochTwoTier(ex,
		func() (int64, error) {
			return botExpiry, nil // bot has a real listing
		},
		func() (int64, error) {
			tier2Called = true
			return 0, pgx.ErrNoRows
		},
	)

	if tier2Called {
		t.Error("tier 2 (player listing fallback) should not be called when tier 1 succeeds")
	}
	if ex.gameEpochUnix == 0 {
		t.Error("gameEpochUnix should have been updated from bot listing")
	}
}

// TestLearnGameEpoch_BothTiersMissDoesNotUpdate verifies that when both tiers
// return no rows, the epoch is left unchanged (no spurious zero write).
func TestLearnGameEpoch_BothTiersMissDoesNotUpdate(t *testing.T) {
	t.Parallel()

	ex := newTestExchange(t)
	ex.gameEpochUnix = 42
	applyLearnedEpochTwoTier(ex,
		func() (int64, error) { return 0, pgx.ErrNoRows },
		func() (int64, error) { return 0, pgx.ErrNoRows },
	)

	if ex.gameEpochUnix != 42 {
		t.Errorf("gameEpochUnix changed unexpectedly: got %d, want 42", ex.gameEpochUnix)
	}
}

func TestDetectExchangeID(t *testing.T) {
	errNoRows := pgx.ErrNoRows

	tests := []struct {
		name       string
		fromOrders func() (int64, error)
		fromTable  func() (int64, error)
		autoCreate func() (int64, error)
		wantID     int64
		wantErr    bool
	}{
		{
			name:       "found in player orders",
			fromOrders: func() (int64, error) { return 7, nil },
			fromTable:  func() (int64, error) { panic("should not be called") },
			autoCreate: func() (int64, error) { panic("should not be called") },
			wantID:     7,
		},
		{
			name:       "found in dune_exchanges table",
			fromOrders: func() (int64, error) { return 0, errNoRows },
			fromTable:  func() (int64, error) { return 3, nil },
			autoCreate: func() (int64, error) { panic("should not be called") },
			wantID:     3,
		},
		{
			name:       "auto-creates via upsert when table empty",
			fromOrders: func() (int64, error) { return 0, errNoRows },
			fromTable:  func() (int64, error) { return 0, errNoRows },
			autoCreate: func() (int64, error) { return 1, nil },
			wantID:     1,
		},
		{
			name:       "all three fail → error",
			fromOrders: func() (int64, error) { return 0, errNoRows },
			fromTable:  func() (int64, error) { return 0, errNoRows },
			autoCreate: func() (int64, error) { return 0, errNoRows },
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := detectExchangeID(tt.fromOrders, tt.fromTable, tt.autoCreate)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("got id=%d want %d", id, tt.wantID)
			}
		})
	}
}
