package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// directorConfigStore is implemented by control planes that expose the
// Battlegroup Director config file (AMP). Reads return the file path + content;
// writes rewrite it. Under AMP the target is $STATE/director_config.ini, which
// prestart.sh seeds from the Funcom template only when absent and then copies
// into the runtime conf dir on EVERY start ("so director picks up any admin
// edits") — so edits persist and take effect on the next instance restart.
type directorConfigStore interface {
	readDirectorConfig(exec Executor) (path, content string, err error)
	writeDirectorConfig(exec Executor, content string) (path string, err error)
}

// directorReadOnlySections are infrastructure wiring: launched values are
// overridden by env/CLI in start-director.sh (Database_*, --RMQ*Hostname) and/or
// contain secrets, so editing them in the ini has no effect — surfaced read-only.
var directorReadOnlySections = map[string]bool{
	"Database": true, "RMQAdmin": true, "RMQGame": true,
}

func isDirectorSecretKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "password") || strings.Contains(k, "secret") || strings.Contains(k, "token")
}

type directorKV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
	Secret  bool   `json:"secret,omitempty"`
}

type directorSection struct {
	Name     string       `json:"name"`
	ReadOnly bool         `json:"read_only"`
	Lines    []directorKV `json:"lines"`
}

// parseDirectorINI parses director_config.ini into ordered sections, splitting
// each "key = value ;; comment" line into its parts. Section headers look like
// "[ Battlegroup ]". Whole-line comments (';', '#') and blanks are skipped.
// Secret values are blanked so they never reach the client.
func parseDirectorINI(content string) []directorSection {
	sections := []directorSection{}
	cur := -1
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			sections = append(sections, directorSection{Name: name, ReadOnly: directorReadOnlySections[name], Lines: []directorKV{}})
			cur = len(sections) - 1
			continue
		}
		if cur < 0 {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		kv := splitDirectorLine(line, eq)
		sections[cur].Lines = append(sections[cur].Lines, kv)
	}
	return sections
}

func splitDirectorLine(line string, eq int) directorKV {
	key := strings.TrimSpace(line[:eq])
	value, comment := line[eq+1:], ""
	if c := strings.Index(value, ";;"); c >= 0 {
		comment = strings.TrimSpace(strings.TrimLeft(value[c:], "; "))
		value = value[:c]
	}
	kv := directorKV{Key: key, Value: strings.TrimSpace(value), Comment: comment, Secret: isDirectorSecretKey(key)}
	if kv.Secret {
		kv.Value = ""
	}
	return kv
}

// applyDirectorEdits rewrites the file, replacing the value of edited keys within
// their section while preserving the key part, inline ';;' comments, ordering,
// and all non-edited lines. edits is section -> key -> new value. Read-only
// sections and secret keys are never written (double-guarded).
func applyDirectorEdits(content string, edits map[string]map[string]string) string {
	lines := strings.Split(content, "\n")
	curSec := ""
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			curSec = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		secEdits, ok := edits[curSec]
		if !ok || directorReadOnlySections[curSec] || trimmed == "" ||
			strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(raw, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(raw[:eq])
		newVal, has := secEdits[key]
		if !has || isDirectorSecretKey(key) {
			continue
		}
		comment := ""
		if c := strings.Index(raw[eq+1:], ";;"); c >= 0 {
			comment = " " + strings.TrimSpace(raw[eq+1+c:])
		}
		lines[i] = raw[:eq+1] + newVal + comment
	}
	return strings.Join(lines, "\n")
}

func directorStore() (directorConfigStore, bool) {
	if globalControl == nil || globalExecutor == nil {
		return nil, false
	}
	store, ok := globalControl.(directorConfigStore)
	return store, ok
}

// @Summary Read the Battlegroup Director config (AMP only)
// @Tags director
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 501 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/director-config [get]
func handleGetDirectorConfig(w http.ResponseWriter, _ *http.Request) {
	if globalControl == nil || globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), http.StatusServiceUnavailable)
		return
	}
	store, ok := directorStore()
	if !ok {
		jsonErr(w, fmt.Errorf("director config is only available on the AMP control plane"), http.StatusNotImplemented)
		return
	}
	path, content, err := store.readDirectorConfig(globalExecutor)
	if err != nil {
		log.Printf("handleGetDirectorConfig: %v", err)
		jsonErr(w, fmt.Errorf("could not read director config"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"path": path, "sections": parseDirectorINI(content)})
}

// @Summary Update the Battlegroup Director config (AMP only)
// @Tags director
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 501 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/director-config [put]
func handleUpdateDirectorConfig(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil || globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), http.StatusServiceUnavailable)
		return
	}
	store, ok := directorStore()
	if !ok {
		jsonErr(w, fmt.Errorf("director config is only available on the AMP control plane"), http.StatusNotImplemented)
		return
	}
	var body struct {
		Updates map[string]map[string]string `json:"updates"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if len(body.Updates) == 0 {
		jsonErr(w, fmt.Errorf("no updates provided"), http.StatusBadRequest)
		return
	}
	_, content, err := store.readDirectorConfig(globalExecutor)
	if err != nil {
		log.Printf("handleUpdateDirectorConfig: read: %v", err)
		jsonErr(w, fmt.Errorf("could not read director config"), http.StatusInternalServerError)
		return
	}
	path, err := store.writeDirectorConfig(globalExecutor, applyDirectorEdits(content, body.Updates))
	if err != nil {
		log.Printf("handleUpdateDirectorConfig: write: %v", err)
		jsonErr(w, fmt.Errorf("could not write director config"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "director config updated — restart the server to apply", "path": path})
}
