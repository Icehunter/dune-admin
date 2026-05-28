package marketbot

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// APIServer handles HTTP requests for bot status and config.
type APIServer struct {
	config    *Config
	ex        *Exchange
	token     string
	startTime time.Time
	mux       *http.ServeMux
}

func newAPIServer(cfg *Config, ex *Exchange, token string) *APIServer {
	s := &APIServer{
		config:    cfg,
		ex:        ex,
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

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ex.statusSnapshot(s.startTime))
}

func (s *APIServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
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

func (s *APIServer) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "no file config to reload"})
}

func (s *APIServer) handleReport(w http.ResponseWriter, r *http.Request) {
	rows := s.ex.reportData(r.Context())
	writeJSON(w, http.StatusOK, rows)
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
