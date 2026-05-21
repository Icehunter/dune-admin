package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return originAllowed(r.Header.Get("Origin")) },
}

type logPod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func handleLogPods(w http.ResponseWriter, r *http.Request) {
	out, err := sshExec(fmt.Sprintf(
		"%s %s %s %s %s %s %s %s",
		"sudo", "kubectl", "get", "pods", "-n", globalPodNS, "--no-headers", "-o custom-columns=NAME:.metadata.name 2>&1"))
	if err != nil {
		jsonErr(w, fmt.Errorf("kubectl: %w", err), 500)
		return
	}
	out2, _ := sshExec("sudo kubectl get pods -n funcom-operators --no-headers -o custom-columns=NAME:.metadata.name 2>&1")

	var pods []logPod
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name != "" && isValidK8sName(name) && !strings.Contains(name, "db-dbdepl") {
			pods = append(pods, logPod{Namespace: globalPodNS, Name: name})
		}
	}
	for _, line := range splitLines(out2) {
		name := strings.TrimSpace(line)
		if name != "" && isValidK8sName(name) {
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
	if !isAllowedLogTarget(ns, pod) {
		http.Error(w, "invalid log stream target", 400)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cmd := strings.Join([]string{"sudo", "kubectl", "logs", "-f", "-n", ns, pod}, " ") + " 2>&1"
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
