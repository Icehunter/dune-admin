package main

import (
	"path/filepath"
	"testing"
)

func TestWelcomeStoreLifecycle(t *testing.T) {
	s, err := openWelcomeStore(filepath.Join(t.TempDir(), "welcome.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = s.close() }()

	// Nothing granted initially.
	if ex, err := s.grantExists("P1", "v1", 10); err != nil || ex {
		t.Fatalf("expected no grant initially (ex=%v err=%v)", ex, err)
	}

	// A granted row registers as existing — the once-each gate.
	if err := s.insertGranted("P0", "v1", 9, "Duncan"); err != nil {
		t.Fatalf("insertGranted: %v", err)
	}
	if ex, _ := s.grantExists("P0", "v1", 9); !ex {
		t.Fatal("expected granted row to exist")
	}

	// A failed row ALSO gates retries (so we don't spam a broken account).
	if err := s.insertFailed("P1", "v1", 10, "Chani", "db timeout"); err != nil {
		t.Fatalf("insertFailed: %v", err)
	}
	if ex, _ := s.grantExists("P1", "v1", 10); !ex {
		t.Fatal("expected failed row to exist (gates retries)")
	}

	rows, err := s.listGrants(10)
	if err != nil {
		t.Fatalf("listGrants: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	var failed *welcomeGrantRecord
	for i := range rows {
		if rows[i].FlsID == "P1" {
			failed = &rows[i]
		}
	}
	if failed == nil || failed.Status != "failed" || failed.LastError != "db timeout" {
		t.Fatalf("failed row not recorded correctly: %+v", failed)
	}

	// Retry clears ONLY the failed row so the next scan re-attempts it.
	if n, err := s.deleteFailed("P1", "v1", 10); err != nil || n != 1 {
		t.Fatalf("deleteFailed on failed row: n=%d err=%v (want 1)", n, err)
	}
	if ex, _ := s.grantExists("P1", "v1", 10); ex {
		t.Fatal("failed row should be cleared after retry")
	}

	// A granted row is NEVER removed by retry — items can't duplicate.
	if n, _ := s.deleteFailed("P0", "v1", 9); n != 0 {
		t.Fatalf("granted row must not be deletable via retry, got %d", n)
	}
	if ex, _ := s.grantExists("P0", "v1", 9); !ex {
		t.Fatal("granted row must remain")
	}

	// Version re-issue: bumping the package version makes the same player
	// eligible again (the ledger key includes the version).
	if ex, _ := s.grantExists("P0", "v2", 9); ex {
		t.Fatal("a new package version should not be granted yet")
	}
}
