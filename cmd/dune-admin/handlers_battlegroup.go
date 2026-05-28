package main

import (
	"archive/zip"
	"bytes"
	"context"
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
	"update": true, "backup": true,
	// restore handled separately via handleBGRestore
}

func handleBGStatus(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	status, err := globalControl.GetStatus(r.Context(), globalExecutor)
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

func safeIdx(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func handleBGExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cmd string `json:"cmd"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if !bgCmdAllowlist[req.Cmd] {
		jsonErr(w, fmt.Errorf("unknown command %q", req.Cmd), 400)
		return
	}
	if globalControl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	out, err := globalControl.ExecCommand(r.Context(), globalExecutor, req.Cmd)
	if err != nil {
		jsonErr(w, fmt.Errorf("exec: %w — output: %s", err, out), 500)
		return
	}
	jsonOK(w, map[string]string{"output": out})
}

func handleBGPods(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	procs, ns, err := globalControl.ListProcesses(r.Context(), globalExecutor)
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

func activeBackupDir() (string, error) {
	if backupDir != "" {
		return backupDir, nil
	}
	if loadedConfig.BackupDir != "" {
		return loadedConfig.BackupDir, nil
	}
	ns := firstNonEmpty(controlNS, loadedConfig.ControlNamespace, globalPodNS)
	bg := strings.TrimPrefix(ns, "funcom-seabass-")
	if globalControl != nil && globalControl.Name() == "local" && ns != "" && globalExecutor != nil {
		pod, err := discoverK8sBackupPod(ns)
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

func discoverK8sBackupPod(ns string) (string, error) {
	if globalExecutor == nil {
		return "", fmt.Errorf("not connected")
	}
	kctl := kubectlCLI(globalExecutor)
	out, err := globalExecutor.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep -- '-sg-' | head -1",
		kctl, shellQuote(ns),
	))
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	out, err = globalExecutor.Exec(fmt.Sprintf(
		"%s get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep bgd | head -1",
		kctl, shellQuote(ns),
	))
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	return "", fmt.Errorf("could not discover backup pod in namespace %s", ns)
}

func ensureBackupDir(dir string) error {
	if globalExecutor == nil {
		return fmt.Errorf("not connected")
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(globalExecutor)
		out, err := globalExecutor.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- mkdir -p %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(inPodDir),
		))
		if err != nil {
			return fmt.Errorf("ensure k8s backup dir: %w (%s)", err, strings.TrimSpace(out))
		}
		return nil
	}
	out, err := globalExecutor.Exec(fmt.Sprintf(
		"mkdir -p %s 2>/dev/null || sudo mkdir -p %s 2>&1",
		shellQuote(dir), shellQuote(dir),
	))
	if err != nil {
		return fmt.Errorf("ensure backup dir: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func listBackupDir(dir string) (string, string, error) {
	if globalExecutor == nil {
		return "", "", fmt.Errorf("not connected")
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(globalExecutor)
		listCmd := fmt.Sprintf(`ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`, inPodDir)
		out, err := globalExecutor.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(listCmd),
		))
		if err != nil {
			return "", "", fmt.Errorf("list backups: %w (%s)", err, strings.TrimSpace(out))
		}
		yamlCmd := fmt.Sprintf(`ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`, inPodDir)
		yamlOut, err := globalExecutor.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(yamlCmd),
		))
		if err != nil {
			return "", "", fmt.Errorf("list backup metadata: %w (%s)", err, strings.TrimSpace(yamlOut))
		}
		return out, yamlOut, nil
	}
	out, err := globalExecutor.Exec(fmt.Sprintf(
		`ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`,
		dir))
	if err != nil {
		out, err = globalExecutor.Exec(fmt.Sprintf(
			`sudo ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`,
			dir))
		if err != nil {
			return "", "", fmt.Errorf("list backups: %w (%s)", err, strings.TrimSpace(out))
		}
	}
	yamlOut, err := globalExecutor.Exec(fmt.Sprintf(
		`ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`,
		dir))
	if err != nil {
		yamlOut, err = globalExecutor.Exec(fmt.Sprintf(
			`sudo ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`,
			dir))
		if err != nil {
			return "", "", fmt.Errorf("list backup metadata: %w (%s)", err, strings.TrimSpace(yamlOut))
		}
	}
	return out, yamlOut, nil
}

func backupFileExists(dir, name string) bool {
	if globalExecutor == nil {
		return false
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(globalExecutor)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		out, _ := globalExecutor.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- sh -lc %s 2>/dev/null",
			kctl, shellQuote(ns), shellQuote(pod),
			shellQuote(fmt.Sprintf("test -f %s && echo yes || echo no", shellQuote(remotePath))),
		))
		return strings.TrimSpace(out) == "yes"
	}
	path := strings.TrimRight(dir, "/") + "/" + name
	out, _ := globalExecutor.Exec(fmt.Sprintf("test -f %s && echo yes || echo no", shellQuote(path)))
	if strings.TrimSpace(out) == "yes" {
		return true
	}
	out, _ = globalExecutor.Exec(fmt.Sprintf("sudo test -f %s && echo yes || echo no", shellQuote(path)))
	return strings.TrimSpace(out) == "yes"
}

func backupReadCmd(dir, name string) string {
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(globalExecutor)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		return fmt.Sprintf("%s exec -n %s %s -- cat %s", kctl, shellQuote(ns), shellQuote(pod), shellQuote(remotePath))
	}
	path := strings.TrimRight(dir, "/") + "/" + name
	return fmt.Sprintf("cat %s 2>/dev/null || sudo cat %s", shellQuote(path), shellQuote(path))
}

func writeBackupFile(dir, name string, src io.Reader) error {
	if globalExecutor == nil {
		return fmt.Errorf("not connected")
	}
	if err := ensureBackupDir(dir); err != nil {
		return err
	}
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		tmp := fmt.Sprintf("/tmp/dune-admin-backup-%d.tmp", time.Now().UnixNano())
		if err := globalExecutor.WriteFile(tmp, src); err != nil {
			return fmt.Errorf("stage upload: %w", err)
		}
		defer func() {
			_, _ = globalExecutor.Exec(fmt.Sprintf("rm -f %s 2>/dev/null || sudo rm -f %s 2>/dev/null || true",
				shellQuote(tmp), shellQuote(tmp)))
		}()
		kctl := kubectlCLI(globalExecutor)
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + name
		out, err := globalExecutor.Exec(fmt.Sprintf(
			"%s cp %s %s/%s:%s 2>&1",
			kctl, shellQuote(tmp), shellQuote(ns), shellQuote(pod), shellQuote(remotePath),
		))
		if err != nil {
			return fmt.Errorf("copy to k8s pod: %w (%s)", err, strings.TrimSpace(out))
		}
		return nil
	}
	return globalExecutor.WriteFile(strings.TrimRight(dir, "/")+"/"+name, src)
}

func handleBGBackupFiles(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	dir, err := activeBackupDir()
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if err := ensureBackupDir(dir); err != nil {
		jsonErr(w, err, 500)
		return
	}
	out, yamlOut, err := listBackupDir(dir)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	hasYAML := make(map[string]bool)
	for _, n := range strings.Split(strings.TrimSpace(yamlOut), "\n") {
		if n != "" {
			hasYAML[strings.TrimSpace(n)] = true
		}
	}
	var files []backupFile
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p := strings.SplitN(line, "|", 3)
		if len(p) < 3 {
			continue
		}
		size, _ := strconv.ParseInt(p[1], 10, 64)
		name := p[0]
		files = append(files, backupFile{Name: name, SizeB: size, Modified: p[2], HasYAML: hasYAML[name]})
	}
	if files == nil {
		files = []backupFile{}
	}
	jsonOK(w, files)
}

func handleBGBackupDownload(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	filename := r.URL.Query().Get("file")
	if filename == "" || strings.ContainsAny(filename, "/\\") || !strings.HasSuffix(filename, ".backup") {
		jsonErr(w, fmt.Errorf("invalid filename"), 400)
		return
	}
	baseName := strings.TrimSuffix(filename, ".backup")
	dir, err := activeBackupDir()
	if err != nil {
		jsonErr(w, err, 500)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, baseName))
	w.Header().Set("Content-Type", "application/zip")

	zw := zip.NewWriter(w)
	for _, ext := range []string{".backup", ".backup.yaml"} {
		name := baseName + ext
		if !backupFileExists(dir, name) {
			continue
		}
		fw, err := zw.Create(name)
		if err != nil {
			continue
		}
		if err := globalExecutor.PipeToWriter(backupReadCmd(dir, name), fw); err != nil {
			fmt.Printf("zip entry %s: %v\n", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Printf("zip close: %v\n", err)
	}
}

func handleBGRestore(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil || globalExecutor == nil {
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
	if req.File == "" || strings.ContainsAny(req.File, "/\\") || !strings.HasSuffix(req.File, ".backup") {
		jsonErr(w, fmt.Errorf("invalid filename"), 400)
		return
	}
	out, err := restoreViaControl(r.Context(), req.File)
	if err != nil {
		jsonErr(w, fmt.Errorf("restore failed: %w\n%s", err, out), 500)
		return
	}
	jsonOK(w, map[string]string{"output": out})
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

func writeBackupArchiveEntries(dir string, zr *zip.Reader) (string, error) {
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
		if err := writeBackupFile(dir, name, rc); err != nil {
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

func uploadBackupArchive(dir string, file multipart.File) (string, int, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return "", 400, fmt.Errorf("read zip: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 400, fmt.Errorf("invalid zip: %w", err)
	}
	backupName, err := writeBackupArchiveEntries(dir, zr)
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

func handleBGBackupUpload(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
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
	dir, err := activeBackupDir()
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if err := ensureBackupDir(dir); err != nil {
		jsonErr(w, err, 500)
		return
	}

	if strings.HasSuffix(filename, ".zip") {
		backupName, status, err := uploadBackupArchive(dir, file)
		if err != nil {
			jsonErr(w, err, status)
			return
		}
		jsonOK(w, map[string]string{"name": backupName})
		return
	}

	if isDirectBackupUpload(filename) {
		if err := writeBackupFile(dir, filename, file); err != nil {
			jsonErr(w, fmt.Errorf("upload failed: %w", err), 500)
			return
		}
		jsonOK(w, map[string]string{"name": filename})
		return
	}

	jsonErr(w, fmt.Errorf("file must be .backup or .zip"), 400)
}

// restoreViaControl runs a restore command appropriate for the active control plane.
// Called by handleBGRestore — kept separate so the restore logic per-provider
// can be extended without touching the HTTP handler.
func restoreViaControl(ctx context.Context, filename string) (string, error) {
	// kubectl uses the battlegroup.sh import script.
	// TODO: NEVER run battlegroup.sh with sudo — see ExecCommand in control_kubectl.go.
	if globalControl != nil && globalControl.Name() == "kubectl" {
		return globalExecutor.Exec(fmt.Sprintf(
			`echo yes | ~/.dune/download/scripts/battlegroup.sh import %s 2>&1`,
			shellQuote(filename)))
	}
	// docker / local: pg_restore from the backup directory.
	dir, err := activeBackupDir()
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(dir, "/") + "/" + filename
	if ns, pod, inPodDir, ok := parseK8sBackupDir(dir); ok {
		kctl := kubectlCLI(globalExecutor)
		tmp := fmt.Sprintf("/tmp/dune-admin-restore-%d.backup", time.Now().UnixNano())
		remotePath := strings.TrimRight(inPodDir, "/") + "/" + filename
		copyOut, copyErr := globalExecutor.Exec(fmt.Sprintf(
			"%s cp %s/%s:%s %s 2>&1",
			kctl, shellQuote(ns), shellQuote(pod), shellQuote(remotePath), shellQuote(tmp),
		))
		if copyErr != nil {
			return copyOut, fmt.Errorf("copy backup to local restore path: %w", copyErr)
		}
		defer func() {
			_, _ = globalExecutor.Exec(fmt.Sprintf("rm -f %s 2>/dev/null || sudo rm -f %s 2>/dev/null || true",
				shellQuote(tmp), shellQuote(tmp)))
		}()
		path = tmp
	}
	return globalExecutor.Exec(fmt.Sprintf(
		`PGPASSWORD=%s pg_restore --no-password --clean --if-exists -h %s -p %d -U %s -d %s %s 2>&1`,
		shellQuote(dbPass), dbHost, dbPort, dbUser, dbName, shellQuote(path)))
}
