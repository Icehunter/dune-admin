package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type backupFile struct {
	Name     string `json:"name"`
	SizeB    int64  `json:"size_bytes"`
	Modified string `json:"modified"`
	HasYAML  bool   `json:"has_yaml"`
}

var bgCmdAllowlist = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"update": true,
	// backup handled separately via handleBGBackup → dispatchBackup
	// restore handled separately via handleBGRestore → dispatchRestore
}

// @Summary Get battlegroup and server status from the control plane
// @Tags battlegroup
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/status [get]
func handleBGStatus(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	if ctrl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	status, err := cachedBGStatus(r, ctrl, exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{"battlegroup": map[string]string{
		"name":     status.Name,
		"title":    status.Title,
		"phase":    status.Phase,
		"database": status.Database,
	}, "servers": status.Servers})
}

// cachedBGStatus serves the battlegroup status from the per-server cache (keyed
// by the request's server scope), loading live on a miss. Falls back to a direct
// GetStatus when the request has no server context or the cache is unavailable.
func cachedBGStatus(r *http.Request, ctrl ControlPlane, exec Executor) (*BattlegroupStatus, error) {
	sc := serverFromCtx(r)
	if sc == nil || globalBGStatusCache == nil {
		return ctrl.GetStatus(r.Context(), exec)
	}
	return globalBGStatusCache.GetOrLoad(r.Context(), cacheKey(sc.ID, "bgstatus"), healthCacheTTL,
		func(ctx context.Context) (*BattlegroupStatus, error) {
			return ctrl.GetStatus(ctx, exec)
		})
}

func safeIdx(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// @Summary Execute a battlegroup lifecycle command via the control plane
// @Tags battlegroup
// @Accept json
// @Produce json
// @Param body body object true "Command: start, stop, restart, update, or backup"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/exec [post]
func handleBGExec(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	var req struct {
		Cmd string `json:"cmd"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if ctrl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	// backup is dispatched to the db-backup provider when one is active (AMP),
	// otherwise to ExecCommand("backup") (kubectl). It is NOT in bgCmdAllowlist.
	if req.Cmd == "backup" {
		out, err := dispatchBackup(r.Context(), ctrl, exec)
		if err != nil {
			jsonErr(w, fmt.Errorf("backup: %w — output: %s", err, out), 500)
			return
		}
		jsonOK(w, map[string]string{"output": out})
		return
	}
	if !bgCmdAllowlist[req.Cmd] {
		jsonErr(w, fmt.Errorf("unknown command %q", req.Cmd), 400)
		return
	}
	out, err := ctrl.ExecCommand(r.Context(), exec, req.Cmd)
	if err != nil {
		jsonErr(w, fmt.Errorf("exec: %w — output: %s", err, out), 500)
		return
	}
	// A lifecycle command (start/stop/restart) changes status — drop the cached
	// health + battlegroup status so the UI reflects it on the next poll.
	if sc := serverFromCtx(r); sc != nil {
		invalidateServerHealth(sc.ID)
	}
	jsonOK(w, map[string]string{"output": out})
}

// @Summary Restart a single map/partition without cycling the whole Battlegroup
// @Tags battlegroup
// @Accept json
// @Produce json
// @Param body body object true "partition: partition index to restart"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 501 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/restart-partition [post]
func handleBGRestartPartition(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	if ctrl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	restarter, ok := ctrl.(partitionRestarter)
	if !ok {
		jsonErr(w, fmt.Errorf("%s control plane does not support per-partition restart", ctrl.Name()), http.StatusNotImplemented)
		return
	}
	var req struct {
		Partition int `json:"partition"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.Partition < 0 {
		jsonErr(w, fmt.Errorf("invalid partition %d", req.Partition), 400)
		return
	}
	out, err := restarter.RestartPartition(r.Context(), executorFromCtx(r), req.Partition)
	if err != nil {
		jsonErr(w, fmt.Errorf("restart partition %d: %w — output: %s", req.Partition, err, out), 500)
		return
	}
	// Same cache-drop as the whole-battlegroup lifecycle commands — a
	// partition restart changes that server's status too.
	if sc := serverFromCtx(r); sc != nil {
		invalidateServerHealth(sc.ID)
	}
	jsonOK(w, map[string]string{"output": out})
}

// @Summary List battlegroup pods/processes and their namespace
// @Tags battlegroup
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/pods [get]
func handleBGPods(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	if ctrl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	procs, ns, err := ctrl.ListProcesses(r.Context(), exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	// Return raw lines for backward compat with the frontend which renders them as-is.
	var lines []string
	for _, p := range procs {
		lines = append(lines, p.Name)
	}
	jsonOK(w, map[string]any{"pods": lines, "namespace": ns})
}

func activeBackupDir(ctrl ControlPlane, exec Executor) (string, error) {
	if backupDir != "" {
		return backupDir, nil
	}
	// backup_dir is per-server (servers table) after the remodel; the global
	// loadedConfig field is cleared, so resolve from the active server.
	if d := activeServerCfg().BackupDir; d != "" {
		return d, nil
	}
	ns := firstNonEmpty(controlNS, loadedConfig.ControlNamespace, globalPodNS)
	bg := strings.TrimPrefix(ns, "funcom-seabass-")
	if ctrl != nil && ctrl.Name() == "local" && ns != "" && exec != nil {
		pod, err := discoverK8sBackupPod(ns, exec)
		if err == nil && pod != "" && bg != "" {
			return fmt.Sprintf("k8s://%s/%s/home/dune/artifacts/database-dumps/%s", ns, pod, bg), nil
		}
	}
	if bg != "" {
		// Legacy kubectl/host default.
		return fmt.Sprintf("/funcom/artifacts/database-dumps/%s", bg), nil
	}
	return "", fmt.Errorf("backup_dir not configured and no battlegroup namespace discovered")
}

func parseK8sBackupDir(dir string) (ns, pod, inPodDir string, ok bool) {
	const prefix = "k8s://"
	if !strings.HasPrefix(dir, prefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(dir, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	ns, pod, inPodDir = parts[0], parts[1], "/"+strings.TrimLeft(parts[2], "/")
	return ns, pod, inPodDir, true
}

func discoverK8sBackupPod(ns string, exec Executor) (string, error) {
	if exec == nil {
		return "", fmt.Errorf("not connected")
	}
	kctl := kubectlCLI(exec)
	out, err := exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep -- '-sg-' | head -1",
		kctl, shellQuote(ns),
	))
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	out, err = exec.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
		kctl, shellQuote(ns),
	))
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	return "", fmt.Errorf("could not discover backup pod in namespace %s", ns)
}

func ensureBackupDir(dir string, exec Executor) error {
	if exec == nil {
		return fmt.Errorf("not connected")
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(exec)
		out, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- mkdir -p %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(inPodDir),
		))
		if err != nil {
			return fmt.Errorf("ensure k8s backup dir: %w (%s)", err, strings.TrimSpace(out))
		}
		return nil
	}
	out, err := exec.Exec(fmt.Sprintf(
		"mkdir -p %s 2>/dev/null || sudo mkdir -p %s 2>&1",
		shellQuote(dir), shellQuote(dir),
	))
	if err != nil {
		return fmt.Errorf("ensure backup dir: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func listBackupDir(dir string, exec Executor) (string, string, error) {
	if exec == nil {
		return "", "", fmt.Errorf("not connected")
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(exec)
		listCmd := fmt.Sprintf(`ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`, inPodDir)
		out, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(listCmd),
		))
		if err != nil {
			return "", "", fmt.Errorf("list backups: %w (%s)", err, strings.TrimSpace(out))
		}
		yamlCmd := fmt.Sprintf(`ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`, inPodDir)
		yamlOut, err := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(yamlCmd),
		))
		if err != nil {
			return "", "", fmt.Errorf("list backup metadata: %w (%s)", err, strings.TrimSpace(yamlOut))
		}
		return out, yamlOut, nil
	}
	out, err := exec.Exec(fmt.Sprintf(
		`ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`,
		dir))
	if err != nil {
		out, err = exec.Exec(fmt.Sprintf(
			`sudo ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`,
			dir))
		if err != nil {
			return "", "", fmt.Errorf("list backups: %w (%s)", err, strings.TrimSpace(out))
		}
	}
	yamlOut, err := exec.Exec(fmt.Sprintf(
		`ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`,
		dir))
	if err != nil {
		yamlOut, err = exec.Exec(fmt.Sprintf(
			`sudo ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`,
			dir))
		if err != nil {
			return "", "", fmt.Errorf("list backup metadata: %w (%s)", err, strings.TrimSpace(yamlOut))
		}
	}
	return out, yamlOut, nil
}

func backupFileExists(dir, name string, exec Executor) bool {
	if exec == nil {
		return false
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(exec)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		out, _ := exec.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>/dev/null",
			kctl, shellQuote(ns), shellQuote(pod),
			shellQuote(fmt.Sprintf("test -f %s && echo yes || echo no", shellQuote(remotePath))),
		))
		return strings.TrimSpace(out) == "yes"
	}
	path := strings.TrimRight(dir, "/") + "/" + name
	out, _ := exec.Exec(fmt.Sprintf("test -f %s && echo yes || echo no", shellQuote(path)))
	if strings.TrimSpace(out) == "yes" {
		return true
	}
	out, _ = exec.Exec(fmt.Sprintf("sudo test -f %s && echo yes || echo no", shellQuote(path)))
	return strings.TrimSpace(out) == "yes"
}

func backupReadCmd(dir, name string, exec Executor) string {
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(exec)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		return fmt.Sprintf("%s exec -n %s %s -- cat %s", kctl, shellQuote(ns), shellQuote(pod), shellQuote(remotePath))
	}
	path := strings.TrimRight(dir, "/") + "/" + name
	return fmt.Sprintf("cat %s 2>/dev/null || sudo cat %s", shellQuote(path), shellQuote(path))
}

func writeBackupFile(dir, name string, src io.Reader, exec Executor) error {
	if exec == nil {
		return fmt.Errorf("not connected")
	}
	if err := ensureBackupDir(dir, exec); err != nil {
		return err
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		tmp := fmt.Sprintf("/tmp/dune-admin-backup-%d.tmp", time.Now().UnixNano())
		if err := exec.WriteFile(tmp, src); err != nil {
			return fmt.Errorf("stage upload: %w", err)
		}
		defer func() {
			_, _ = exec.Exec(fmt.Sprintf("rm -f %s 2>/dev/null || sudo rm -f %s 2>/dev/null || true",
				shellQuote(tmp), shellQuote(tmp)))
		}()
		kctl := kubectlCLI(exec)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		out, err := exec.Exec(fmt.Sprintf(
			"%s cp %s %s/%s:%s 2>&1",
			kctl, shellQuote(tmp), shellQuote(ns), shellQuote(pod), shellQuote(remotePath),
		))
		if err != nil {
			return fmt.Errorf("copy to k8s pod: %w (%s)", err, strings.TrimSpace(out))
		}
		return nil
	}
	cleanDir := filepath.Clean(dir)
	destPath := filepath.Join(cleanDir, name)
	if !strings.HasPrefix(destPath, cleanDir+string(filepath.Separator)) {
		return fmt.Errorf("backup entry %q escapes target directory", name)
	}
	return exec.WriteFile(destPath, src)
}

// @Summary List available database backup files in the backup directory
// @Tags battlegroup
// @Produce json
// @Success 200 {object} []backupFile
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/backup-files [get]
func handleBGBackupFiles(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	if exec == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	// When a db-backup provider is active (AMP), the battlegroup page shares the
	// same .dump store as the Database page (issue #169).
	if _, ok := ctrl.(dbBackupProvider); ok {
		files, err := listDBBackupsAsBGFiles()
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		jsonOK(w, files)
		return
	}
	dir, err := activeBackupDir(ctrl, exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if err := ensureBackupDir(dir, exec); err != nil {
		jsonErr(w, err, 500)
		return
	}
	out, yamlOut, err := listBackupDir(dir, exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, parseHostBackupListing(out, yamlOut))
}

// parseHostBackupListing turns the `ls`/yaml output from listBackupDir into the
// backupFile slice the frontend renders (always non-nil).
func parseHostBackupListing(out, yamlOut string) []backupFile {
	hasYAML := make(map[string]bool)
	for n := range strings.SplitSeq(strings.TrimSpace(yamlOut), "\n") {
		if n != "" {
			hasYAML[strings.TrimSpace(n)] = true
		}
	}
	files := []backupFile{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p := strings.SplitN(line, "|", 3)
		if len(p) < 3 {
			continue
		}
		size, _ := strconv.ParseInt(p[1], 10, 64)
		files = append(files, backupFile{Name: p[0], SizeB: size, Modified: p[2], HasYAML: hasYAML[p[0]]})
	}
	return files
}

// @Summary Download a backup file (and its YAML metadata) as a zip archive
// @Tags battlegroup
// @Produce application/zip
// @Param file query string true "Backup filename (must end in .backup)"
// @Success 200 {file} binary
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/backup-files/download [get]
func handleBGBackupDownload(w http.ResponseWriter, r *http.Request) {
	exec := executorFromCtx(r)
	if exec == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	// Under a db-backup provider (AMP) the store holds .dump files served from
	// the host dir — reuse the Database-page download path (issue #169).
	if _, ok := controlFromCtx(r).(dbBackupProvider); ok {
		handleDBBackupDownload(w, r)
		return
	}
	filename := r.URL.Query().Get("file")
	if filename == "" || strings.ContainsAny(filename, "/\\") || !strings.HasSuffix(filename, ".backup") {
		jsonErr(w, fmt.Errorf("invalid filename"), 400)
		return
	}
	baseName := strings.TrimSuffix(filename, ".backup")
	dir, err := activeBackupDir(controlFromCtx(r), exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, baseName))
	w.Header().Set("Content-Type", "application/zip")

	zw := zip.NewWriter(w)
	for _, ext := range []string{".backup", ".backup.yaml"} {
		name := baseName + ext
		if !backupFileExists(dir, name, exec) {
			continue
		}
		fw, err := zw.Create(name)
		if err != nil {
			continue
		}
		if err := exec.PipeToWriter(backupReadCmd(dir, name, exec), fw); err != nil {
			fmt.Printf("zip entry %s: %v\n", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Printf("zip close: %v\n", err)
	}
}

// @Summary Restore the database from a named backup file via the control plane
// @Tags battlegroup
// @Accept json
// @Produce json
// @Param body body object true "Backup filename (must end in .backup)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/restore [post]
func handleBGRestore(w http.ResponseWriter, r *http.Request) {
	ctrl := controlFromCtx(r)
	exec := executorFromCtx(r)
	if ctrl == nil || exec == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	var req struct {
		File string `json:"file"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	// Validate BEFORE starting — never touch game servers over a bad filename.
	if err := validateRestoreFilename(ctrl, req.File); err != nil {
		jsonErr(w, err, 400)
		return
	}
	// Same one-click background flow as the Database page: stop running shards
	// where the control plane supports it (AMP), restore, classify, recycle
	// the pool — progress polled via /db-backups/restore/status. This closes
	// the guard gap where this handler previously restored over a live
	// battlegroup with no check at all.
	if err := startRestoreJob(r, req.File); err != nil {
		jsonErr(w, err, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "restore started"})
}

func allowedBackupArchiveEntry(entryName string) (string, bool) {
	name := filepath.Base(entryName)
	if strings.ContainsAny(name, "/\\") {
		return "", false
	}
	if strings.HasSuffix(name, ".backup") || strings.HasSuffix(name, ".backup.yaml") {
		return name, true
	}
	return "", false
}

func writeBackupArchiveEntries(dir string, zr *zip.Reader, exec Executor) (string, error) {
	var backupName string
	for _, zf := range zr.File {
		name, ok := allowedBackupArchiveEntry(zf.Name)
		if !ok {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		if err := writeBackupFile(dir, name, rc, exec); err != nil {
			_ = rc.Close()
			return "", fmt.Errorf("upload failed for %s: %w", name, err)
		}
		if err := rc.Close(); err != nil {
			continue
		}
		if strings.HasSuffix(name, ".backup") {
			backupName = name
		}
	}
	return backupName, nil
}

func uploadBackupArchive(dir string, file multipart.File, exec Executor) (string, int, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return "", 400, fmt.Errorf("read zip: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 400, fmt.Errorf("invalid zip: %w", err)
	}
	backupName, err := writeBackupArchiveEntries(dir, zr, exec)
	if err != nil {
		return "", 500, err
	}
	if backupName == "" {
		return "", 400, fmt.Errorf("zip contains no .backup file")
	}
	return backupName, 200, nil
}

func isDirectBackupUpload(filename string) bool {
	return strings.HasSuffix(filename, ".backup") && !strings.ContainsAny(filename, "/\\")
}

// @Summary Upload a backup file (.backup or .zip) to the backup directory
// @Tags battlegroup
// @Accept multipart/form-data
// @Produce json
// @Param backup formData file true "Backup file (.backup or .zip)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/battlegroup/backup-files/upload [post]
func handleBGBackupUpload(w http.ResponseWriter, r *http.Request) {
	exec := executorFromCtx(r)
	if exec == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonErr(w, fmt.Errorf("parse form: %w", err), 400)
		return
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		jsonErr(w, fmt.Errorf("no file: %w", err), 400)
		return
	}
	defer func() { _ = file.Close() }()

	filename := header.Filename
	dir, err := activeBackupDir(controlFromCtx(r), exec)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if err := ensureBackupDir(dir, exec); err != nil {
		jsonErr(w, err, 500)
		return
	}

	if strings.HasSuffix(filename, ".zip") {
		backupName, status, err := uploadBackupArchive(dir, file, exec)
		if err != nil {
			jsonErr(w, err, status)
			return
		}
		jsonOK(w, map[string]string{"name": backupName})
		return
	}

	if isDirectBackupUpload(filename) {
		if err := writeBackupFile(dir, filename, file, exec); err != nil {
			jsonErr(w, fmt.Errorf("upload failed: %w", err), 500)
			return
		}
		jsonOK(w, map[string]string{"name": filename})
		return
	}

	jsonErr(w, fmt.Errorf("file must be .backup or .zip"), 400)
}

// restoreViaControl runs a restore command appropriate for the active control plane.
// Called via dispatchRestore — kept separate so the restore logic per-provider
// can be extended without touching the HTTP handler. The pg_restore result is
// interpreted by classifyPgRestoreResult (exit 1 with a completion summary is
// success-with-ignorable-errors, not failure).
func restoreViaControl(_ context.Context, ctrl ControlPlane, exec Executor, filename string) (string, int, error) {
	// kubectl uses the battlegroup.sh import script — not pg_restore, so its
	// exit code is meaningful as-is.
	// TODO: NEVER run battlegroup.sh with sudo — see ExecCommand in control_kubectl.go.
	if ctrl != nil && ctrl.Name() == "kubectl" {
		out, err := exec.Exec(fmt.Sprintf(
			`echo yes | ~/.dune/download/scripts/battlegroup.sh import %s 2>&1`,
			shellQuote(filename)))
		return out, 0, err
	}
	// docker / local: pg_restore from the backup directory.
	dir, err := activeBackupDir(ctrl, exec)
	if err != nil {
		return "", 0, err
	}
	path := strings.TrimRight(dir, "/") + "/" + filename
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(exec)
		tmp := fmt.Sprintf("/tmp/dune-admin-restore-%d.backup", time.Now().UnixNano())
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + filename
		copyOut, copyErr := exec.Exec(fmt.Sprintf(
			"%s cp %s/%s:%s %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(remotePath), shellQuote(tmp),
		))
		if copyErr != nil {
			return copyOut, 0, fmt.Errorf("copy backup to local restore path: %w", copyErr)
		}
		defer func() {
			_, _ = exec.Exec(fmt.Sprintf("rm -f %s 2>/dev/null || sudo rm -f %s 2>/dev/null || true",
				shellQuote(tmp), shellQuote(tmp)))
		}()
		path = tmp
	}
	out, restoreErr := exec.Exec(fmt.Sprintf(
		`PGPASSWORD=%s pg_restore --no-password --clean --if-exists -h %s -p %d -U %s -d %s %s 2>&1`,
		shellQuote(dbPass), dbHost, dbPort, dbUser, dbName, shellQuote(path)))
	ignored, err := classifyPgRestoreResult(out, restoreErr)
	if err != nil {
		return out, 0, err
	}
	return out, ignored, nil
}

// dispatchBackup runs a battlegroup backup against the active control plane.
// When the plane implements dbBackupProvider (AMP), a backup IS a database
// backup: it delegates to the same pg_dump path the Database page uses, sharing
// one .dump store (issue #169). Otherwise it falls back to ExecCommand("backup")
// (kubectl runs battlegroup.sh backup). The returned string is human-facing
// output forwarded to the command-output modal.
func dispatchBackup(ctx context.Context, ctrl ControlPlane, exec Executor) (string, error) {
	if ctrl == nil {
		return "", fmt.Errorf("not connected")
	}
	if prov, ok := ctrl.(dbBackupProvider); ok {
		name, size, err := createDBBackup(prov, exec)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("database backup created: %s (%d bytes)", name, size), nil
	}
	return ctrl.ExecCommand(ctx, exec, "backup")
}

// dispatchRestore restores a named backup against the active control plane.
// When the plane implements dbBackupProvider (AMP) the file is a .dump restored
// via pg_restore (issue #169); otherwise it is a .backup restored via
// restoreViaControl (kubectl battlegroup.sh import / docker / local pg_restore).
// Filename validation branches on the active store's extension. Returns the
// human-facing output and the count of ignorable pg_restore errors (see
// classifyPgRestoreResult). Callers own the game-stopped guard — use
// prepareAndRestoreDB unless you have a reason not to.
func dispatchRestore(ctx context.Context, ctrl ControlPlane, exec Executor, filename string) (string, int, error) {
	if ctrl == nil || exec == nil {
		return "", 0, fmt.Errorf("not connected")
	}
	if err := validateRestoreFilename(ctrl, filename); err != nil {
		return "", 0, err
	}
	if prov, ok := ctrl.(dbBackupProvider); ok {
		return restoreDBBackupFile(prov, filename, exec)
	}
	return restoreViaControl(ctx, ctrl, exec, filename)
}

// validateRestoreFilename applies the control-plane-appropriate filename rule:
// .dump (strict charset, validateBackupName) for dbBackupProvider planes (AMP),
// .backup with no path separators for the legacy control-script path. Shared
// by dispatchRestore and handleBGRestore's pre-start validation.
func validateRestoreFilename(ctrl ControlPlane, filename string) error {
	if _, ok := ctrl.(dbBackupProvider); ok {
		return validateBackupName(filename)
	}
	if filename == "" || strings.ContainsAny(filename, `/\`) || !strings.HasSuffix(filename, ".backup") {
		return fmt.Errorf("invalid filename")
	}
	return nil
}

// listDBBackupsAsBGFiles adapts the Database-page .dump listing into the
// battlegroup page's backupFile shape so both pages render the same store under
// AMP. .dump files carry no sidecar YAML, so HasYAML is always false.
func listDBBackupsAsBGFiles() ([]backupFile, error) {
	dumps, err := listDBBackups()
	if err != nil {
		return nil, err
	}
	files := make([]backupFile, 0, len(dumps))
	for _, d := range dumps {
		files = append(files, backupFile{Name: d.Name, SizeB: d.SizeB, Modified: d.Modified, HasYAML: false})
	}
	return files, nil
}
