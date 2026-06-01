package main

import (
	"context"
	"errors"
	"testing"
)

// TestProcessWhisper exercises the whisper orchestration with injected deps (no
// DB/broker), mirroring the processGiveItems testing pattern. It asserts the
// resolved GM sender + recipient identities flow into send in the right slots and
// that each failure short-circuits the steps after it.
func TestProcessWhisper(t *testing.T) {
	t.Parallel()

	okGM := func(context.Context) (gmIdentity, error) {
		return gmIdentity{AccountID: gmIdentityAccountID, HexID: "GMHEX", FuncomID: "Server#0001"}, nil
	}

	t.Run("happy path passes resolved identities to send", func(t *testing.T) {
		t.Parallel()
		var got struct{ senderFuncom, senderHex, recipFuncom, recipName, msg string }
		err := processWhisper(context.Background(), 42, "hello", whisperDeps{
			getGM: okGM,
			resolveRecip: func(_ context.Context, accountID int64) (string, string, error) {
				if accountID != 42 {
					t.Fatalf("resolveRecip got account %d, want 42", accountID)
				}
				return "Tester#1234", "Tester", nil
			},
			send: func(sf, sh, rf, rn, m string) error {
				got.senderFuncom, got.senderHex, got.recipFuncom, got.recipName, got.msg = sf, sh, rf, rn, m
				return nil
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.senderFuncom != "Server#0001" || got.senderHex != "GMHEX" {
			t.Fatalf("sender identity wrong: %+v", got)
		}
		if got.recipFuncom != "Tester#1234" || got.recipName != "Tester" || got.msg != "hello" {
			t.Fatalf("recipient/message wrong: %+v", got)
		}
	})

	t.Run("gm not provisioned short-circuits before resolve/send", func(t *testing.T) {
		t.Parallel()
		called := false
		err := processWhisper(context.Background(), 42, "hi", whisperDeps{
			getGM:        func(context.Context) (gmIdentity, error) { return gmIdentity{}, errGMNotProvisioned },
			resolveRecip: func(context.Context, int64) (string, string, error) { called = true; return "", "", nil },
			send:         func(string, string, string, string, string) error { called = true; return nil },
		})
		if !errors.Is(err, errGMNotProvisioned) {
			t.Fatalf("want errGMNotProvisioned, got %v", err)
		}
		if called {
			t.Fatal("resolve/send must not run when GM identity is missing")
		}
	})

	t.Run("recipient resolve error short-circuits send", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("no such recipient")
		sent := false
		err := processWhisper(context.Background(), 42, "hi", whisperDeps{
			getGM:        okGM,
			resolveRecip: func(context.Context, int64) (string, string, error) { return "", "", boom },
			send:         func(string, string, string, string, string) error { sent = true; return nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
		if sent {
			t.Fatal("send must not run when recipient resolution fails")
		}
	})

	t.Run("send error propagates", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("broker down")
		err := processWhisper(context.Background(), 42, "hi", whisperDeps{
			getGM:        okGM,
			resolveRecip: func(context.Context, int64) (string, string, error) { return "r", "n", nil },
			send:         func(string, string, string, string, string) error { return boom },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})
}
