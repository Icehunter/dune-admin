package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// dbBackupProviderOrErr guards the globals and asserts the control plane supports
// native DB backups, writing the appropriate error response if not.
func dbBackupProviderOrErr(w http.ResponseWriter) (dbBackupProvider, bool) {
	if globalControl == nil || globalExecutor == nil {
		jsonErr(w, fmt.Errorf("control plane not connected"), http.StatusServiceUnavailable)
		return nil, false
	}
	prov, ok := globalControl.(dbBackupProvider)
	if !ok {
		jsonErr(w, fmt.Errorf("database backups are not supported by the %q control plane", globalControl.Name()),
			http.StatusNotImplemented)
		return nil, false
	}
	return prov, true
}

// gameServersRunning reports whether any game-server processes are live, used as
// the "battlegroup is stopped" guard for the destructive restore.
func gameServersRunning(ctx context.Context) (bool, error) {
	st, err := globalControl.GetStatus(ctx, globalExecutor)
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
		log.Printf("handleDBBackupList: %v", err)
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
func handleDBBackupCreate(w http.ResponseWriter, _ *http.Request) {
	prov, ok := dbBackupProviderOrErr(w)
	if !ok {
		return
	}
	name, size, err := createDBBackup(prov)
	if err != nil {
		log.Printf("handleDBBackupCreate: %v", err)
		jsonErr(w, fmt.Errorf("backup failed"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": "backup created", "name": name, "size_bytes": size})
}

// createDBBackup takes a fresh pg_dump via the provider, verifies the archive,
// and returns the new file's name and size. Shared by the Database-page create
// handler and the battlegroup-page backup dispatch (issue #169).
func createDBBackup(prov dbBackupProvider) (name string, size int64, err error) {
	dir, err := dbBackupDir()
	if err != nil {
		return "", 0, fmt.Errorf("prepare backup dir: %w", err)
	}
	name = dbBackupFilename(time.Now())
	dest := filepath.Join(dir, name)
	if out, bErr := prov.BackupDatabase(globalExecutor, dbBackupConn(), dest); bErr != nil {
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
// dispatch (issue #169). Callers own the destructive-op (game-stopped) guard.
func restoreDBBackupFile(prov dbBackupProvider, name string) (string, error) {
	dir, err := dbBackupDir()
	if err != nil {
		return "", fmt.Errorf("backup dir unavailable: %w", err)
	}
	src := filepath.Join(dir, name)
	if _, statErr := os.Stat(src); statErr != nil {
		return "", fmt.Errorf("backup not found")
	}
	out, err := prov.RestoreDatabase(globalExecutor, dbBackupConn(), src)
	if err != nil {
		return out, fmt.Errorf("restore: %w", err)
	}
	invalidateAllJourneyCache() // the database was replaced under us
	return out, nil
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
		log.Printf("handleDBBackupDownload: %v", err)
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
		log.Printf("handleDBBackupDelete: %v", err)
		jsonErr(w, fmt.Errorf("could not delete backup"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "backup deleted"})
}

// @Summary Restore the database from a backup (DESTRUCTIVE — battlegroup must be stopped)
// @Tags db-backups
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
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
	if err := validateBackupName(body.File); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	prov, ok := dbBackupProviderOrErr(w)
	if !ok {
		return
	}
	// Destructive-op guard: refuse while the game is live — pg_restore --clean
	// over a running server would corrupt in-flight state.
	running, err := gameServersRunning(r.Context())
	if err != nil {
		log.Printf("handleDBBackupRestore: status check: %v", err)
		jsonErr(w, fmt.Errorf("could not verify the battlegroup is stopped"), http.StatusInternalServerError)
		return
	}
	if running {
		jsonErr(w, fmt.Errorf("stop the battlegroup before restoring — game servers are running"),
			http.StatusConflict)
		return
	}
	out, err := restoreDBBackupFile(prov, body.File)
	if err != nil {
		log.Printf("handleDBBackupRestore: %v", err)
		jsonErr(w, fmt.Errorf("restore failed"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "database restored", "output": out})
}
