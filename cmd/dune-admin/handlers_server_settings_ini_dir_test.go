package main

import (
	"context"
	"errors"
	"testing"
)

// stubControlPlane is a minimal ControlPlane that only implements DiscoverIniDir.
// All other methods return "not implemented" errors.
type stubControlPlane struct {
	iniDir string
	iniErr error
}

func (s *stubControlPlane) Name() string { return "stub" }
func (s *stubControlPlane) GetStatus(_ context.Context, _ Executor) (*BattlegroupStatus, error) {
	return nil, errors.New("not implemented")
}
func (s *stubControlPlane) ExecCommand(_ context.Context, _ Executor, _ string) (string, error) {
	return "", errors.New("not implemented")
}
func (s *stubControlPlane) ListProcesses(_ context.Context, _ Executor) ([]ProcessInfo, string, error) {
	return nil, "", errors.New("not implemented")
}
func (s *stubControlPlane) ListLogSources(_ context.Context, _ Executor) ([]LogSource, error) {
	return nil, errors.New("not implemented")
}
func (s *stubControlPlane) StreamLog(_ context.Context, _ Executor, _, _ string) (<-chan string, func(), error) {
	return nil, nil, errors.New("not implemented")
}
func (s *stubControlPlane) CaptureJWT(_ context.Context, _ Executor) (string, string, error) {
	return "", "", errors.New("not implemented")
}
func (s *stubControlPlane) EvalOnGameBroker(_ context.Context, _ Executor, _ string) (string, error) {
	return "", errors.New("not implemented")
}
func (s *stubControlPlane) DiscoverIniDir(_ context.Context, _ Executor) (string, error) {
	return s.iniDir, s.iniErr
}
func (s *stubControlPlane) ReadDefaultINI(_ context.Context, _ Executor, _ string) string {
	return ""
}

func saveIniDirGlobals(t *testing.T) {
	t.Helper()
	origControl := globalControl
	origConfig := loadedConfig
	origServerIniDir := serverIniDir
	t.Cleanup(func() {
		globalControl = origControl
		loadedConfig = origConfig
		serverIniDir = origServerIniDir
	})
}

// TestIniDir_PrefersControlPlaneOverConfigured verifies that when the control
// plane successfully returns a path, iniDir() uses it even if server_ini_dir is
// set in config. This ensures amp's ue5-saved/UserSettings probe always runs.
func TestIniDir_PrefersControlPlaneOverConfigured(t *testing.T) {
	saveIniDirGlobals(t)
	setGlobalExecutor(t, func(_ string) (string, error) { return "", nil })

	globalControl = &stubControlPlane{iniDir: "/discovered/ue5-saved/UserSettings"}
	loadedConfig.ServerIniDir = "/configured/state"

	dir, err := iniDir(globalControl, globalExecutor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/discovered/ue5-saved/UserSettings" {
		t.Errorf("got %q, want /discovered/ue5-saved/UserSettings", dir)
	}
}

// TestIniDir_FallsBackToConfiguredWhenControlPlaneFails verifies that when the
// control plane errors (e.g. docker with no server_ini_dir stored), iniDir()
// falls back to the configured server_ini_dir value.
func TestIniDir_FallsBackToConfiguredWhenControlPlaneFails(t *testing.T) {
	saveIniDirGlobals(t)
	setGlobalExecutor(t, func(_ string) (string, error) { return "", nil })

	globalControl = &stubControlPlane{iniErr: errors.New("control plane cannot discover ini dir")}
	loadedConfig.ServerIniDir = "/configured/state"

	dir, err := iniDir(globalControl, globalExecutor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/configured/state" {
		t.Errorf("got %q, want /configured/state", dir)
	}
}

// TestIniDir_UsesConfiguredWhenNoControlPlane verifies that when no control
// plane is connected, iniDir() returns the configured server_ini_dir.
func TestIniDir_UsesConfiguredWhenNoControlPlane(t *testing.T) {
	saveIniDirGlobals(t)
	origExecutor := globalExecutor
	t.Cleanup(func() { globalExecutor = origExecutor })

	globalControl = nil
	globalExecutor = nil
	loadedConfig.ServerIniDir = "/configured/state"

	dir, err := iniDir(globalControl, globalExecutor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/configured/state" {
		t.Errorf("got %q, want /configured/state", dir)
	}
}

// TestIniDir_ErrorsWhenNothingConfigured verifies that iniDir() returns an
// error when no control plane is available and no server_ini_dir is configured.
func TestIniDir_ErrorsWhenNothingConfigured(t *testing.T) {
	saveIniDirGlobals(t)
	origExecutor := globalExecutor
	t.Cleanup(func() { globalExecutor = origExecutor })

	globalControl = nil
	globalExecutor = nil
	loadedConfig.ServerIniDir = ""
	serverIniDir = ""

	_, err := iniDir(globalControl, globalExecutor)
	if err == nil {
		t.Fatal("expected error when nothing is configured, got nil")
	}
}
