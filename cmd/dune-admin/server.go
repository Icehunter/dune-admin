package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

var allowedOrigins []string

func init() {
	raw := envOr("ALLOWED_ORIGINS", "https://dune-admin.layout.tools,http://localhost:5173")
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowedOrigins = append(allowedOrigins, o)
		}
	}
}

func originAllowed(origin string) bool {
	for _, o := range allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

// originAllowedForRequest applies the explicit allowlist AND a same-host
// exception: a browser requesting from `http://172.16.12.59:9090/` against the
// dune-admin server running on the same host should not be considered cross-
// origin and never needs to be added to ALLOWED_ORIGINS.
//
// When Origin is absent (non-browser WebSocket clients), the request is allowed
// only if the TCP connection originates from a loopback address. r.RemoteAddr
// is used — not r.Host, which is a client-controlled header and can be spoofed.
func originAllowedForRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header: allow only actual loopback TCP connections.
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return false
		}
		ip := net.ParseIP(remoteHost)
		return ip != nil && ip.IsLoopback()
	}
	if u, err := url.Parse(origin); err == nil && u.Host == r.Host {
		return true
	}
	return originAllowed(origin)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Set("Vary", "Origin")
		if origin != "" && originAllowedForRequest(r) {
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
	mux.HandleFunc("GET /api/v1/config", handleGetConfig)
	mux.HandleFunc("POST /api/v1/config", handleSaveConfig)
	mux.HandleFunc("GET /api/v1/update/check", handleUpdateCheck)
	mux.HandleFunc("POST /api/v1/update/apply", handleUpdateApply)

	// ── server settings (UserGame.ini / UserOverrides.ini) ────────────────
	mux.HandleFunc("GET /api/v1/server-settings", handleGetServerSettings)
	mux.HandleFunc("PUT /api/v1/server-settings", handleUpdateServerSettings)
	mux.HandleFunc("PUT /api/v1/server-settings/raw", handleUpdateRawSection)

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
	mux.HandleFunc("POST /api/v1/players/progression-reverse", handleProgressionReverse)
	mux.HandleFunc("GET /api/v1/progression/presets", handleListProgressionPresets)
	mux.HandleFunc("POST /api/v1/players/progression/apply-preset", handleApplyProgressionPreset)
	mux.HandleFunc("POST /api/v1/players/journey/complete", handleJourneyComplete)
	mux.HandleFunc("POST /api/v1/players/journey/reset", handleJourneyReset)
	mux.HandleFunc("POST /api/v1/players/journey/wipe", handleJourneyWipe)
	mux.HandleFunc("POST /api/v1/players/contract/complete", handleCompleteContract)
	mux.HandleFunc("POST /api/v1/players/contracts/complete", handleCompleteContracts)
	mux.HandleFunc("POST /api/v1/players/contracts/reverse", handleReverseContracts)
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
	mux.HandleFunc("POST /api/v1/players/reset-all-keystones", handleResetAllKeystones)
	mux.HandleFunc("POST /api/v1/players/grant-max-spec", handleGrantMaxSpec)
	mux.HandleFunc("GET /api/v1/players/{id}/vehicles", handleGetPlayerVehicles)
	mux.HandleFunc("POST /api/v1/players/repair-item", handleRepairItem)
	mux.HandleFunc("POST /api/v1/players/repair-gear", handleRepairPlayerGear)
	mux.HandleFunc("POST /api/v1/players/repair-vehicle", handleRepairVehicle)
	mux.HandleFunc("POST /api/v1/players/refuel-vehicle", handleRefuelVehicle)
	mux.HandleFunc("GET /api/v1/players/partitions", handleGetPartitions)
	mux.HandleFunc("POST /api/v1/players/teleport", handleTeleportPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/position", handleGetPlayerPosition)
	mux.HandleFunc("POST /api/v1/players/teleport-to-player", handleTeleportToPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/events", handleGetPlayerEvents)
	mux.HandleFunc("GET /api/v1/players/{id}/dungeons", handleGetPlayerDungeons)
	mux.HandleFunc("GET /api/v1/players/{id}/stats", handleGetPlayerStats)
	mux.HandleFunc("GET /api/v1/players/{id}/solaris-history", handleGetSolarisHistory)
	mux.HandleFunc("GET /api/v1/players/{id}/session-history", handleGetSessionHistory)
	mux.HandleFunc("GET /api/v1/players/{id}/stat-snapshot-history", handleGetStatSnapshotHistory)

	// ── database ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/database/tables", handleDBTables)
	mux.HandleFunc("GET /api/v1/database/describe", handleDBDescribe)
	mux.HandleFunc("GET /api/v1/database/sample", handleDBSample)
	mux.HandleFunc("GET /api/v1/database/search", handleDBSearch)
	mux.HandleFunc("POST /api/v1/database/sql", handleDBSQL)

	// ── live map ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/map/markers", handleGetMapMarkers)

	// ── logs ──────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/logs/pods", handleLogPods)
	mux.HandleFunc("GET /api/v1/logs/stream", handleLogStream)
	mux.HandleFunc("GET /api/v1/logs/cheats", handleGetCheatLog)

	// ── notifications ────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/v1/notify", handleNotify)

	// ── server commands (RabbitMQ, fire-and-forget) ───────────────────────────
	mux.HandleFunc("POST /api/v1/players/kick", handleRMQKickPlayer)
	mux.HandleFunc("POST /api/v1/players/fill-water", handleRMQFillWater)
	mux.HandleFunc("POST /api/v1/players/set-skill-points", handleRMQSetSkillPoints)
	mux.HandleFunc("POST /api/v1/players/clean-inventory", handleRMQCleanInventory)
	mux.HandleFunc("POST /api/v1/players/reset-progression", handleRMQResetProgression)
	mux.HandleFunc("POST /api/v1/players/set-skill-module", handleRMQSetSkillModule)
	mux.HandleFunc("POST /api/v1/players/give-item-live", handleRMQGiveItem)
	mux.HandleFunc("POST /api/v1/players/cheat-script", handleRMQCheatScript)
	mux.HandleFunc("POST /api/v1/vehicles/spawn", handleRMQSpawnVehicle)
	mux.HandleFunc("POST /api/v1/broadcast", handleRMQBroadcast)
	mux.HandleFunc("POST /api/v1/broadcast/shutdown", handleRMQBroadcastShutdown)
	mux.HandleFunc("POST /api/v1/chat/whisper", handleRMQWhisper)
	mux.HandleFunc("GET /api/v1/players/{id}/player-ids", handlePlayerIDDebug)

	// ── storage ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/storage", handleListStorage)
	mux.HandleFunc("GET /api/v1/storage/{id}/items", handleGetStorageItems)
	mux.HandleFunc("POST /api/v1/storage/{id}/give-item", handleGiveItemToStorage)
	mux.HandleFunc("POST /api/v1/storage/{id}/give-items", handleGiveItemsToStorage)
	mux.HandleFunc("GET /api/v1/storage/{id}/owner-debug", handleStorageOwnerDebug)

	// ── blueprints ────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/blueprints", handleListBlueprints)
	mux.HandleFunc("GET /api/v1/blueprints/{id}/export", handleExportBlueprint)
	mux.HandleFunc("POST /api/v1/blueprints/import", handleImportBlueprint)

	// ── bases ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/bases", handleListBases)
	mux.HandleFunc("GET /api/v1/bases/{id}/export", handleExportBase)

	// ── market board ─────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/market/items", handleMarketItems)
	mux.HandleFunc("GET /api/v1/market/listings", handleMarketListings)
	mux.HandleFunc("GET /api/v1/market/sales", handleMarketSales)
	mux.HandleFunc("GET /api/v1/market/stats", handleMarketStats)
	mux.HandleFunc("GET /api/v1/market/categories", handleMarketCategories)
	mux.HandleFunc("GET /api/v1/market/catalog", handleMarketCatalog)

	// ── market bot control ────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/market-bot/status", handleMarketBotStatus)
	mux.HandleFunc("GET /api/v1/market-bot/config", handleMarketBotConfig)
	mux.HandleFunc("PUT /api/v1/market-bot/config", handleMarketBotConfig)
	mux.HandleFunc("POST /api/v1/market-bot/exec", handleMarketBotExec)
	mux.HandleFunc("POST /api/v1/market-bot/cleanup", handleMarketBotCleanup)
	mux.HandleFunc("GET /api/v1/market-bot/logs-ready", handleMarketBotLogsReady)
	mux.HandleFunc("GET /api/v1/market-bot/logs", handleMarketBotLogs)

	// ── welcome package ───────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/welcome-package/config", handleGetWelcomeConfig)
	mux.HandleFunc("PUT /api/v1/welcome-package/config", handlePutWelcomeConfig)
	mux.HandleFunc("GET /api/v1/welcome-package/grants", handleGetWelcomeGrants)
	mux.HandleFunc("POST /api/v1/welcome-package/retry", handleRetryWelcomeGrant)
	mux.HandleFunc("POST /api/v1/welcome-package/run", handleRunWelcomePackage)

	// ── swagger UI ────────────────────────────────────────────────────────────
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	// ── director reverse proxy (universal, opt-in) ──────────────────────────
	if loadedConfig.DirectorURL != "" {
		if target, err := url.Parse(loadedConfig.DirectorURL); err == nil {
			proxy := httputil.NewSingleHostReverseProxy(target)
			mux.HandleFunc("/director/", func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/director")
				if r.URL.Path == "" {
					r.URL.Path = "/"
				}
				r.Host = target.Host
				proxy.ServeHTTP(w, r)
			})
			log.Printf("Proxying /director/ → %s", loadedConfig.DirectorURL)
		}
	}

	// SPA frontend: prefer the embedded FS (release builds with -tags=embed),
	// then fall back to a local dist directory for dev/AMP deployments.
	if fsys := embeddedSPAFS(); fsys != nil {
		log.Println("Serving frontend from embedded assets")
		mux.Handle("/", spaHandlerFS(fsys))
	} else {
		for _, dir := range []string{"./dist", "./web/dist"} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				log.Printf("Serving frontend from %s", dir)
				mux.Handle("/", spaHandler(dir))
				break
			}
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
// for any path that does not match a real file (client-side routing).
func spaHandler(distDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))
	cleanDist := filepath.Clean(distDir)
	sep := string(filepath.Separator)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(cleanDist, filepath.FromSlash(r.URL.Path))
		if p != cleanDist && !strings.HasPrefix(p, cleanDist+sep) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(p); err == nil { // #nosec G703 -- path validated against cleanDist prefix above
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(cleanDist, "index.html"))
	})
}

// spaHandlerFS serves an embedded http.FileSystem as a SPA, falling back to
// index.html for any path that does not map to a real file.
//
// Note: we open index.html directly instead of routing through http.FileServer
// because FileServer always 301-redirects "/index.html" → "/" which creates an
// infinite redirect loop (ERR_TOO_MANY_REDIRECTS) in browsers.
func spaHandlerFS(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && isRegularFile(fsys, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		f, err := fsys.Open("/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()
		fi, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "index.html", fi.ModTime(), f)
	})
}

func isRegularFile(fsys http.FileSystem, path string) bool {
	f, err := fsys.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	return err == nil && !fi.IsDir()
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// handleStatus returns connection state and provider info.
//
// @Summary Return connection state and build info
// @Tags status
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/status [get]
func handleStatus(w http.ResponseWriter, r *http.Request) {
	executorType := "none"
	controlName := "none"
	if globalExecutor != nil {
		executorType = globalExecutor.Type()
	}
	if globalControl != nil {
		controlName = globalControl.Name()
	}
	jsonOK(w, map[string]any{
		"executor":      executorType,
		"control":       controlName,
		"ssh_connected": globalSSH != nil,
		"db_connected":  globalDB != nil,
		"pod_ns":        globalPodNS,
		"pod_ip":        globalPodIP,
		"ssh_host":      sshHost,
		"db_host":       dbHost,
		"version":       AppVersion,
		"commit":        GitCommit,
		"build_time":    BuildTime,
	})
}

// handleReconnect tears down and re-establishes all connections.
//
// @Summary Tear down and re-establish all backend connections
// @Tags status
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/reconnect [post]
func handleReconnect(w http.ResponseWriter, r *http.Request) {
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}
	if globalExecutor != nil {
		globalExecutor.Close()
		globalExecutor = nil
	}
	globalSSH = nil
	globalControl = nil

	if err := connectAll(); err != nil {
		jsonErr(w, err, 500)
		return
	}
	handleStatus(w, r)
}
