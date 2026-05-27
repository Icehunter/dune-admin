package main

import "testing"

func TestAllKeystoneIDs(t *testing.T) {
	t.Parallel()

	ids := allKeystoneIDs()
	if len(ids) != 205 {
		t.Fatalf("expected 205 keystone ids, got %d", len(ids))
	}
	if ids[0] != 1 || ids[len(ids)-1] != 205 {
		t.Fatalf("unexpected ID bounds: first=%d last=%d", ids[0], ids[len(ids)-1])
	}
	for i, id := range ids {
		if int(id) != i+1 {
			t.Fatalf("unexpected id sequence at index %d: got %d", i, id)
		}
	}
}

func TestGrantAllKeystoneTargets(t *testing.T) {
	t.Parallel()

	bonus := keystoneSPBonus(allKeystoneIDs())
	expectedTotal, expectedUnspent, gotBonus := grantAllKeystoneTargets(0, 0)
	if gotBonus != bonus {
		t.Fatalf("expected bonus %d, got %d", bonus, gotBonus)
	}
	if expectedTotal != bonus {
		t.Fatalf("expected total skill points %d at xp=0, got %d", bonus, expectedTotal)
	}
	if expectedUnspent != expectedTotal-1 {
		t.Fatalf("expected unspent=%d, got %d", expectedTotal-1, expectedUnspent)
	}

	_, expectedUnspent, _ = grantAllKeystoneTargets(0, 99999)
	if expectedUnspent != 0 {
		t.Fatalf("expected negative unspent to clamp to 0, got %d", expectedUnspent)
	}
}
