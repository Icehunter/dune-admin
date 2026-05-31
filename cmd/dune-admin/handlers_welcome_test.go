package main

import (
	"testing"
	"time"
)

func TestBuildWelcomeRuntime(t *testing.T) {
	t.Parallel()
	pkgs := []welcomePackage{{Version: "v1"}, {Version: "v2"}}
	tests := []struct {
		name         string
		enabled      bool
		active       string
		scanSecs     int
		packages     []welcomePackage
		wantActive   string
		wantInterval time.Duration
	}{
		{"defaults active to first package", true, "", 0, pkgs, "v1", welcomeDefaultScanInterval},
		{"unknown active falls back to first", true, "vX", 0, pkgs, "v1", welcomeDefaultScanInterval},
		{"explicit active respected", true, "v2", 120, pkgs, "v2", 120 * time.Second},
		{"interval below floor is clamped", false, "v1", 1, pkgs, "v1", welcomeDefaultScanInterval},
		{"min interval honored", true, "v2", 5, pkgs, "v2", 5 * time.Second},
		{"no packages → empty active", true, "", 60, nil, "", 60 * time.Second},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := buildWelcomeRuntime(tt.enabled, tt.active, tt.scanSecs, tt.packages)
			if rt.enabled != tt.enabled {
				t.Fatalf("enabled: want %v, got %v", tt.enabled, rt.enabled)
			}
			if rt.activeVersion != tt.wantActive {
				t.Fatalf("activeVersion: want %q, got %q", tt.wantActive, rt.activeVersion)
			}
			if rt.interval != tt.wantInterval {
				t.Fatalf("interval: want %v, got %v", tt.wantInterval, rt.interval)
			}
		})
	}
}

func TestWelcomeRuntimeActive(t *testing.T) {
	t.Parallel()
	rt := buildWelcomeRuntime(true, "v2", 30, []welcomePackage{
		{Version: "v1", Items: []welcomePackageItem{{Template: "A", Qty: 1}}},
		{Version: "v2", Items: []welcomePackageItem{{Template: "B", Qty: 2}}},
	})
	p, ok := rt.active()
	if !ok {
		t.Fatal("expected an active package")
	}
	if p.Version != "v2" || len(p.Items) != 1 || p.Items[0].Template != "B" {
		t.Fatalf("active package wrong: %+v", p)
	}

	empty := buildWelcomeRuntime(true, "", 30, nil)
	if _, ok := empty.active(); ok {
		t.Fatal("expected no active package when library is empty")
	}
}
