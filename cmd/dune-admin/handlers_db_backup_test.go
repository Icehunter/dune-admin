package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// restoreFlowControl stands in for a dbBackupProvider control plane (AMP)
// whose GetStatus can report running game shards; it records the call order
// so tests can assert stop-before-restore. It deliberately does NOT implement
// gameServerStopper — see stopperFlowControl.
type restoreFlowControl struct {
	stubControlPlane
	servers    int
	restoreOut string
	restoreErr error
	order      []string
}

func (c *restoreFlowControl) Name() string { return "amp" }

func (c *restoreFlowControl) GetStatus(_ context.Context, _ Executor) (*BattlegroupStatus, error) {
	return &BattlegroupStatus{Servers: make([]ServerRow, c.servers)}, nil
}

func (c *restoreFlowControl) BackupDatabase(_ Executor, _ dbConn, _ string) (string, error) {
	return "", nil
}

func (c *restoreFlowControl) RestoreDatabase(_ Executor, _ dbConn, _ string) (string, error) {
	c.order = append(c.order, "restore")
	return c.restoreOut, c.restoreErr
}

// stopperFlowControl additionally implements gameServerStopper: a successful
// stop clears the reported shard count, mirroring the real AMP behaviour.
type stopperFlowControl struct {
	restoreFlowControl
	stopErr error
}

func (c *stopperFlowControl) StopGameServers(_ context.Context, _ Executor) error {
	c.order = append(c.order, "stop")
	if c.stopErr != nil {
		return c.stopErr
	}
	c.servers = 0
	return nil
}

// writeRestoreDump writes a named dump file into the temp backup dir set up by
// saveBackupGlobals so restoreDBBackupFile's stat check passes.
func writeRestoreDump(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("PGDMPxxxx"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareAndRestoreDB(t *testing.T) {
	const name = "dune-20260101-000000.dump"

	t.Run("not running: restores without stopping", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{servers: 0, restoreOut: "restored"}
		res, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ServersStopped {
			t.Fatal("ServersStopped = true, want false")
		}
		if len(ctrl.order) != 1 || ctrl.order[0] != "restore" {
			t.Fatalf("call order = %v, want [restore]", ctrl.order)
		}
	})

	t.Run("running + stopper: stops shards, then restores", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &stopperFlowControl{restoreFlowControl: restoreFlowControl{servers: 5, restoreOut: "restored"}}
		res, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.ServersStopped {
			t.Fatal("ServersStopped = false, want true")
		}
		want := []string{"stop", "restore"}
		if len(ctrl.order) != 2 || ctrl.order[0] != want[0] || ctrl.order[1] != want[1] {
			t.Fatalf("call order = %v, want %v", ctrl.order, want)
		}
	})

	t.Run("running + stopper failure: restore never runs", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		boom := errors.New("shards would not die")
		ctrl := &stopperFlowControl{restoreFlowControl: restoreFlowControl{servers: 5}, stopErr: boom}
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if !errors.Is(err, boom) {
			t.Fatalf("want stop error, got %v", err)
		}
		for _, step := range ctrl.order {
			if step == "restore" {
				t.Fatal("restore must not run when stopping fails")
			}
		}
	})

	t.Run("running + no stopper: errBattlegroupRunning", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{servers: 3}
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if !errors.Is(err, errBattlegroupRunning) {
			t.Fatalf("want errBattlegroupRunning, got %v", err)
		}
		if len(ctrl.order) != 0 {
			t.Fatalf("nothing should have run, got %v", ctrl.order)
		}
	})

	t.Run("pg_restore exit 1 with completion summary is success with count", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{
			restoreOut: "pg_restore: warning: errors ignored on restore: 38",
			restoreErr: errors.New("exit status 1"),
		}
		res, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IgnoredErrors != 38 {
			t.Fatalf("IgnoredErrors = %d, want 38", res.IgnoredErrors)
		}
	})

	t.Run("report callback emits the full step sequence", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &stopperFlowControl{restoreFlowControl: restoreFlowControl{servers: 5, restoreOut: "restored"}}
		var emits []string
		report := func(step, status string) { emits = append(emits, step+":"+status) }
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, report)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{
			"check:running", "check:done",
			"stop:running", "stop:done",
			"restore:running", "restore:done",
			"finalize:running", "finalize:done",
		}
		if len(emits) != len(want) {
			t.Fatalf("emits = %v, want %v", emits, want)
		}
		for i := range want {
			if emits[i] != want[i] {
				t.Fatalf("emits = %v, want %v", emits, want)
			}
		}
	})

	t.Run("report callback marks stop skipped when nothing is running", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{servers: 0, restoreOut: "restored"}
		var emits []string
		report := func(step, status string) { emits = append(emits, step+":"+status) }
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, report)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, e := range emits {
			if e == "stop:skipped" {
				found = true
			}
			if e == "stop:running" || e == "stop:done" {
				t.Fatalf("stop must be skipped when nothing is running, emits = %v", emits)
			}
		}
		if !found {
			t.Fatalf("expected stop:skipped in %v", emits)
		}
	})

	t.Run("report callback marks the failing step failed", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{
			restoreOut: "pg_restore: error: connection to server failed",
			restoreErr: errors.New("exit status 1"),
		}
		var emits []string
		report := func(step, status string) { emits = append(emits, step+":"+status) }
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, report)
		if err == nil {
			t.Fatal("expected an error")
		}
		if emits[len(emits)-1] != "restore:failed" {
			t.Fatalf("last emit = %q, want restore:failed (emits %v)", emits[len(emits)-1], emits)
		}
	})

	t.Run("report callback marks stop failed for a running battlegroup without a stopper", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{servers: 3}
		var emits []string
		report := func(step, status string) { emits = append(emits, step+":"+status) }
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, report)
		if !errors.Is(err, errBattlegroupRunning) {
			t.Fatalf("want errBattlegroupRunning, got %v", err)
		}
		if emits[len(emits)-1] != "stop:failed" {
			t.Fatalf("last emit = %q, want stop:failed (emits %v)", emits[len(emits)-1], emits)
		}
	})

	t.Run("real pg_restore failure surfaces the output tail", func(t *testing.T) {
		dir := saveBackupGlobals(t)
		writeRestoreDump(t, dir, name)
		ctrl := &restoreFlowControl{
			restoreOut: "pg_restore: error: connection to server failed",
			restoreErr: errors.New("exit status 1"),
		}
		_, err := prepareAndRestoreDB(context.Background(), ctrl, globalExecutor, nil, "", name, nil)
		if err == nil || !strings.Contains(err.Error(), "connection to server failed") {
			t.Fatalf("error should carry pg_restore output, got %v", err)
		}
	})
}

// TestHandleDBBackupRestore_RequiresConfirm verifies the destructive restore
// endpoint rejects a request without confirm=true before doing anything else.
func TestHandleDBBackupRestore_RequiresConfirm(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups/restore",
		strings.NewReader(`{"file":"dune-x.dump","confirm":false}`))
	handleDBBackupRestore(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("restore without confirm: code = %d, want 400", rec.Code)
	}
}

// TestHandleDBBackupRestore_StartsJobAndReportsStatus drives the async flow
// end to end at the handler layer: POST starts the job (202), the status
// endpoint reflects the terminal state with all steps resolved.
func TestHandleDBBackupRestore_StartsJobAndReportsStatus(t *testing.T) {
	dir := saveBackupGlobals(t)
	const name = "dune-20260101-000000.dump"
	writeRestoreDump(t, dir, name)
	prevJobs := globalRestoreJobs
	globalRestoreJobs = newDBRestoreJobs()
	t.Cleanup(func() { globalRestoreJobs = prevJobs })
	prevCtrl := globalControl
	globalControl = &restoreFlowControl{servers: 0, restoreOut: "pg_restore: warning: errors ignored on restore: 5"}
	t.Cleanup(func() { globalControl = prevCtrl })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups/restore",
		strings.NewReader(`{"file":"`+name+`","confirm":true}`))
	handleDBBackupRestore(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: code = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}

	globalRestoreJobs.wait("")

	rec = httptest.NewRecorder()
	handleDBRestoreStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/db-backups/restore/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: code = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"done":true`, `"failed":false`, `"ignored_errors":5`, `"key":"restore","status":"done"`, `"key":"stop","status":"skipped"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("status body missing %q:\n%s", want, body)
		}
	}
}

// TestHandleDBBackupRestore_ConflictWhileRunning verifies a second start on
// the same scope 409s while a job is in flight.
func TestHandleDBBackupRestore_ConflictWhileRunning(t *testing.T) {
	dir := saveBackupGlobals(t)
	const name = "dune-20260101-000000.dump"
	writeRestoreDump(t, dir, name)
	prevJobs := globalRestoreJobs
	globalRestoreJobs = newDBRestoreJobs()
	t.Cleanup(func() { globalRestoreJobs = prevJobs })
	prevCtrl := globalControl
	globalControl = &restoreFlowControl{}
	t.Cleanup(func() { globalControl = prevCtrl })

	release := make(chan struct{})
	if err := globalRestoreJobs.Start("", name, func(func(string, string)) (dbRestoreResult, error) {
		<-release
		return dbRestoreResult{}, nil
	}); err != nil {
		t.Fatalf("seed running job: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups/restore",
		strings.NewReader(`{"file":"`+name+`","confirm":true}`))
	handleDBBackupRestore(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second start: code = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
	close(release)
	globalRestoreJobs.wait("")
}

// TestHandleDBRestoreStatus_IdleShape verifies the idle status is a full
// all-pending step list, not an empty object.
func TestHandleDBRestoreStatus_IdleShape(t *testing.T) {
	prevJobs := globalRestoreJobs
	globalRestoreJobs = newDBRestoreJobs()
	t.Cleanup(func() { globalRestoreJobs = prevJobs })

	rec := httptest.NewRecorder()
	handleDBRestoreStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/db-backups/restore/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"running":false`, `"key":"check","status":"pending"`, `"key":"finalize","status":"pending"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("idle body missing %q:\n%s", want, body)
		}
	}
}

// TestHandleDBBackupCreate_NoControl verifies a 503 when no control plane is
// connected (globals nil).
func TestHandleDBBackupCreate_NoControl(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })
	globalControl, globalExecutor = nil, nil

	rec := httptest.NewRecorder()
	handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/db-backups", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("create with no control: code = %d, want 503", rec.Code)
	}
}

// TestHandleDBBackupDownload_BadName verifies path-traversal / bad names are rejected.
func TestHandleDBBackupDownload_BadName(t *testing.T) {
	rec := httptest.NewRecorder()
	handleDBBackupDownload(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/db-backups/download?file=../../etc/passwd", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("download traversal: code = %d, want 400", rec.Code)
	}
}

// TestDbBackupProviderOrErr_CtxControlOverridesGlobal verifies that a
// ServerContext stashed in the request context satisfies the control guard even
// when globalControl is nil.
func TestDbBackupProviderOrErr_CtxControlOverridesGlobal(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })

	ctrl := &dbProviderControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Control: ctrl, Executor: exec}
	reg.Register(sc)

	var capturedProv dbBackupProvider
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prov, ok := dbBackupProviderOrErr(w, r)
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		capturedProv = prov
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedProv == nil {
		t.Fatal("expected a dbBackupProvider to be resolved from ctx control")
	}
}

// TestGameServersRunning_ExplicitCtrl verifies the function uses the passed ctrl
// rather than globalControl.
func TestGameServersRunning_ExplicitCtrl(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })

	ctrl := &recordingControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}

	running, err := gameServersRunning(t.Context(), ctrl, exec)
	if err != nil {
		t.Fatalf("gameServersRunning: %v", err)
	}
	if running {
		t.Error("expected false — recordingControl returns empty status")
	}
}
