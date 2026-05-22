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
		return originAllowed(r.Header.Get("Origin"))
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
	out, err := sshExec(fmt.Sprintf(
		"sudo kubectl get pods -n %s --no-headers -o custom-columns=NAME:.metadata.name 2>&1", globalPodNS))
	if err != nil {
		jsonErr(w, fmt.Errorf("kubectl: %w", err), 500)
		return
	}
	// Also get funcom-operators namespace pods
	out2, _ := sshExec(
		"sudo kubectl get pods -n funcom-operators --no-headers -o custom-columns=NAME:.metadata.name 2>&1")

	var pods []logPod
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name != "" && !strings.Contains(name, "db-dbdepl") {
			pods = append(pods, logPod{Namespace: globalPodNS, Name: name})
		}
	}
	for _, line := range splitLines(out2) {
		name := strings.TrimSpace(line)
		if name != "" {
			pods = append(pods, logPod{Namespace: "funcom-operators", Name: name})
		}
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
	if !isValidK8sName(ns) || !isValidK8sName(pod) {
		http.Error(w, "invalid ns or pod name", 400)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Time{})

	cmd := fmt.Sprintf("sudo kubectl logs -f -n %s %s 2>&1", ns, pod)
	ch, cancel, err := sshStream(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
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
