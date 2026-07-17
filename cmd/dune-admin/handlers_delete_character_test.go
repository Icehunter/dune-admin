package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProcessDeleteCharacter exercises validation and orchestration with
// injected deps — no DB needed.
func TestProcessDeleteCharacter(t *testing.T) {
	t.Parallel()

	t.Run("happy path: resolves user then deletes", func(t *testing.T) {
		t.Parallel()
		var resolvedID int64
		var gotUser, gotReason string
		err := processDeleteCharacter(deleteCharacterParams{
			accountID: 42,
			reason:    "ban evasion",
			resolveUser: func(id int64) (string, error) {
				resolvedID = id
				return "DEADBEEF", nil
			},
			deleteAccount: func(user, reason string) (bool, error) {
				gotUser = user
				gotReason = reason
				return true, nil
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolvedID != 42 {
			t.Fatalf("resolveUser got id %d, want 42", resolvedID)
		}
		if gotUser != "DEADBEEF" || gotReason != "ban evasion" {
			t.Fatalf("deleteAccount called with user=%q reason=%q", gotUser, gotReason)
		}
	})

	t.Run("zero account id rejected", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     0,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { t.Error("resolveUser must not be called"); return "", nil },
			deleteAccount: func(string, string) (bool, error) { t.Error("deleteAccount must not be called"); return false, nil },
		})
		if err == nil {
			t.Fatal("expected error for zero account id")
		}
	})

	t.Run("empty reason rejected", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "   ",
			resolveUser:   func(int64) (string, error) { t.Error("resolveUser must not be called"); return "", nil },
			deleteAccount: func(string, string) (bool, error) { t.Error("deleteAccount must not be called"); return false, nil },
		})
		if err == nil {
			t.Fatal("expected error for empty reason")
		}
	})

	t.Run("resolveUser error propagates", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("no such account")
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "", boom },
			deleteAccount: func(string, string) (bool, error) { t.Error("deleteAccount must not be called"); return false, nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("empty resolved user rejected", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "", nil },
			deleteAccount: func(string, string) (bool, error) { t.Error("deleteAccount must not be called"); return false, nil },
		})
		if err == nil {
			t.Fatal("expected error for empty resolved user")
		}
	})

	t.Run("deleteAccount error propagates", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("db down")
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return false, boom },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("not found (returns false) is an error", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return false, nil },
		})
		if err == nil {
			t.Fatal("expected error when delete_account reports no rows affected")
		}
	})

	// #290: dune.delete_account never deletes the orphaned dune.player_state
	// row it leaves behind, which is what causes duplicate Players-list rows
	// and give-items/teleport to resolve against a stale pawn actor after a
	// deletion. cleanupOrphans is the injected hook that cleans that up —
	// it must run only after a genuinely successful delete, never otherwise.

	t.Run("cleanupOrphans called after successful delete", func(t *testing.T) {
		t.Parallel()
		called := false
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return true, nil },
			cleanupOrphans: func() error {
				called = true
				return nil
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("cleanupOrphans must be called after a successful delete")
		}
	})

	t.Run("cleanupOrphans not called when deleteAccount fails", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("db down")
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return false, boom },
			cleanupOrphans: func() error {
				t.Error("cleanupOrphans must not be called when deleteAccount fails")
				return nil
			},
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("cleanupOrphans not called when deleteAccount reports not found", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return false, nil },
			cleanupOrphans: func() error {
				t.Error("cleanupOrphans must not be called when delete_account reports no rows affected")
				return nil
			},
		})
		if err == nil {
			t.Fatal("expected error when delete_account reports no rows affected")
		}
	})

	t.Run("cleanupOrphans error propagates", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("cleanup boom")
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:      42,
			reason:         "x",
			resolveUser:    func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount:  func(string, string) (bool, error) { return true, nil },
			cleanupOrphans: func() error { return boom },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want cleanup error to propagate wrapping boom, got %v", err)
		}
	})

	t.Run("nil cleanupOrphans is fine", func(t *testing.T) {
		t.Parallel()
		err := processDeleteCharacter(deleteCharacterParams{
			accountID:     42,
			reason:        "x",
			resolveUser:   func(int64) (string, error) { return "DEADBEEF", nil },
			deleteAccount: func(string, string) (bool, error) { return true, nil },
			// cleanupOrphans intentionally omitted (nil) — must not panic.
		})
		if err != nil {
			t.Fatalf("unexpected error with nil cleanupOrphans: %v", err)
		}
	})
}

// TestHandleDeleteCharacter_InputValidation verifies bad input returns 400.
func TestHandleDeleteCharacter_InputValidation(t *testing.T) {
	tests := []struct {
		name       string
		rawBody    []byte
		wantStatus int
	}{
		{
			name:       "missing account_id",
			rawBody:    []byte(`{"reason":"ban"}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "zero account_id",
			rawBody:    []byte(`{"account_id":0,"reason":"ban"}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty reason",
			rawBody:    []byte(`{"account_id":42,"reason":""}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "whitespace reason",
			rawBody:    []byte(`{"account_id":42,"reason":"   "}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bad json",
			rawBody:    []byte(`{bad`),
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/players/delete", bytes.NewReader(tt.rawBody))
			rec := httptest.NewRecorder()
			handleDeleteCharacter(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHandleDeleteCharacter_NilDB verifies a valid request with no DB returns 500.
func TestHandleDeleteCharacter_NilDB(t *testing.T) {
	// NOT parallel — reads globalDB package global (nil in tests).
	body, _ := json.Marshal(map[string]any{"account_id": 42, "reason": "ban"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/players/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleDeleteCharacter(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 (nil DB), got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestPlayersDeleteCapability verifies the new capability is registered and
// is NOT part of the read-only default seed.
func TestPlayersDeleteCapability(t *testing.T) {
	t.Parallel()
	if _, ok := allCapabilities[capPlayersDelete]; !ok {
		t.Fatal("allCapabilities missing players:delete")
	}
	for _, name := range defaultSeedCaps() {
		if name == string(capPlayersDelete) {
			t.Fatal("players:delete must not be in the read-only default seed")
		}
	}
}
