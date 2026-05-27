package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return originAllowedForRequest(r, true)
	},
}

// logPod is a discovered kubernetes pod available for log streaming.
type logPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

var k8sNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-\.]*[a-z0-9])?$`)

func isValidK8sName(name string) bool {
	return len(name) > 0 && len(name) <= 253 && k8sNameRe.MatchString(name)
}

func handleLogPods(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	sources, err := globalControl.ListLogSources(r.Context(), globalExecutor)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	// Convert to logPod for frontend compat.
	var pods []logPod
	for _, s := range sources {
		pods = append(pods, logPod(s))
	}
	if pods == nil {
		pods = []logPod{}
	}
	jsonOK(w, pods)
}

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	pod := r.URL.Query().Get("pod")
	if ns == "" || pod == "" {
		http.Error(w, "ns and pod required", 400)
		return
	}
	if isValidK8sName(ns) && isValidK8sName(pod) {
		// K8s names validated — safe for kubectl.
	} else if strings.ContainsAny(ns+pod, ";|&`$(){}\\") {
		http.Error(w, "invalid characters in ns or pod", 400)
		return
	}

	if globalControl == nil {
		http.Error(w, "not connected", http.StatusServiceUnavailable)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch, cancel, err := globalControl.StreamLog(r.Context(), globalExecutor, ns, pod)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error())) //nolint:errcheck
		return
	}
	defer cancel()

	for line := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func handleGetCheatLog(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchCheatLog()().(msgCheatLog)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []cheatEntry{}
	}
	jsonOK(w, rows)
}
