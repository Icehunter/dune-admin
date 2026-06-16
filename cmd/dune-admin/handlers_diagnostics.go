package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// issueRepo is the upstream repository new issues are filed against,
// regardless of which fork is running.
const issueRepo = "Icehunter/dune-admin"

// @Summary dune-admin environment summary
// @Tags diagnostics
// @Produce json
// @Success 200 {object} environmentSummary
// @Router /api/v1/diagnostics/environment [get]
func handleDiagnosticsEnvironment(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, buildEnvironment())
}

// @Summary Build a redacted GitHub issue title and body
// @Tags diagnostics
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/diagnostics/report [get]
func handleDiagnosticsReport(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		jsonErr(w, fmt.Errorf("logging not initialised"), http.StatusServiceUnavailable)
		return
	}
	title, body := buildReport(globalLogRing.Snapshot(), buildEnvironment(), 6000)
	jsonOK(w, map[string]string{"title": title, "body": body, "repo": issueRepo})
}

// @Summary Download a redacted diagnostics bundle (zip)
// @Tags diagnostics
// @Produce application/zip
// @Router /api/v1/diagnostics/bundle [get]
func handleDiagnosticsBundle(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		jsonErr(w, fmt.Errorf("logging not initialised"), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="diagnostics.zip"`)
	if err := writeDiagnosticsBundle(w, globalLogRing.Snapshot(), buildEnvironment()); err != nil {
		log.Printf("handleDiagnosticsBundle: %v", err)
	}
}

// @Summary Stream dune-admin's own logs (raw) via WebSocket
// @Tags diagnostics
// @Produce text/plain
// @Router /api/v1/diagnostics/logs/stream [get]
func handleDiagnosticsLogStream(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		http.Error(w, "logging not initialised", http.StatusServiceUnavailable)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch, cancel := globalLogRing.Subscribe()
	defer cancel()
	for _, e := range globalLogRing.Snapshot() {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(e.Line)); err != nil {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(e.Line)); err != nil {
				return
			}
		}
	}
}
