// cmd/dune-admin/handlers_update_test.go
package main

import (
	"testing"
)

func TestArtifactName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "dune-admin_linux_amd64.tar.gz"},
		{"linux", "arm64", "dune-admin_linux_arm64.tar.gz"},
		{"darwin", "amd64", "dune-admin_darwin_universal.tar.gz"},
		{"darwin", "arm64", "dune-admin_darwin_universal.tar.gz"},
		{"windows", "amd64", "dune-admin_windows_amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"_"+tt.goarch, func(t *testing.T) {
			got := artifactName(tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("artifactName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	content := "abc123  dune-admin_linux_amd64.tar.gz\ndef456  dune-admin_darwin_universal.tar.gz\n"
	tests := []struct {
		artifact string
		want     string
		wantErr  bool
	}{
		{"dune-admin_linux_amd64.tar.gz", "abc123", false},
		{"dune-admin_darwin_universal.tar.gz", "def456", false},
		{"dune-admin_windows_amd64.zip", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.artifact, func(t *testing.T) {
			got, err := parseChecksum(content, tt.artifact)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseChecksum error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseChecksum = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.15.2", "0.15.2", false},
		{"0.15.2", "0.16.0", true},
		{"0.15.2-dev", "0.15.2", false},
		{"0.15.2-dev", "0.16.0", false},
		{"dev", "0.16.0", false},
		{"0.15.2", "", false}, // no release tag: treat as no update available
	}
	for _, tt := range tests {
		t.Run(tt.current+"->"+tt.latest, func(t *testing.T) {
			got := needsUpdate(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("needsUpdate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
