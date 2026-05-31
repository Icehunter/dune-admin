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
