package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run() must translate the immediate-mode outcomes that main() previously
// handled with os.Exit / a bare return into ordinary error returns, so that any
// deferred cleanup registered later in run() is guaranteed to unwind. These
// tests exercise the render-k8s immediate mode because it has no DB or network
// dependency: it only reads package globals and writes a manifest file.

// TestRun_ImmediateModeHandledReturnsNil locks in the mapping
// (handled, nil) -> return nil: a successfully handled immediate mode makes
// run() return nil without proceeding to loadRuntimeData/server startup.
func TestRun_ImmediateModeHandledReturnsNil(t *testing.T) {
	prev := renderK8SOut
	t.Cleanup(func() { renderK8SOut = prev })

	renderK8SOut = filepath.Join(t.TempDir(), "manifest.yaml")
	if err := run(context.Background()); err != nil {
		t.Fatalf("run() returned error for a successful immediate mode: %v", err)
	}
	if _, err := os.Stat(renderK8SOut); err != nil {
		t.Fatalf("expected manifest written at %s: %v", renderK8SOut, err)
	}
}

// TestRun_VersionModeHandled locks in that -version is an immediate mode: it
// prints the version to stdout and returns (handled, nil) so run() exits without
// starting the server / touching the DB.
func TestRun_VersionModeHandled(t *testing.T) {
	prev := versionMode
	t.Cleanup(func() { versionMode = prev })
	versionMode = true

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	handled, err := runImmediateModes()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if !handled || err != nil {
		t.Fatalf("version mode: handled=%v err=%v, want true, nil", handled, err)
	}
	if !strings.Contains(string(out), AppVersion) {
		t.Fatalf("version output %q does not contain AppVersion %q", out, AppVersion)
	}
}

// TestRun_ImmediateModeErrorPropagates locks in the mapping
// (handled, err) -> return err (previously os.Exit(1)): a failing immediate mode
// is surfaced as an error carrying its label prefix, never terminating the
// process before deferred cleanup can run.
func TestRun_ImmediateModeErrorPropagates(t *testing.T) {
	prev := renderK8SOut
	t.Cleanup(func() { renderK8SOut = prev })

	// A regular file used as a parent directory component forces MkdirAll
	// (and thus renderK8SManifest) to fail deterministically — no DB or network.
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	renderK8SOut = filepath.Join(notADir, "sub", "manifest.yaml")

	err := run(context.Background())
	if err == nil {
		t.Fatal("run() returned nil for a failing immediate mode; want error")
	}
	if !strings.Contains(err.Error(), "render-k8s: ") {
		t.Errorf("error %q missing immediate-mode label prefix %q", err, "render-k8s: ")
	}
}
