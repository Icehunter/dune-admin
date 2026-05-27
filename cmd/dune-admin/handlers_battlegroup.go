package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
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

func bgName() string { return strings.TrimPrefix(globalPodNS, "funcom-seabass-") }

func activeBackupDir() string {
	if backupDir != "" {
		return backupDir
	}
	// Legacy K8s default.
	return fmt.Sprintf("/funcom/artifacts/database-dumps/%s", bgName())
}

func handleBGBackupFiles(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	dir := activeBackupDir()
	out, _ := globalExecutor.Exec(fmt.Sprintf(
		`sudo ls -lt %s/ 2>/dev/null | awk '/\.backup$/{print $NF"|"$5"|"$6" "$7" "$8}'`,
		dir))
	yamlOut, _ := globalExecutor.Exec(fmt.Sprintf(
		`sudo ls %s/*.backup.yaml 2>/dev/null | xargs -r -I{} basename {} .yaml`,
		dir))
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
	dir := activeBackupDir()

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, baseName))
	w.Header().Set("Content-Type", "application/zip")

	zw := zip.NewWriter(w)
	for _, ext := range []string{".backup", ".backup.yaml"} {
		name := baseName + ext
		remotePath := dir + "/" + name
		exists, _ := globalExecutor.Exec(fmt.Sprintf("sudo test -f %s && echo yes || echo no", shellQuote(remotePath)))
		if strings.TrimSpace(exists) != "yes" {
			continue
		}
		fw, err := zw.Create(name)
		if err != nil {
			continue
		}
		if err := globalExecutor.PipeToWriter(fmt.Sprintf("sudo cat %s", shellQuote(remotePath)), fw); err != nil {
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
	dir := activeBackupDir()

	if strings.HasSuffix(filename, ".zip") {
		data, err := io.ReadAll(file)
		if err != nil {
			jsonErr(w, fmt.Errorf("read zip: %w", err), 400)
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			jsonErr(w, fmt.Errorf("invalid zip: %w", err), 400)
			return
		}
		var backupName string
		for _, zf := range zr.File {
			name := filepath.Base(zf.Name)
			if strings.ContainsAny(name, "/\\") {
				continue
			}
			if !strings.HasSuffix(name, ".backup") && !strings.HasSuffix(name, ".backup.yaml") {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			if err := globalExecutor.WriteFile(dir+"/"+name, rc); err != nil {
				_ = rc.Close()
				continue
			}
			if err := rc.Close(); err != nil {
				continue
			}
			if strings.HasSuffix(name, ".backup") && !strings.HasSuffix(name, ".yaml") {
				backupName = name
			}
		}
		if backupName == "" {
			jsonErr(w, fmt.Errorf("zip contains no .backup file"), 400)
			return
		}
		jsonOK(w, map[string]string{"name": backupName})
	} else if strings.HasSuffix(filename, ".backup") && !strings.ContainsAny(filename, "/\\") {
		if err := globalExecutor.WriteFile(dir+"/"+filename, file); err != nil {
			jsonErr(w, fmt.Errorf("upload failed: %w", err), 500)
			return
		}
		jsonOK(w, map[string]string{"name": filename})
	} else {
		jsonErr(w, fmt.Errorf("file must be .backup or .zip"), 400)
	}
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
	dir := activeBackupDir()
	path := dir + "/" + filename
	return globalExecutor.Exec(fmt.Sprintf(
		`pg_restore --clean --if-exists -h %s -p %d -U %s -d %s %s 2>&1`,
		dbHost, dbPort, dbUser, dbName, shellQuote(path)))
}
