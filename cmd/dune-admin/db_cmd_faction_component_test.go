package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// buildFactionDataArray is the pure heart of the in-game rank fix: it produces
// the FactionPlayerComponent.m_FactionDataArray cache the game reads for rank
// and per-territory vendor gating. It must always emit BOTH great houses
// (Atreides=1, Harkonnen=2), defaulting a missing house to 0, and ignore any
// non-great-house faction ids (None=3, Smuggler=4).
func TestBuildFactionDataArray(t *testing.T) {
	t.Parallel()

	const ts = 1780198964.0

	repOf := func(entries []factionDataEntry, name string) (int32, bool) {
		for _, e := range entries {
			if e.Faction.Name == name {
				return e.ReputationAmount, true
			}
		}
		return 0, false
	}

	tests := []struct {
		name     string
		reps     map[int16]int32
		wantAtre int32
		wantHark int32
	}{
		{name: "both houses present", reps: map[int16]int32{1: 1500, 2: 2000}, wantAtre: 1500, wantHark: 2000},
		{name: "only harkonnen → atreides defaults 0", reps: map[int16]int32{2: 2000}, wantAtre: 0, wantHark: 2000},
		{name: "only atreides → harkonnen defaults 0", reps: map[int16]int32{1: 1500}, wantAtre: 1500, wantHark: 0},
		{name: "empty → both 0", reps: map[int16]int32{}, wantAtre: 0, wantHark: 0},
		{name: "ignores none and smuggler", reps: map[int16]int32{1: 100, 3: 50, 4: 999}, wantAtre: 100, wantHark: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildFactionDataArray(tt.reps, ts)

			// Exactly the two great houses, always.
			if len(got) != 2 {
				t.Fatalf("expected 2 entries (both great houses), got %d: %+v", len(got), got)
			}
			atre, okA := repOf(got, "Atreides")
			hark, okH := repOf(got, "Harkonnen")
			if !okA || !okH {
				t.Fatalf("expected both Atreides and Harkonnen entries, got %+v", got)
			}
			if atre != tt.wantAtre {
				t.Fatalf("Atreides rep: want %d, got %d", tt.wantAtre, atre)
			}
			if hark != tt.wantHark {
				t.Fatalf("Harkonnen rep: want %d, got %d", tt.wantHark, hark)
			}
			// timestamp propagated to every entry.
			for _, e := range got {
				if e.Timestamp != ts {
					t.Fatalf("entry %s timestamp: want %v, got %v", e.Faction.Name, ts, e.Timestamp)
				}
			}
		})
	}
}

// The marshaled shape must match what the game reads exactly:
// {"Faction":{"Name":"Harkonnen"},"timestamp":<float>,"ReputationAmount":<int>}
func TestFactionDataEntryJSONShape(t *testing.T) {
	t.Parallel()

	arr := buildFactionDataArray(map[int16]int32{2: 2000}, 1780198964.5)
	raw, err := json.Marshal(arr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back []map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var hark map[string]any
	for _, e := range back {
		if f, ok := e["Faction"].(map[string]any); ok && f["Name"] == "Harkonnen" {
			hark = e
		}
	}
	if hark == nil {
		t.Fatalf("no Harkonnen entry in %s", raw)
	}
	if _, ok := hark["Faction"].(map[string]any)["Name"]; !ok {
		t.Fatalf("missing Faction.Name in %s", raw)
	}
	if _, ok := hark["timestamp"]; !ok {
		t.Fatalf("missing timestamp key in %s", raw)
	}
	if rep, ok := hark["ReputationAmount"]; !ok || rep.(float64) != 2000 {
		t.Fatalf("ReputationAmount wrong in %s", raw)
	}
}

// stubExecer lets us test writeFactionComponent's row-count guard without a real DB.
type stubExecer struct {
	tag     pgconn.CommandTag
	err     error
	gotSQL  string
	gotArgs []any
}

func (s *stubExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	s.gotSQL = sql
	s.gotArgs = args
	return s.tag, s.err
}

func TestWriteFactionComponent(t *testing.T) {
	t.Parallel()

	arr := buildFactionDataArray(map[int16]int32{2: 2000}, 1.0)

	t.Run("one row affected → success", func(t *testing.T) {
		t.Parallel()
		s := &stubExecer{tag: pgconn.NewCommandTag("UPDATE 1")}
		if err := writeFactionComponent(context.Background(), s, 17, arr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.gotArgs) != 2 {
			t.Fatalf("expected 2 args (payload, controllerID), got %d", len(s.gotArgs))
		}
	})

	t.Run("zero rows → error (kills the silent no-op)", func(t *testing.T) {
		t.Parallel()
		s := &stubExecer{tag: pgconn.NewCommandTag("UPDATE 0")}
		err := writeFactionComponent(context.Background(), s, 999, arr)
		if err == nil {
			t.Fatalf("expected error when no row updated, got nil")
		}
	})

	t.Run("exec error is wrapped", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("connection refused")
		s := &stubExecer{err: sentinel}
		err := writeFactionComponent(context.Background(), s, 17, arr)
		if err == nil || !errors.Is(err, sentinel) {
			t.Fatalf("expected wrapped exec error, got %v", err)
		}
	})
}
