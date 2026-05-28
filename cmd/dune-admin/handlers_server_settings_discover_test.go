package main

import (
	"path/filepath"
	"testing"
)

func TestConfiguredDefaultINIPath(t *testing.T) {
	originalConfig := loadedConfig
	t.Cleanup(func() { loadedConfig = originalConfig })

	loadedConfig.DefaultIniDir = "/opt/dune/config"
	if got := configuredDefaultINIPath("DefaultGame.ini"); got != "/opt/dune/config/DefaultGame.ini" {
		t.Fatalf("unexpected configured path: %q", got)
	}

	loadedConfig.DefaultIniDir = ""
	if got := configuredDefaultINIPath("DefaultGame.ini"); got != "" {
		t.Fatalf("expected empty configured path when default ini dir unset, got %q", got)
	}
}

func TestK8sDerivedDefaultINICandidates(t *testing.T) {
	t.Parallel()

	got := k8sDerivedDefaultINICandidates("/home/dune/server/state", "DefaultEngine.ini")
	if len(got) < 7 {
		t.Fatalf("expected at least 7 candidates, got %d", len(got))
	}
	if got[0] != "/home/Config/DefaultEngine.ini" {
		t.Fatalf("unexpected first candidate: %q", got[0])
	}
	if got[len(got)-1] != "/game/DuneSandbox/Config/DefaultEngine.ini" {
		t.Fatalf("unexpected last candidate: %q", got[len(got)-1])
	}
}

func TestHostDefaultINICandidates(t *testing.T) {
	t.Parallel()

	got := hostDefaultINICandidates("DefaultGame.ini")
	want := []string{
		"/home/dune/DefaultGame.ini",
		"/home/DefaultGame.ini",
		"/root/DefaultGame.ini",
		"/dune/DefaultGame.ini",
		"/home/dune/server/DuneSandbox/Config/DefaultGame.ini",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d host candidates, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d mismatch: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestRelativeDefaultINICandidates(t *testing.T) {
	t.Parallel()

	base := filepath.Clean("/srv/dune/server/state")
	got := relativeDefaultINICandidates(base, "DefaultEngine.ini")
	want := []string{
		filepath.Join(base, "..", "..", "..", "Config", "DefaultEngine.ini"),
		filepath.Join(base, "..", "..", "Config", "DefaultEngine.ini"),
		filepath.Join(base, "..", "..", "..", "..", "Config", "DefaultEngine.ini"),
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d relative candidates, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d mismatch: want %q got %q", i, want[i], got[i])
		}
	}
}
