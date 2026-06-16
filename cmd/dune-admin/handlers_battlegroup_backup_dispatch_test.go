package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingControl embeds stubControlPlane and records ExecCommand calls. It does
// NOT implement dbBackupProvider, so it stands in for kubectl/docker/local.
type recordingControl struct {
	stubControlPlane
	name     string
	execCmds []string
	execOut  string
}

func (r *recordingControl) Name() string {
	if r.name != "" {
		return r.name
	}
	return "kubectl"
}

func (r *recordingControl) ExecCommand(_ context.Context, _ Executor, cmd string) (string, error) {
	r.execCmds = append(r.execCmds, cmd)
	return r.execOut, nil
}

func (r *recordingControl) GetStatus(_ context.Context, _ Executor) (*BattlegroupStatus, error) {
	return &BattlegroupStatus{}, nil
}

// dbProviderControl embeds recordingControl and additionally implements
// dbBackupProvider, standing in for the AMP control plane.
type dbProviderControl struct {
	recordingControl
	backupCalled  bool
	restoreCalled bool
	restoreSrc    string
	backupErr     error
	restoreErr    error
}

func (d *dbProviderControl) Name() string { return "amp" }

func (d *dbProviderControl) BackupDatabase(_ Executor, _ dbConn, destPath string) (string, error) {
	d.backupCalled = true
	if d.backupErr != nil {
		return "", d.backupErr
	}
	// Write a valid pg_dump custom-format archive (magic "PGDMP") so verifyDumpFile passes.
	if err := os.WriteFile(destPath, []byte("PGDMPxxxx"), 0o600); err != nil {
		return "", err
	}
	return "ok", nil
}

func (d *dbProviderControl) RestoreDatabase(_ Executor, _ dbConn, srcPath string) (string, error) {
	d.restoreCalled = true
	d.restoreSrc = srcPath
	if d.restoreErr != nil {
		return "", d.restoreErr
	}
	return "restored", nil
}

// saveBackupGlobals snapshots and restores the globals + config the dispatch
// helpers touch, and points AmpBackupDir at a temp dir.
func saveBackupGlobals(t *testing.T) string {
	t.Helper()
	prevC, prevE := globalControl, globalExecutor
	prevCfg := loadedConfig
	prevReg := globalRegistry
	dir := t.TempDir()
	loadedConfig.AmpBackupDir = dir
	globalExecutor = &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	// The DB-backup dir/conn resolve from the active server's ServerConfig after
	// the storage remodel, so register one pointing at the temp dir.
	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "1", Name: "T", Cfg: ServerConfig{ID: 1, AmpBackupDir: dir}})
	_ = globalRegistry.SetActive("1")
	t.Cleanup(func() {
		globalControl, globalExecutor = prevC, prevE
		loadedConfig = prevCfg
		globalRegistry = prevReg
	})
	return dir
}

// TestDispatchBackup_AMPUsesDBProvider verifies that when the control plane
// implements dbBackupProvider, a battlegroup backup is taken via BackupDatabase
// (pg_dump) and NOT routed through ExecCommand("backup").
func TestDispatchBackup_AMPUsesDBProvider(t *testing.T) {
	dir := saveBackupGlobals(t)
	ctrl := &dbProviderControl{}
	globalControl = ctrl

	out, err := dispatchBackup(context.Background(), globalControl, globalExecutor)
	if err != nil {
		t.Fatalf("dispatchBackup: %v", err)
	}
	if !ctrl.backupCalled {
		t.Fatal("expected BackupDatabase to be called under AMP")
	}
	if len(ctrl.execCmds) != 0 {
		t.Fatalf("expected no ExecCommand calls, got %v", ctrl.execCmds)
	}
	// A .dump file should now exist in the backup dir.
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".dump") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a .dump file to be written")
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

// TestDispatchBackup_NonProviderUsesExecCommand verifies that for control planes
// without dbBackupProvider (kubectl/docker/local), backup routes through
// ExecCommand("backup").
func TestDispatchBackup_NonProviderUsesExecCommand(t *testing.T) {
	saveBackupGlobals(t)
	ctrl := &recordingControl{execOut: "backup done"}
	globalControl = ctrl

	out, err := dispatchBackup(context.Background(), globalControl, globalExecutor)
	if err != nil {
		t.Fatalf("dispatchBackup: %v", err)
	}
	if len(ctrl.execCmds) != 1 || ctrl.execCmds[0] != "backup" {
		t.Fatalf("expected ExecCommand(\"backup\"), got %v", ctrl.execCmds)
	}
	if out != "backup done" {
		t.Fatalf("output = %q, want %q", out, "backup done")
	}
}

// TestDispatchRestore_AMPUsesDBProvider verifies that an AMP restore validates a
// .dump name and routes through RestoreDatabase (pg_restore), not battlegroup.sh.
func TestDispatchRestore_AMPUsesDBProvider(t *testing.T) {
	dir := saveBackupGlobals(t)
	name := "dune-20260101-000000.dump"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("PGDMPxxxx"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctrl := &dbProviderControl{}
	globalControl = ctrl

	out, err := dispatchRestore(context.Background(), globalControl, globalExecutor, name)
	if err != nil {
		t.Fatalf("dispatchRestore: %v", err)
	}
	if !ctrl.restoreCalled {
		t.Fatal("expected RestoreDatabase to be called under AMP")
	}
	if ctrl.restoreSrc != filepath.Join(dir, name) {
		t.Fatalf("restore src = %q, want %q", ctrl.restoreSrc, filepath.Join(dir, name))
	}
	if len(ctrl.execCmds) != 0 {
		t.Fatalf("expected no ExecCommand calls, got %v", ctrl.execCmds)
	}
	if out != "restored" {
		t.Fatalf("output = %q, want %q", out, "restored")
	}
}

// TestDispatchRestore_AMPRejectsNonDump verifies AMP restore rejects a .backup
// filename (the db-backup store uses .dump).
func TestDispatchRestore_AMPRejectsNonDump(t *testing.T) {
	saveBackupGlobals(t)
	globalControl = &dbProviderControl{}

	if _, err := dispatchRestore(context.Background(), globalControl, globalExecutor, "snapshot.backup"); err == nil {
		t.Fatal("expected error restoring a .backup name under AMP")
	}
}

// TestDispatchRestore_NonProviderUsesControlScript verifies that for kubectl the
// restore goes through restoreViaControl (battlegroup.sh import), validating a
// .backup name.
func TestDispatchRestore_NonProviderUsesControlScript(t *testing.T) {
	saveBackupGlobals(t)
	ctrl := &recordingControl{name: "kubectl"}
	globalControl = ctrl
	// restoreViaControl shells out via globalExecutor for kubectl; capture the cmd.
	var got string
	globalExecutor = &fnExecutor{fn: func(cmd string) (string, error) {
		got = cmd
		return "imported", nil
	}}

	out, err := dispatchRestore(context.Background(), globalControl, globalExecutor, "snapshot.backup")
	if err != nil {
		t.Fatalf("dispatchRestore: %v", err)
	}
	if !strings.Contains(got, "battlegroup.sh") || !strings.Contains(got, "import") || !strings.Contains(got, "snapshot.backup") {
		t.Fatalf("expected battlegroup.sh import command, got %q", got)
	}
	if out != "imported" {
		t.Fatalf("output = %q, want %q", out, "imported")
	}
}

// TestDispatchRestore_NonProviderRejectsNonBackup verifies kubectl restore
// rejects a name that doesn't end in .backup.
func TestDispatchRestore_NonProviderRejectsNonBackup(t *testing.T) {
	saveBackupGlobals(t)
	globalControl = &recordingControl{name: "kubectl"}

	if _, err := dispatchRestore(context.Background(), globalControl, globalExecutor, "dune-x.dump"); err == nil {
		t.Fatal("expected error restoring a .dump name under kubectl")
	}
}

// TestHandleBGStatus_CtxControlOverridesGlobal verifies that a control plane
// stashed in the request context is used instead of globalControl.
func TestHandleBGStatus_CtxControlOverridesGlobal(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })

	ctrl := &recordingControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Control: ctrl, Executor: exec}
	reg.Register(sc)

	inner := http.HandlerFunc(handleBGStatus)
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlegroup/status", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusServiceUnavailable {
		t.Error("ctx control should prevent 503 when globalControl is nil")
	}
}

// TestBGCmdAllowlist_NoBackup verifies "backup" has been removed from the
// battlegroup exec allowlist (backup now dispatches via dispatchBackup).
func TestBGCmdAllowlist_NoBackup(t *testing.T) {
	if bgCmdAllowlist["backup"] {
		t.Fatal(`"backup" must no longer be in bgCmdAllowlist`)
	}
	for _, c := range []string{"start", "stop", "restart", "update"} {
		if !bgCmdAllowlist[c] {
			t.Errorf("%q missing from bgCmdAllowlist", c)
		}
	}
}
