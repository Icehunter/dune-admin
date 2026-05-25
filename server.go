package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var allowedOrigins []string

func init() {
	raw := envOr("ALLOWED_ORIGINS", "https://dune-admin.layout.tools,http://localhost:5173,http://localhost:9090")
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowedOrigins = append(allowedOrigins, o)
		}
	}
}

func originAllowed(origin string) bool {
	// Same-origin requests: if the frontend is served by this process,
	// the origin will match our listen address.
	if strings.HasSuffix(origin, listenAddr) {
		return true
	}
	for _, o := range allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Set("Vary", "Origin")
		if originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func startServer(addr string) {
	mux := http.NewServeMux()

	// ── status ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/status", handleStatus)
	mux.HandleFunc("POST /api/v1/reconnect", handleReconnect)

	// ── battlegroup ───────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/battlegroup/status", handleBGStatus)
	mux.HandleFunc("POST /api/v1/battlegroup/exec", handleBGExec)
	mux.HandleFunc("GET /api/v1/battlegroup/pods", handleBGPods)
	mux.HandleFunc("GET /api/v1/battlegroup/backup-files", handleBGBackupFiles)
	mux.HandleFunc("GET /api/v1/battlegroup/backup-files/download", handleBGBackupDownload)
	mux.HandleFunc("POST /api/v1/battlegroup/backup-files/upload", handleBGBackupUpload)
	mux.HandleFunc("POST /api/v1/battlegroup/restore", handleBGRestore)

	// ── players ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/players", handleGetPlayers)
	mux.HandleFunc("GET /api/v1/players/online", handleGetOnlineState)
	mux.HandleFunc("GET /api/v1/players/currency", handleGetCurrency)
	mux.HandleFunc("GET /api/v1/players/factions", handleGetFactions)
	mux.HandleFunc("GET /api/v1/players/specs", handleGetSpecs)
	mux.HandleFunc("GET /api/v1/players/templates", handleGetTemplates)
	mux.HandleFunc("GET /api/v1/players/{id}/inventory", handleGetInventory)
	mux.HandleFunc("GET /api/v1/players/{id}/journey", handleGetJourney)
	mux.HandleFunc("POST /api/v1/players/give-item", handleGiveItem)
	mux.HandleFunc("POST /api/v1/players/give-items", handleGiveItems)
	mux.HandleFunc("POST /api/v1/players/give-currency", handleGiveCurrency)
	mux.HandleFunc("POST /api/v1/players/grant-live", handleGrantLive)
	mux.HandleFunc("POST /api/v1/players/give-faction-rep", handleGiveFactionRep)
	mux.HandleFunc("POST /api/v1/players/give-scrip", handleGiveScrip)
	mux.HandleFunc("POST /api/v1/players/award-xp", handleAwardXP)
	mux.HandleFunc("POST /api/v1/players/award-char-xp", handleAwardCharXP)
	mux.HandleFunc("POST /api/v1/players/award-intel", handleAwardIntel)
	mux.HandleFunc("POST /api/v1/players/rename", handleRenameCharacter)
	mux.HandleFunc("GET /api/v1/players/{id}/tags", handleGetPlayerTags)
	mux.HandleFunc("POST /api/v1/players/update-tags", handleUpdatePlayerTags)
	mux.HandleFunc("POST /api/v1/players/returning-player-award", handleGrantReturningPlayerAward)
	mux.HandleFunc("POST /api/v1/players/dismiss-returning-player-award", handleDismissReturningPlayerAward)
	mux.HandleFunc("GET /api/v1/players/{id}/export", handleCharacterExport)
	mux.HandleFunc("POST /api/v1/players/delete-account", handleDeleteAccount)
	mux.HandleFunc("DELETE /api/v1/players/item/{id}", handleDeleteItem)
	mux.HandleFunc("POST /api/v1/players/reset-spec", handleResetSpec)
	mux.HandleFunc("POST /api/v1/players/set-faction-tier", handleSetFactionTier)
	mux.HandleFunc("POST /api/v1/players/progression-unlock", handleProgressionUnlock)
	mux.HandleFunc("GET /api/v1/progression/presets", handleListProgressionPresets)
	mux.HandleFunc("POST /api/v1/players/progression/apply-preset", handleApplyProgressionPreset)
	mux.HandleFunc("GET /api/v1/server-settings", handleGetServerSettings)
	mux.HandleFunc("PUT /api/v1/server-settings", handleUpdateServerSettings)
	mux.HandleFunc("POST /api/v1/players/journey/complete", handleJourneyComplete)
	mux.HandleFunc("POST /api/v1/players/journey/reset", handleJourneyReset)
	mux.HandleFunc("POST /api/v1/players/journey/wipe", handleJourneyWipe)
	mux.HandleFunc("POST /api/v1/players/contract/complete", handleCompleteContract)
	mux.HandleFunc("POST /api/v1/players/contracts/complete", handleCompleteContracts)
	mux.HandleFunc("POST /api/v1/players/grant-job-skills", handleGrantJobSkills)
	mux.HandleFunc("POST /api/v1/players/reset-job-skills", handleResetJobSkills)
	mux.HandleFunc("POST /api/v1/players/set-starter-class", handleSetStarterClass)
	mux.HandleFunc("GET /api/v1/contracts", handleListContracts)
	mux.HandleFunc("POST /api/v1/players/delete-tutorials", handleDeleteTutorials)
	mux.HandleFunc("POST /api/v1/players/wipe-codex", handleWipeCodex)
	mux.HandleFunc("GET /api/v1/players/{id}/char-xp", handleGetCharXP)
	mux.HandleFunc("GET /api/v1/players/{id}/specs", handleGetPlayerSpecs)
	mux.HandleFunc("GET /api/v1/players/{id}/keystones", handleGetPlayerKeystones)
	mux.HandleFunc("POST /api/v1/players/grant-all-keystones", handleGrantAllKeystones)
	mux.HandleFunc("POST /api/v1/players/grant-max-spec", handleGrantMaxSpec)
	mux.HandleFunc("GET /api/v1/players/{id}/vehicles", handleGetPlayerVehicles)
	mux.HandleFunc("POST /api/v1/players/repair-item", handleRepairItem)
	mux.HandleFunc("GET /api/v1/players/partitions", handleGetPartitions)
	mux.HandleFunc("POST /api/v1/players/teleport", handleTeleportPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/events", handleGetPlayerEvents)
	mux.HandleFunc("GET /api/v1/players/{id}/dungeons", handleGetPlayerDungeons)

	// ── database ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/database/tables", handleDBTables)
	mux.HandleFunc("GET /api/v1/database/describe", handleDBDescribe)
	mux.HandleFunc("GET /api/v1/database/sample", handleDBSample)
	mux.HandleFunc("GET /api/v1/database/search", handleDBSearch)
	mux.HandleFunc("POST /api/v1/database/sql", handleDBSQL)

	// ── logs ──────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/logs/pods", handleLogPods)
	mux.HandleFunc("GET /api/v1/logs/stream", handleLogStream)
	mux.HandleFunc("GET /api/v1/logs/cheats", handleGetCheatLog)

	// ── notifications ────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/v1/notify", handleNotify)

	// ── storage ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/storage", handleListStorage)
	mux.HandleFunc("GET /api/v1/storage/{id}/items", handleGetStorageItems)
	mux.HandleFunc("POST /api/v1/storage/{id}/give-item", handleGiveItemToStorage)
	mux.HandleFunc("POST /api/v1/storage/{id}/give-items", handleGiveItemsToStorage)

	// ── blueprints ────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/blueprints", handleListBlueprints)
	mux.HandleFunc("GET /api/v1/blueprints/{id}/export", handleExportBlueprint)
	mux.HandleFunc("POST /api/v1/blueprints/import", handleImportBlueprint)

	// ── bases ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/bases", handleListBases)
	mux.HandleFunc("GET /api/v1/bases/{id}/export", handleExportBase)

	// ── director proxy (direct mode) ─────────────────────────────────────────
	if connectionMode == "direct" && directorURL != "" {
		if target, err := url.Parse(directorURL); err == nil {
			proxy := httputil.NewSingleHostReverseProxy(target)
			mux.HandleFunc("/director/", func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/director")
				if r.URL.Path == "" {
					r.URL.Path = "/"
				}
				r.Host = target.Host
				proxy.ServeHTTP(w, r)
			})
			log.Printf("Proxying /director/ → %s", directorURL)
		}
	}

	// ── frontend (SPA) ───────────────────────────────────────────────────────
	for _, dir := range []string{"./dist", "./web/dist"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			log.Printf("Serving frontend from %s", dir)
			mux.Handle("/", spaHandler(dir))
			break
		}
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Minute, // backup/restore/download can take several minutes
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("dune-admin listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// spaHandler serves static files from distDir, falling back to index.html
// for any path that doesn't match a real file (client-side routing).
func spaHandler(distDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(distDir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
	})
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// handleStatus returns SSH and DB connection state.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{
		"ssh_connected":   globalSSH != nil,
		"db_connected":    globalDB != nil,
		"connection_mode": connectionMode,
		"pod_ns":          globalPodNS,
		"pod_ip":          globalPodIP,
		"ssh_host":        sshHost,
		"version":         version,
	})
}

// requireSSH returns false and writes an error response if the current
// connection mode does not support SSH-dependent operations.
func requireSSH(w http.ResponseWriter) bool {
	if connectionMode == "direct" {
		jsonErr(w, fmt.Errorf("not available in direct connection mode"), http.StatusNotImplemented)
		return false
	}
	if globalSSH == nil {
		jsonErr(w, fmt.Errorf("SSH not connected"), http.StatusServiceUnavailable)
		return false
	}
	return true
}

// handleReconnect attempts to re-establish SSH+DB connections.
func handleReconnect(w http.ResponseWriter, r *http.Request) {
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}
	if connectionMode != "direct" && globalSSH != nil {
		globalSSH.Close()
		globalSSH = nil
	}
	msg, ok := cmdConnect().(msgConnect)
	if !ok || msg.err != nil {
		var errMsg string
		if msg.err != nil {
			errMsg = msg.err.Error()
		}
		jsonErr(w, fmt.Errorf("%s", errMsg), 500)
		return
	}
	handleStatus(w, r)
}
