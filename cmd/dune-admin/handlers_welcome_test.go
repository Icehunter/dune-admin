package main

import (
	"testing"
	"time"
)

func TestBuildWelcomeRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		enabled      bool
		version      string
		scanSecs     int
		wantVersion  string
		wantInterval time.Duration
	}{
		{"defaults version + interval", true, "", 0, "v1", welcomeDefaultScanInterval},
		{"interval below floor is clamped", true, "v2", 1, "v2", welcomeDefaultScanInterval},
		{"explicit interval respected", false, "season3", 120, "season3", 120 * time.Second},
		{"min interval honored", true, "v1", 5, "v1", 5 * time.Second},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := buildWelcomeRuntime(tt.enabled, tt.version, tt.scanSecs, nil)
			if rt.enabled != tt.enabled {
				t.Fatalf("enabled: want %v, got %v", tt.enabled, rt.enabled)
			}
			if rt.version != tt.wantVersion {
				t.Fatalf("version: want %q, got %q", tt.wantVersion, rt.version)
			}
			if rt.interval != tt.wantInterval {
				t.Fatalf("interval: want %v, got %v", tt.wantInterval, rt.interval)
			}
		})
	}
}
