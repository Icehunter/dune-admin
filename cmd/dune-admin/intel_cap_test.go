package main

import "testing"

// TestIntelGrantDelta verifies the headroom clamp used by cmdAwardIntelCtx so a
// battlepass grant can never push a character above maxIntelPoints, leaving
// unspendable intel (#208). The delta is how much intel is actually added.
// TestIntelSetValue verifies the clamp used by cmdSetIntelCtx: a set-intel
// request lands exactly on the requested value bounded to [0, maxIntelPoints].
// Unlike intelGrantDelta this MAY reduce a balance — that is its purpose
// (cleaning up over-granted intel from the #293 incident).
func TestIntelSetValue(t *testing.T) {
	tests := []struct {
		name      string
		requested int64
		want      int64
	}{
		{"zero", 0, 0},
		{"negative clamps to zero", -50, 0},
		{"mid value passes through", 1200, 1200},
		{"exactly cap", maxIntelPoints, maxIntelPoints},
		{"over cap clamps to cap", maxIntelPoints + 500, maxIntelPoints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intelSetValue(tt.requested); got != tt.want {
				t.Errorf("intelSetValue(%d) = %d, want %d", tt.requested, got, tt.want)
			}
		})
	}
}

// TestIntelAuditOverages verifies the pure filter that keeps only characters
// whose intel exceeds the expected cumulative value for their level (#293
// cleanup: finds every character hit by the mass-grant).
func TestIntelAuditOverages(t *testing.T) {
	rows := []intelAuditRow{
		{PawnID: 1, Level: 5, Intel: intelAtLevel(5)},         // exactly expected
		{PawnID: 2, Level: 5, Intel: intelAtLevel(5) - 3},     // under
		{PawnID: 3, Level: 5, Intel: intelAtLevel(5) + 1},     // over by 1
		{PawnID: 4, Level: 8, Intel: 4000},                    // #293 victim
		{PawnID: 5, Level: 0, Intel: 1},                       // level-0 edge: expected 0, over
		{PawnID: 6, Level: 200, Intel: maxIntelPoints},        // at cap for max level
		{PawnID: 7, Level: 200, Intel: maxIntelPoints + 1000}, // over cap
	}
	got := intelAuditOverages(rows)
	wantPawns := []int64{3, 4, 5, 7}
	if len(got) != len(wantPawns) {
		t.Fatalf("got %d overage rows, want %d (%+v)", len(got), len(wantPawns), got)
	}
	for i, r := range got {
		if r.PawnID != wantPawns[i] {
			t.Errorf("row %d pawn = %d, want %d", i, r.PawnID, wantPawns[i])
		}
		if r.ExpectedIntel != intelAtLevel(r.Level) {
			t.Errorf("row %d expected_intel = %d, want intelAtLevel(%d)=%d",
				i, r.ExpectedIntel, r.Level, intelAtLevel(r.Level))
		}
	}
}

func TestIntelGrantDelta(t *testing.T) {
	tests := []struct {
		name      string
		current   int64
		requested int64
		want      int64
	}{
		{"under cap, fits", 100, 50, 50},
		{"under cap, would exceed clamps to headroom", maxIntelPoints - 10, 100, 10},
		{"exactly at cap grants nothing", maxIntelPoints, 100, 0},
		{"over cap (defensive) grants nothing", maxIntelPoints + 500, 100, 0},
		{"zero request", 100, 0, 0},
		{"negative request never reduces", 100, -50, 0},
		{"empty character fills to cap", 0, maxIntelPoints + 1000, maxIntelPoints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intelGrantDelta(tt.current, tt.requested); got != tt.want {
				t.Errorf("intelGrantDelta(%d, %d) = %d, want %d", tt.current, tt.requested, got, tt.want)
			}
		})
	}
}
