package main

import (
	"testing"
	"time"
)

// 2026-06-10 12:00 UTC is a Wednesday (weekday 3).
var schedNow = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

const (
	wed = 3
	thu = 4
)

func TestParseHHMM(t *testing.T) {
	t.Parallel()
	ok := map[string][2]int{"04:00": {4, 0}, "23:59": {23, 59}, "0:0": {0, 0}, " 14:30 ": {14, 30}}
	for s, want := range ok {
		h, m, valid := parseHHMM(s)
		if !valid || h != want[0] || m != want[1] {
			t.Errorf("parseHHMM(%q) = %d,%d,%v; want %d,%d,true", s, h, m, valid, want[0], want[1])
		}
	}
	for _, bad := range []string{"24:00", "12:60", "-1:00", "abc", "1200", ""} {
		if _, _, valid := parseHHMM(bad); valid {
			t.Errorf("parseHHMM(%q) should be invalid", bad)
		}
	}
}

func TestNextRestartAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		rules []restartRule
		want  time.Time
		ok    bool
	}{
		{"today later", []restartRule{{Days: []int{wed}, Time: "14:00"}}, time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC), true},
		{"today passed -> next week", []restartRule{{Days: []int{wed}, Time: "10:00"}}, time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC), true},
		{"tomorrow", []restartRule{{Days: []int{thu}, Time: "03:00"}}, time.Date(2026, 6, 11, 3, 0, 0, 0, time.UTC), true},
		{"earliest of many", []restartRule{{Days: []int{wed}, Time: "14:00"}, {Days: []int{thu}, Time: "03:00"}}, time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC), true},
		{"none", nil, time.Time{}, false},
		{"invalid time ignored", []restartRule{{Days: []int{wed}, Time: "bad"}}, time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nextRestartAt(schedNow, tt.rules, time.UTC)
			if ok != tt.ok || (ok && !got.Equal(tt.want)) {
				t.Fatalf("nextRestartAt = %v,%v; want %v,%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPrevRestartAt(t *testing.T) {
	t.Parallel()
	// Today earlier today -> today; today-not-yet -> last week.
	if got, ok := prevRestartAt(schedNow, []restartRule{{Days: []int{wed}, Time: "11:00"}}, time.UTC); !ok || !got.Equal(time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("prev (today 11:00) = %v,%v", got, ok)
	}
	if got, ok := prevRestartAt(schedNow, []restartRule{{Days: []int{wed}, Time: "14:00"}}, time.UTC); !ok || !got.Equal(time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("prev (today 14:00 not yet -> last week) = %v,%v", got, ok)
	}
}

func TestRestartDecision(t *testing.T) {
	t.Parallel()
	base := func(rules []restartRule) scheduledRestartConfig {
		return scheduledRestartConfig{Enabled: true, Rules: rules, WarnMinutes: 10}
	}

	if act, _ := restartDecision(schedNow, scheduledRestartConfig{Enabled: false, Rules: []restartRule{{Days: []int{wed}, Time: "11:59"}}}, time.UTC, time.Time{}); act != restartNone {
		t.Errorf("disabled should be none, got %v", act)
	}

	// Restart 1 min ago, never fired -> fire.
	if act, target := restartDecision(schedNow, base([]restartRule{{Days: []int{wed}, Time: "11:59"}}), time.UTC, time.Time{}); act != restartFire || !target.Equal(time.Date(2026, 6, 10, 11, 59, 0, 0, time.UTC)) {
		t.Errorf("due restart should fire, got %v %v", act, target)
	}

	// Upcoming in 5 min (within 10-min lead), not warned -> warn.
	if act, target := restartDecision(schedNow, base([]restartRule{{Days: []int{wed}, Time: "12:05"}}), time.UTC, time.Time{}); act != restartWarn || !target.Equal(time.Date(2026, 6, 10, 12, 5, 0, 0, time.UTC)) {
		t.Errorf("upcoming within lead should warn, got %v %v", act, target)
	}

	// Same, but already warned for that target -> none.
	warned := time.Date(2026, 6, 10, 12, 5, 0, 0, time.UTC)
	if act, _ := restartDecision(schedNow, base([]restartRule{{Days: []int{wed}, Time: "12:05"}}), time.UTC, warned); act != restartNone {
		t.Errorf("already-warned should be none, got %v", act)
	}

	// Upcoming far away (8h) -> none.
	if act, _ := restartDecision(schedNow, base([]restartRule{{Days: []int{wed}, Time: "20:00"}}), time.UTC, time.Time{}); act != restartNone {
		t.Errorf("far-future should be none, got %v", act)
	}

	// Upcoming within lead but watermark covers it (skipped) -> none.
	cfg := base([]restartRule{{Days: []int{wed}, Time: "12:05"}})
	cfg.LastFired = time.Date(2026, 6, 10, 12, 5, 0, 0, time.UTC).Unix()
	if act, _ := restartDecision(schedNow, cfg, time.UTC, time.Time{}); act != restartNone {
		t.Errorf("skipped (watermark) should be none, got %v", act)
	}
}
