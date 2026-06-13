package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestValidateWelcomeItems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		items   []welcomePackageItem
		wantErr bool
	}{
		{"empty list", nil, true},
		{"empty template", []welcomePackageItem{{Template: "", Qty: 1}}, true},
		{"zero qty", []welcomePackageItem{{Template: "PlantFiber", Qty: 0}}, true},
		{"negative quality", []welcomePackageItem{{Template: "PlantFiber", Qty: 1, Quality: -1}}, true},
		{"valid", []welcomePackageItem{{Template: "PlantFiber", Qty: 5, Quality: 0}}, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWelcomeItems(tt.items)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateWelcomeItems(%+v) err=%v, wantErr=%v", tt.items, err, tt.wantErr)
			}
		})
	}
}

func TestOverrideGrantToAccount(t *testing.T) {
	store, err := openWelcomeStore(filepath.Join(t.TempDir(), "w.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.close() }()

	items := []welcomePackageItem{{Template: "PlantFiber", Qty: 2, Quality: 0}}

	t.Run("bypasses the already-granted guard and re-grants", func(t *testing.T) {
		// Pre-grant so a normal scan would skip — override must NOT skip.
		if err := store.insertGranted("FLS1", "v1", 1, "Paul"); err != nil {
			t.Fatal(err)
		}
		var granted bool
		acc := welcomeAccount{AccountID: 1, PawnID: 10, FlsID: "FLS1", CharacterName: "Paul"}
		grantErr := overrideGrantToAccount(context.Background(), acc, "v1", items, welcomeScanDeps{
			grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
				granted = true
				return nil, nil
			},
			store: store,
		})
		if grantErr != nil {
			t.Fatalf("override: %v", grantErr)
		}
		if !granted {
			t.Fatal("override should re-grant even when a ledger row exists")
		}
		if ex, _ := store.grantExists("FLS1", "v1", 1); !ex {
			t.Fatal("ledger row should exist after override")
		}
	})

	t.Run("rejects empty FLS id", func(t *testing.T) {
		acc := welcomeAccount{AccountID: 2, PawnID: 20, FlsID: "", CharacterName: "NoFls"}
		err := overrideGrantToAccount(context.Background(), acc, "v1", items, welcomeScanDeps{
			grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
				t.Fatal("grant must not be called for empty FLS id")
				return nil, nil
			},
			store: store,
		})
		if err == nil {
			t.Fatal("override should reject empty FLS id")
		}
	})

	t.Run("records failed and errors on grant error", func(t *testing.T) {
		acc := welcomeAccount{AccountID: 3, PawnID: 30, FlsID: "FLS3", CharacterName: "Chani"}
		err := overrideGrantToAccount(context.Background(), acc, "v1", items, welcomeScanDeps{
			grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
				return nil, errors.New("rmq down")
			},
			store: store,
		})
		if err == nil {
			t.Fatal("override should return an error when grant fails")
		}
		row, ok, _ := store.findGrant("FLS3", "v1", 3)
		if !ok || row.Status != "failed" {
			t.Fatalf("expected failed ledger row, ok=%v row=%+v", ok, row)
		}
	})

	t.Run("records failed and errors on skipped items", func(t *testing.T) {
		acc := welcomeAccount{AccountID: 4, PawnID: 40, FlsID: "FLS4", CharacterName: "Stilgar"}
		err := overrideGrantToAccount(context.Background(), acc, "v1", items, welcomeScanDeps{
			grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
				return []string{"PlantFiber: inventory full"}, nil
			},
			store: store,
		})
		if err == nil {
			t.Fatal("override should return an error when items are skipped")
		}
		row, ok, _ := store.findGrant("FLS4", "v1", 4)
		if !ok || row.Status != "failed" {
			t.Fatalf("expected failed ledger row for skipped items, ok=%v row=%+v", ok, row)
		}
	})
}

func TestWelcomePackageScanOnce(t *testing.T) {
	store, err := openWelcomeStore(filepath.Join(t.TempDir(), "w.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.close() }()

	// Pre-grant account 9 so the scan skips it (idempotency).
	if err := store.insertGranted("FLS9", "v1", 9, "Old"); err != nil {
		t.Fatal(err)
	}

	accounts := []welcomeAccount{
		{AccountID: 9, PawnID: 90, FlsID: "FLS9", CharacterName: "Old"},      // already granted → skip
		{AccountID: 10, PawnID: 100, FlsID: "FLS10", CharacterName: "Paul"},  // clean → granted
		{AccountID: 11, PawnID: 110, FlsID: "FLS11", CharacterName: "Chani"}, // skipped item → failed
		{AccountID: 12, PawnID: 120, FlsID: "", CharacterName: "NoFls"},      // no fls → ignored entirely
	}
	items := []welcomePackageItem{{Template: "PlantFiber", Qty: 2, Quality: 0}}

	grant := func(_ context.Context, pawnID int64, _ string, _ []welcomePackageItem) ([]string, error) {
		switch pawnID {
		case 100:
			return nil, nil // success
		case 110:
			return []string{"PlantFiber: inventory full"}, nil // partial → recorded failed
		default:
			return nil, errors.New("unexpected pawn id in test")
		}
	}

	g, f, skipped, err := welcomePackageScanOnce(context.Background(), "v1", items, welcomeScanDeps{
		listAccounts: func(context.Context) ([]welcomeAccount, error) { return accounts, nil },
		grant:        grant,
		store:        store,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if g != 1 {
		t.Fatalf("granted: want 1, got %d", g)
	}
	if f != 1 {
		t.Fatalf("failed: want 1, got %d", f)
	}
	if skipped != 1 {
		t.Fatalf("skipped (already granted): want 1, got %d", skipped)
	}

	if ex, _ := store.grantExists("FLS10", "v1", 10); !ex {
		t.Fatal("account 10 should be granted")
	}
	if ex, _ := store.grantExists("FLS11", "v1", 11); !ex {
		t.Fatal("account 11 should have a failed ledger row")
	}
	if ex, _ := store.grantExists("", "v1", 12); ex {
		t.Fatal("no-fls account must not be recorded")
	}
}
