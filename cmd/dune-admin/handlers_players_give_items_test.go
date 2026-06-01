package main

import (
	"context"
	"errors"
	"testing"
)

func TestResolveGiveItemsOnlinePath(t *testing.T) {
	t.Parallel()

	offline := func(context.Context, int64) error { return nil }
	online := func(context.Context, int64) error { return errors.New("online") }
	resolve := func(context.Context, int64) (string, error) { return "fls-123", nil }
	failResolve := func(context.Context, int64) (string, error) { return "", errors.New("boom") }

	if on, fls := resolveGiveItemsOnlinePath(context.Background(), 0, online, resolve); on || fls != "" {
		t.Fatalf("expected playerID=0 to force DB path, got on=%v fls=%q", on, fls)
	}
	if on, fls := resolveGiveItemsOnlinePath(context.Background(), 42, offline, resolve); on || fls != "" {
		t.Fatalf("expected offline player to use DB path, got on=%v fls=%q", on, fls)
	}
	if on, fls := resolveGiveItemsOnlinePath(context.Background(), 42, online, resolve); !on || fls != "fls-123" {
		t.Fatalf("expected online RMQ path with fls id, got on=%v fls=%q", on, fls)
	}
	if on, fls := resolveGiveItemsOnlinePath(context.Background(), 42, online, failResolve); on || fls != "" {
		t.Fatalf("expected FLS resolve failure to fall back to DB, got on=%v fls=%q", on, fls)
	}
}

func TestProcessGiveItems(t *testing.T) {
	t.Parallel()

	req := giveItemsRequest{
		PlayerID: 11,
		Items: []giveItemInput{
			{Template: "A", Qty: 2, Quality: 0},
			{Template: "B", Qty: 1, Quality: 5},
			{Template: "C", Qty: 1, Quality: 0},
		},
	}

	var rmqSent []string
	given, skipped := processGiveItems(context.Background(), req, true, "fls-123", giveItemsDeps{
		checkCapacity: func(_ context.Context, _ int64, template string, _ int64) error {
			if template == "C" {
				return errors.New("inventory full")
			}
			return nil
		},
		rmqAdd: func(_ string, template string, _ int, _ float64) error {
			rmqSent = append(rmqSent, template)
			return nil
		},
		dbGive: func(_ int64, template string, _, _ int64) (msgMutate, bool) {
			if template != "B" {
				t.Fatalf("unexpected DB call for template %q", template)
			}
			return msgMutate{ok: "done"}, true
		},
		needsDBPath: func(string) bool { return false },
	})

	if len(given) != 2 || given[0] != "A" || given[1] != "B" {
		t.Fatalf("unexpected given: %v", given)
	}
	if len(skipped) != 1 || skipped[0].Template != "C" || skipped[0].Reason != "inventory full" {
		t.Fatalf("unexpected skipped: %+v", skipped)
	}
	if len(rmqSent) != 1 || rmqSent[0] != "A" {
		t.Fatalf("unexpected RMQ calls: %v", rmqSent)
	}
}

func TestProcessGiveItems_DBFailureReasons(t *testing.T) {
	t.Parallel()

	req := giveItemsRequest{
		PlayerID: 11,
		Items: []giveItemInput{
			{Template: "X", Qty: 1, Quality: 9},
			{Template: "Y", Qty: 1, Quality: 9},
		},
	}

	given, skipped := processGiveItems(context.Background(), req, false, "", giveItemsDeps{
		checkCapacity: func(context.Context, int64, string, int64) error { return nil },
		rmqAdd:        func(string, string, int, float64) error { return nil },
		dbGive: func(_ int64, template string, _, _ int64) (msgMutate, bool) {
			if template == "X" {
				return msgMutate{}, false
			}
			return msgMutate{err: errors.New("db failed")}, true
		},
		needsDBPath: func(string) bool { return false },
	})

	if len(given) != 0 {
		t.Fatalf("expected no successful grants, got %v", given)
	}
	if len(skipped) != 2 {
		t.Fatalf("expected two skipped items, got %+v", skipped)
	}
	if skipped[0].Template != "X" || skipped[0].Reason != "internal error" {
		t.Fatalf("unexpected first skipped entry: %+v", skipped[0])
	}
	if skipped[1].Template != "Y" || skipped[1].Reason != "db failed" {
		t.Fatalf("unexpected second skipped entry: %+v", skipped[1])
	}
}

func TestProcessGiveItems_SchematicUsesDBPath(t *testing.T) {
	t.Parallel()

	req := giveItemsRequest{
		PlayerID: 11,
		Items:    []giveItemInput{{Template: "SchematicPattern_Sword", Qty: 1, Quality: 0}},
	}

	var rmqCalled bool
	var dbCalled bool
	processGiveItems(context.Background(), req, true, "fls-abc", giveItemsDeps{
		checkCapacity: func(context.Context, int64, string, int64) error { return nil },
		rmqAdd: func(string, string, int, float64) error {
			rmqCalled = true
			return nil
		},
		dbGive: func(_ int64, _ string, _, _ int64) (msgMutate, bool) {
			dbCalled = true
			return msgMutate{ok: "done"}, true
		},
		needsDBPath: func(template string) bool { return template == "SchematicPattern_Sword" },
	})

	if rmqCalled {
		t.Fatal("schematic with quality=0 should not use RMQ path")
	}
	if !dbCalled {
		t.Fatal("schematic with quality=0 should use DB path")
	}
}

func TestProcessGiveItems_AugmentUsesDBPath(t *testing.T) {
	t.Parallel()

	req := giveItemsRequest{
		PlayerID: 11,
		Items:    []giveItemInput{{Template: "Augment_ArmorPiercing", Qty: 1, Quality: 0}},
	}

	var rmqCalled bool
	var dbCalled bool
	processGiveItems(context.Background(), req, true, "fls-abc", giveItemsDeps{
		checkCapacity: func(context.Context, int64, string, int64) error { return nil },
		rmqAdd: func(string, string, int, float64) error {
			rmqCalled = true
			return nil
		},
		dbGive: func(_ int64, _ string, _, _ int64) (msgMutate, bool) {
			dbCalled = true
			return msgMutate{ok: "done"}, true
		},
		needsDBPath: func(template string) bool { return template == "Augment_ArmorPiercing" },
	})

	if rmqCalled {
		t.Fatal("augment item with quality=0 should not use RMQ path")
	}
	if !dbCalled {
		t.Fatal("augment item with quality=0 should use DB path")
	}
}
