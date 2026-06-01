package main

import (
	"strings"
	"testing"
)

// TestGMSeedSpec locks the recon-derived seed values for the GM/Server persona:
// the sentinel ids (collision-free per Phase 0 recon), the exact actor class paths
// the live schema uses (or the game's player-info lookup fails and the sender never
// renders), and the blast-radius-safe defaults (Offline status; the seed routine
// leaves actors.transform NULL so the GM never plots on the live map).
func TestGMSeedSpec(t *testing.T) {
	t.Parallel()
	s := gmSeedSpec()

	if s.AccountID != gmIdentityAccountID {
		t.Fatalf("AccountID = %d, want %d", s.AccountID, gmIdentityAccountID)
	}
	// Actor ids derive from the account id: 9000001 -> 900000101/02/03.
	if s.ControllerID != 900000101 || s.StateID != 900000102 || s.PawnID != 900000103 {
		t.Fatalf("actor ids wrong: %d/%d/%d", s.ControllerID, s.StateID, s.PawnID)
	}
	if !strings.Contains(s.ControllerClass, "BP_DunePlayerController") {
		t.Fatalf("controller class wrong: %s", s.ControllerClass)
	}
	if !strings.Contains(s.StateClass, "DunePlayerState") {
		t.Fatalf("state class wrong: %s", s.StateClass)
	}
	if !strings.Contains(s.PawnClass, "BP_DunePlayerCharacter") {
		t.Fatalf("pawn class wrong: %s", s.PawnClass)
	}
	// Blast-radius: Offline keeps the GM out of the online pollers / welcome scanner.
	if s.OnlineStatus != "Offline" {
		t.Fatalf("OnlineStatus = %q, want Offline", s.OnlineStatus)
	}
	if s.LifeState != "Alive" {
		t.Fatalf("LifeState = %q, want Alive", s.LifeState)
	}
	if s.FuncomID != "GM#0001" || s.CharacterName != "GM" {
		t.Fatalf("persona wrong: funcom=%q char=%q", s.FuncomID, s.CharacterName)
	}
	if s.Map != "HaggaBasin" || s.PartitionID != 1 {
		t.Fatalf("location wrong: map=%q partition=%d", s.Map, s.PartitionID)
	}
}
