package main

import "testing"

func TestRepairItemNoChangeMessage(t *testing.T) {
	t.Parallel()

	msg := repairItemNoChangeMessage(42, false)
	if msg.err == nil || msg.err.Error() != "item 42 has no durability field" {
		t.Fatalf("expected no-durability error, got %+v", msg)
	}

	msg = repairItemNoChangeMessage(42, true)
	if msg.err != nil {
		t.Fatalf("expected success message, got error %v", msg.err)
	}
	if msg.ok != "Item 42 already at full durability" {
		t.Fatalf("unexpected success message: %q", msg.ok)
	}
}

func TestRepairItemSuccessMessage(t *testing.T) {
	t.Parallel()

	msg := repairItemSuccessMessage(99)
	if msg.err != nil {
		t.Fatalf("expected nil error, got %v", msg.err)
	}
	if msg.ok != "Repaired item 99 — relog to see in-game" {
		t.Fatalf("unexpected success message: %q", msg.ok)
	}
}
