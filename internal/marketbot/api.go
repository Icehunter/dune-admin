package marketbot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// APIServer handles HTTP requests for bot status and config.
type APIServer struct {
	config    *Config
	ex        *Exchange
	inst      *Instance // optional; required for /exec, /cleanup, /logs
	token     string
	startTime time.Time
	mux       *http.ServeMux
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func newAPIServer(cfg *Config, ex *Exchange, inst *Instance, token string) *APIServer {
	s := &APIServer{
		config:    cfg,
		ex:        ex,
		inst:      inst,
		token:     token,
		startTime: time.Now(),
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /status", s.auth(s.handleStatus))
	s.mux.HandleFunc("GET /config", s.auth(s.handleGetConfig))
	s.mux.HandleFunc("PUT /config", s.auth(s.handlePutConfig))
	s.mux.HandleFunc("POST /config/reload", s.auth(s.handleConfigReload))
	s.mux.HandleFunc("GET /report", s.auth(s.handleReport))
	s.mux.HandleFunc("POST /exec", s.auth(s.handleExec))
	s.mux.HandleFunc("POST /cleanup", s.auth(s.handleCleanup))
	s.mux.HandleFunc("GET /logs-ready", s.auth(s.handleLogsReady))
	s.mux.HandleFunc("GET /logs", s.auth(s.handleLogs))
	return s
}

func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *APIServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		hdr := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(hdr, "Bearer ")
		if !strings.HasPrefix(hdr, "Bearer ") || subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: writeJSON encode error: %v", err)
	}
}

func (s *APIServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *APIServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.ex.statusSnapshot(s.startTime))
}

func (s *APIServer) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.config)
}

func (s *APIServer) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.config.Apply(patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "changes apply on next tick"})
}

func (s *APIServer) handleConfigReload(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "no file config to reload"})
}

func (s *APIServer) handleReport(w http.ResponseWriter, r *http.Request) {
	rows := s.ex.reportData(r.Context())
	writeJSON(w, http.StatusOK, rows)
}

var execAllowlist = map[string]bool{
	"start": true, "stop": true, "restart": true,
}

func (s *APIServer) handleExec(w http.ResponseWriter, r *http.Request) {
	if s.inst == nil {
		http.Error(w, "instance not available", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !execAllowlist[req.Cmd] {
		http.Error(w, "unknown command", http.StatusBadRequest)
		return
	}
	var output string
	switch req.Cmd {
	case "start":
		s.inst.Resume()
		output = "resumed"
	case "stop":
		s.inst.Pause()
		output = "paused"
	case "restart":
		if err := s.inst.Restart(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		output = "restarted"
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (s *APIServer) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if s.inst == nil {
		http.Error(w, "instance not available", http.StatusServiceUnavailable)
		return
	}
	orders, items, err := s.inst.CleanupListings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{
		"orders_deleted": orders,
		"items_deleted":  items,
	})
}

func (s *APIServer) handleLogsReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": true, "mode": "remote"})
}

func (s *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.inst == nil {
		http.Error(w, "instance not available", http.StatusServiceUnavailable)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch := s.inst.Sink.Subscribe()
	defer s.inst.Sink.Unsubscribe(ch)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Pump client pings so we detect disconnects.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				return
			}
		}
	}
}

// ListenAndServe starts the HTTP server on addr. Blocks until the server stops.
func (s *APIServer) ListenAndServe(addr string) {
	if s.token == "" {
		log.Printf("api: WARNING: no API token configured — all authenticated endpoints are disabled")
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("api: listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("api: %v", err)
	}
}
