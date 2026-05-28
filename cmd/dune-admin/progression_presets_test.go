package main

import (
	"errors"
	"testing"
)

func TestProgressionPresetByID(t *testing.T) {
	t.Parallel()

	preset := progressionPresetByID("skip_npe")
	if preset == nil {
		t.Fatalf("expected known preset to be found")
	}
	if preset.Name == "" || len(preset.Nodes) == 0 {
		t.Fatalf("unexpected preset payload: %+v", preset)
	}
	if progressionPresetByID("missing") != nil {
		t.Fatalf("expected unknown preset lookup to return nil")
	}
}

func TestApplyProgressionPresetNodes(t *testing.T) {
	t.Parallel()

	var calls []string
	nodes, tags, err := applyProgressionPresetNodes(77, "p1", []string{"A", "B"}, func(accountID int64, nodeID string) (msgMutate, bool) {
		if accountID != 77 {
			t.Fatalf("unexpected accountID: %d", accountID)
		}
		calls = append(calls, nodeID)
		switch nodeID {
		case "A":
			return msgMutate{ok: "Completed A + 4 node(s), +2 tag(s) — takes effect on next login"}, true
		case "B":
			return msgMutate{ok: "Completed B + 1 node(s) — takes effect on next login"}, true
		default:
			t.Fatalf("unexpected node %q", nodeID)
			return msgMutate{}, false
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodes != 7 || tags != 2 {
		t.Fatalf("unexpected totals: nodes=%d tags=%d", nodes, tags)
	}
	if len(calls) != 2 || calls[0] != "A" || calls[1] != "B" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestApplyProgressionPresetNodes_Errors(t *testing.T) {
	t.Parallel()

	_, _, err := applyProgressionPresetNodes(1, "p2", []string{"A"}, func(int64, string) (msgMutate, bool) {
		return msgMutate{}, false
	})
	if err == nil || err.Error() != "apply p2 (node A): internal error" {
		t.Fatalf("unexpected internal-error result: %v", err)
	}

	_, _, err = applyProgressionPresetNodes(1, "p3", []string{"B"}, func(int64, string) (msgMutate, bool) {
		return msgMutate{err: errors.New("nope")}, true
	})
	if err == nil || err.Error() != "apply p3 (node B): nope" {
		t.Fatalf("unexpected wrapped error: %v", err)
	}
}
