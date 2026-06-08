package main

import (
	"testing"
	"time"
)

func TestBackupShouldFire(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	// All-days rule at 04:00 so the test is weekday-agnostic.
	cfg := scheduledBackupConfig{
		Enabled: true,
		Rules:   []backupRule{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Time: "04:00"}},
	}
	at0400 := time.Date(2026, 6, 8, 4, 0, 0, 0, loc)

	tests := []struct {
		name      string
		now       time.Time
		enabled   bool
		lastFired int64
		wantFire  bool
	}{
		{"fires 2 min after occurrence", time.Date(2026, 6, 8, 4, 2, 0, 0, loc), true, 0, true},
		{"already fired this occurrence", time.Date(2026, 6, 8, 4, 2, 0, 0, loc), true, at0400.Unix(), false},
		{"missed beyond grace window", time.Date(2026, 6, 8, 4, 15, 0, 0, loc), true, 0, false},
		{"disabled never fires", time.Date(2026, 6, 8, 4, 2, 0, 0, loc), false, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg
			c.Enabled = tt.enabled
			c.LastFired = tt.lastFired
			at, fire := backupShouldFire(tt.now, c, loc)
			if fire != tt.wantFire {
				t.Fatalf("backupShouldFire = %v (at %v), want %v", fire, at, tt.wantFire)
			}
			if fire && !at.Equal(at0400) {
				t.Fatalf("fire target = %v, want %v", at, at0400)
			}
		})
	}

	// No rules → never fires even when enabled.
	if _, fire := backupShouldFire(time.Date(2026, 6, 8, 4, 2, 0, 0, loc),
		scheduledBackupConfig{Enabled: true}, loc); fire {
		t.Fatalf("empty rules should not fire")
	}
}
