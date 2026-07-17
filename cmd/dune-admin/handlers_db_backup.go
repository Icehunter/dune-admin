package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dbBackupProviderOrErr guards the control plane and asserts it supports native
// DB backups, writing the appropriate error response if not.
func dbBackupProviderOrErr(w http.ResponseWriter, r *http.Request) (dbBackupProvider, bool) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	if ctrl == nil || exec == nil {
		jsonErr(w, fmt.Errorf("control plane not connected"), http.StatusServiceUnavailable)
		return nil, false
	}
	prov, ok := ctrl.(dbBackupProvider)
	if !ok {
		jsonErr(w, fmt.Errorf("database backups are not supported by the %q control plane", ctrl.Name()),
			http.StatusNotImplemented)
		return nil, false
	}
	return prov, true
}

// gameServersRunning reports whether any game-server processes are live, used as
// the "battlegroup is stopped" guard for the destructive restore.
func gameServersRunning(ctx context.Context, ctrl ControlPlane, exec Executor) (bool, error) {
	st, err := ctrl.GetStatus(ctx, exec)
	if err != nil {
		return false, err
	}
	return len(st.Servers) > 0, nil
}

// verifyDumpFile sanity-checks that a freshly written backup is a non-empty
// pg_dump custom-format archive (magic "PGDMP"), so a silent failure (exit 0 but
// empty output) doesn't masquerade as a good backup.
func verifyDumpFile(path string) error {
	f, err := os.Open(path) // #nosec G304 G703 -- path is dbBackupDir() + a timestamped name we generated
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	hdr := make([]byte, 5)
	n, _ := io.ReadFull(f, hdr)
	if n < 5 || string(hdr[:5]) != "PGDMP" {
		return fmt.Errorf("not a pg_dump custom-format archive")
	}
	return nil
}

// @Summary List database backups
// @Tags db-backups
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/db-backups [get]
func handleDBBackupList(w http.ResponseWriter, _ *http.Request) {
	files, err := listDBBackups()
	if err != nil {
		componentLog("db_backup").Error().Err(err).Msg("list backups failed")
		jsonErr(w, fmt.Errorf("could not list backups"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"backups": files})
}

// @Summary Take a database backup now
// @Tags db-backups
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 501 {object} map[string]string
// @Router /api/v1/db-backups [post]
func handleDBBackupCreate(w http.ResponseWriter, r *http.Request) {
	prov, ok := dbBackupProviderOrErr(w, r)
	if !ok {
		return
	}
	name, size, err := createDBBackup(prov, executorFromCtx(r))
	if err != nil {
		componentLog("db_backup").Error().Err(err).Msg("create backup failed")
		jsonErr(w, fmt.Errorf("backup failed"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": "backup created", "name": name, "size_bytes": size})
}

// createDBBackup takes a fresh pg_dump via the provider, verifies the archive,
// and returns the new file's name and size. Shared by the Database-page create
// handler and the battlegroup-page backup dispatch (issue #169).
func createDBBackup(prov dbBackupProvider, exec Executor) (name string, size int64, err error) {
	dir, err := dbBackupDir()
	if err != nil {
		return "", 0, fmt.Errorf("prepare backup dir: %w", err)
	}
	name = dbBackupFilename(time.Now())
	dest := filepath.Join(dir, name)
	if out, bErr := prov.BackupDatabase(exec, dbBackupConn(), dest); bErr != nil {
		return "", 0, fmt.Errorf("backup: %w (%s)", bErr, out)
	}
	if vErr := verifyDumpFile(dest); vErr != nil {
		_ = os.Remove(dest)
		return "", 0, fmt.Errorf("backup produced no valid archive: %w", vErr)
	}
	if info, statErr := os.Stat(dest); statErr == nil {
		size = info.Size()
	}
	return name, size, nil
}

// restoreDBBackupFile restores a named .dump from the db-backup dir via the
// provider. The name must be validated by validateBackupName before calling.
// Shared by the Database-page restore handler and the battlegroup-page restore
// dispatch (issue #169). Callers own the destructive-op (game-stopped) guard —
// use prepareAndRestoreDB unless you have a reason not to. The result is
// interpreted by classifyPgRestoreResult, so an exit-1 run that completed with
// only ignorable errors reports success (with the count), not failure.
func restoreDBBackupFile(prov dbBackupProvider, name string, exec Executor) (string, int, error) {
	dir, err := dbBackupDir()
	if err != nil {
		return "", 0, fmt.Errorf("backup dir unavailable: %w", err)
	}
	src := filepath.Join(dir, name)
	if _, statErr := os.Stat(src); statErr != nil {
		return "", 0, fmt.Errorf("backup not found")
	}
	out, restoreErr := prov.RestoreDatabase(exec, dbBackupConn(), src)
	ignored, err := classifyPgRestoreResult(out, restoreErr)
	if err != nil {
		return out, 0, fmt.Errorf("restore: %w", err)
	}
	invalidateAllJourneyCache() // the database was replaced under us
	return out, ignored, nil
}

// errBattlegroupRunning is returned by prepareAndRestoreDB when game servers
// are running and the control plane can't stop just the shards (kubectl/
// docker/local — where stopping the battlegroup leaves Postgres up, so the
// operator can do it themselves). The Database-page handler maps it to 409.
var errBattlegroupRunning = fmt.Errorf("stop the battlegroup before restoring — game servers are running")

// dbRestoreResult is the outcome of a full prepareAndRestoreDB flow.
type dbRestoreResult struct {
	Output         string
	IgnoredErrors  int
	ServersStopped bool
}

// Step keys and statuses reported by prepareAndRestoreDB, mirrored in the
// frontend's restore-progress dialog (backups.restoreProgress.* i18n keys).
const (
	restoreStepCheck    = "check"
	restoreStepStop     = "stop"
	restoreStepRestore  = "restore"
	restoreStepFinalize = "finalize"

	restoreStatusRunning = "running"
	restoreStatusDone    = "done"
	restoreStatusSkipped = "skipped"
	restoreStatusFailed  = "failed"
)

// prepareAndRestoreDB is the one-click restore flow shared by the Database
// and Battlegroup pages: if game shards are running and the control plane can
// stop just them (AMP — where a full battlegroup stop would take Postgres
// down too and make restore impossible), stop them first; then restore and
// classify the result; then recycle the pgx pool (the schema was replaced
// under it, so pooled connections hold stale state). scope, when non-empty,
// is the server's cache scope — used to drop the health/battlegroup status
// caches after shards are stopped so the UI reflects reality immediately.
// report, when non-nil, receives each step transition (restoreStep* ×
// restoreStatus*) — this is what feeds the frontend's real step-progress
// dialog; nothing here is cosmetic. The battlegroup is deliberately left
// stopped after a successful restore: auto-starting the game on top of a
// restore the operator hasn't looked at yet is worse than one more click.
func prepareAndRestoreDB(ctx context.Context, ctrl ControlPlane, exec Executor, pool *pgxpool.Pool, scope, file string, report func(step, status string)) (dbRestoreResult, error) {
	if report == nil {
		report = func(string, string) {}
	}
	var res dbRestoreResult
	report(restoreStepCheck, restoreStatusRunning)
	running, err := gameServersRunning(ctx, ctrl, exec)
	if err != nil {
		report(restoreStepCheck, restoreStatusFailed)
		return res, fmt.Errorf("could not verify the battlegroup is stopped: %w", err)
	}
	report(restoreStepCheck, restoreStatusDone)
	if running {
		stopper, ok := ctrl.(gameServerStopper)
		if !ok {
			report(restoreStepStop, restoreStatusFailed)
			return res, errBattlegroupRunning
		}
		report(restoreStepStop, restoreStatusRunning)
		if err := stopper.StopGameServers(ctx, exec); err != nil {
			report(restoreStepStop, restoreStatusFailed)
			return res, fmt.Errorf("stop game servers: %w", err)
		}
		report(restoreStepStop, restoreStatusDone)
		res.ServersStopped = true
		if scope != "" {
			invalidateServerHealth(scope)
		}
	} else {
		report(restoreStepStop, restoreStatusSkipped)
	}
	report(restoreStepRestore, restoreStatusRunning)
	out, ignored, err := dispatchRestore(ctx, ctrl, exec, file)
	res.Output, res.IgnoredErrors = out, ignored
	if err != nil {
		report(restoreStepRestore, restoreStatusFailed)
		return res, err
	}
	report(restoreStepRestore, restoreStatusDone)
	report(restoreStepFinalize, restoreStatusRunning)
	if pool != nil {
		pool.Reset()
	}
	invalidateAllJourneyCache()
	// The restore runs as a background job that returned 202 long ago, so the
	// handleAPI cache-bust already fired (at job start, minutes back). Bust the
	// players list again HERE, at genuine completion, or a read during the
	// multi-minute restore repopulates the cache with pre-restore data that
	// then survives up to playersCacheTTL past the swap.
	if scope != "" {
		invalidatePlayersCache(scope)
	}
	report(restoreStepFinalize, restoreStatusDone)
	return res, nil
}

// @Summary Download a database backup
// @Tags db-backups
// @Produce octet-stream
// @Param file query string true "backup filename"
// @Router /api/v1/db-backups/download [get]
func handleDBBackupDownload(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	if err := validateBackupName(name); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	dir, err := dbBackupDir()
	if err != nil {
		jsonErr(w, fmt.Errorf("backup dir unavailable"), http.StatusInternalServerError)
		return
	}
	f, err := os.Open(filepath.Join(dir, name)) // #nosec G304 G703 -- name validated by validateBackupName (no separators/..)
	if err != nil {
		jsonErr(w, fmt.Errorf("backup not found"), http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	if _, err := io.Copy(w, f); err != nil {
		componentLog("db_backup").Error().Err(err).Msg("download copy failed")
	}
}

// @Summary Delete a database backup
// @Tags db-backups
// @Produce json
// @Param file query string true "backup filename"
// @Router /api/v1/db-backups [delete]
func handleDBBackupDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	if err := validateBackupName(name); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := deleteDBBackup(name); err != nil {
		componentLog("db_backup").Error().Err(err).Msg("delete backup failed")
		jsonErr(w, fmt.Errorf("could not delete backup"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "backup deleted"})
}

// startRestoreJob validates nothing — callers own validation. It launches the
// full prepareAndRestoreDB flow as a background job for the request's scope,
// capturing the request-scoped control/executor/pool BEFORE the request ends
// (the job outlives it, so it runs on context.Background()). Errors from the
// flow (including errBattlegroupRunning on no-stopper planes) surface in the
// polled job status, where the progress dialog shows them on the failing step.
func startRestoreJob(r *http.Request, file string) error {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	pool := dbFromCtx(r)
	scope := ""
	if sc := serverFromCtx(r); sc != nil {
		scope = sc.ID
	}
	return globalRestoreJobs.Start(scope, file, func(report func(step, status string)) (dbRestoreResult, error) {
		res, err := prepareAndRestoreDB(context.Background(), ctrl, exec, pool, scope, file, report)
		if err != nil {
			componentLog("db_backup").Error().Err(err).Str("file", file).Msg("restore failed")
		}
		return res, err
	})
}

// restoreScopeFromRequest resolves the job scope for status lookups.
func restoreScopeFromRequest(r *http.Request) string {
	if sc := serverFromCtx(r); sc != nil {
		return sc.ID
	}
	return ""
}

// @Summary Start a database restore job (DESTRUCTIVE — stops running game servers first where supported)
// @Tags db-backups
// @Accept json
// @Produce json
// @Success 202 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /api/v1/db-backups/restore [post]
func handleDBBackupRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		File    string `json:"file"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if !body.Confirm {
		jsonErr(w, fmt.Errorf("restore requires confirm=true"), http.StatusBadRequest)
		return
	}
	// Validate BEFORE starting — never touch game servers over a bad filename.
	if err := validateBackupName(body.File); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if _, ok := dbBackupProviderOrErr(w, r); !ok {
		return
	}
	if err := startRestoreJob(r, body.File); err != nil {
		jsonErr(w, err, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "restore started"})
}

// @Summary Poll the current restore job's progress for this server
// @Tags db-backups
// @Produce json
// @Success 200 {object} dbRestoreStatus
// @Router /api/v1/db-backups/restore/status [get]
func handleDBRestoreStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, globalRestoreJobs.Status(restoreScopeFromRequest(r)))
}
