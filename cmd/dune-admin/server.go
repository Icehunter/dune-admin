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
	"slices"
	"strings"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

var allowedOrigins []string

func init() {
	raw := envOr("ALLOWED_ORIGINS", "https://dune-admin.layout.tools,http://localhost:5173")
	for o := range strings.SplitSeq(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowedOrigins = append(allowedOrigins, o)
		}
	}
}

func originAllowed(origin string) bool {
	return slices.Contains(allowedOrigins, origin)
}

// newDirectorProxy builds the /director/ reverse-proxy handler for target. It
// strips the /director prefix before forwarding and routes upstream connections
// through dial (the executor tunnel), so the director is reachable from
// wherever the executor runs rather than the dune-admin host.
func newDirectorProxy(target *url.URL, dial func(network, addr string) (net.Conn, error)) http.HandlerFunc {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = httpTransportVia(dial)
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/director")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		r.Host = target.Host
		proxy.ServeHTTP(w, r)
	}
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Dune-Server")
			// Session cookies require credentialed CORS. Safe because the
			// allow-origin value is always an exact allowlisted origin, never "*".
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildMux registers every route. API routes go through handleAPI, which
// records the capability required by the auth middleware; only the auth
// endpoints themselves and non-API content are registered directly.
func buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// ── auth (exempt from capability enforcement) ────────────────────────────
	registerAuthRoutes(mux)

	// ── servers registry ──────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/servers", capServerRead, handleListServers)
	handleAPI(mux, "GET /api/v1/servers/health", capServerRead, handleServersHealth)
	handleAPI(mux, "POST /api/v1/servers", capServerControl, handleAddServer)
	handleAPI(mux, "POST /api/v1/servers/discover", capServerControl, handleDiscoverServer)
	handleAPI(mux, "PUT /api/v1/servers/active", capServerControl, handleSetActiveServer)
	handleAPI(mux, "DELETE /api/v1/servers/{id}", capServerControl, handleDeleteServer)
	handleAPI(mux, "POST /api/v1/servers/{id}/reconnect", capServerControl, handleReconnectServer)
	handleAPI(mux, "GET /api/v1/servers/{id}/config", capServerRead, handleGetServerConfig)
	handleAPI(mux, "PUT /api/v1/servers/{id}/config", capServerControl, handleUpdateServerConfig)

	// ── status ────────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/status", capServerRead, handleStatus)
	handleAPI(mux, "POST /api/v1/reconnect", capServerControl, handleReconnect)
	handleAPI(mux, "GET /api/v1/config", capConfigRead, handleGetConfig)
	handleAPI(mux, "POST /api/v1/config", capConfigWrite, handleSaveConfig)
	handleAPI(mux, "POST /api/v1/discover", capServerControl, handleDiscover)
	handleAPI(mux, "GET /api/v1/update/check", capServerRead, handleUpdateCheck)
	handleAPI(mux, "POST /api/v1/update/apply", capServerControl, handleUpdateApply)

	// ── server settings (UserGame.ini / UserOverrides.ini) ────────────────
	handleAPI(mux, "GET /api/v1/server-settings", capConfigRead, handleGetServerSettings)
	handleAPI(mux, "PUT /api/v1/server-settings", capConfigWrite, handleUpdateServerSettings)
	handleAPI(mux, "PUT /api/v1/server-settings/raw", capConfigWrite, handleUpdateRawSection)

	// ── director config (Battlegroup Director / map persistence — AMP) ────────
	handleAPI(mux, "GET /api/v1/director-config", capConfigRead, handleGetDirectorConfig)
	handleAPI(mux, "PUT /api/v1/director-config", capConfigWrite, handleUpdateDirectorConfig)

	// ── scheduled restarts ────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/scheduled-restarts", capRestartsRead, handleGetScheduledRestarts)
	handleAPI(mux, "PUT /api/v1/scheduled-restarts", capRestartsManage, handleUpdateScheduledRestarts)
	handleAPI(mux, "POST /api/v1/scheduled-restarts/skip-next", capRestartsManage, handleSkipNextRestart)

	// Database backups (#150) — AMP-native pg_dump/restore + scheduling.
	handleAPI(mux, "GET /api/v1/db-backups", capBackupsRead, handleDBBackupList)
	handleAPI(mux, "POST /api/v1/db-backups", capBackupsManage, handleDBBackupCreate)
	handleAPI(mux, "DELETE /api/v1/db-backups", capBackupsManage, handleDBBackupDelete)
	handleAPI(mux, "GET /api/v1/db-backups/download", capBackupsRead, handleDBBackupDownload)
	handleAPI(mux, "POST /api/v1/db-backups/restore", capBackupsManage, handleDBBackupRestore)
	handleAPI(mux, "GET /api/v1/scheduled-backups", capBackupsRead, handleGetScheduledBackups)
	handleAPI(mux, "PUT /api/v1/scheduled-backups", capBackupsManage, handleUpdateScheduledBackups)

	// Web interfaces (#155) — operator-configurable links for the Server Health card.
	handleAPI(mux, "GET /api/v1/web-interfaces", capServerRead, handleGetWebInterfaces)
	handleAPI(mux, "PUT /api/v1/web-interfaces", capConfigWrite, handleUpdateWebInterfaces)

	// ── battlegroup ───────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/battlegroup/status", capServerRead, handleBGStatus)
	handleAPI(mux, "POST /api/v1/battlegroup/exec", capServerControl, handleBGExec)
	handleAPI(mux, "GET /api/v1/battlegroup/pods", capServerRead, handleBGPods)
	handleAPI(mux, "GET /api/v1/battlegroup/backup-files", capBackupsRead, handleBGBackupFiles)
	handleAPI(mux, "GET /api/v1/battlegroup/backup-files/download", capBackupsRead, handleBGBackupDownload)
	handleAPI(mux, "POST /api/v1/battlegroup/backup-files/upload", capBackupsManage, handleBGBackupUpload)
	handleAPI(mux, "POST /api/v1/battlegroup/restore", capBackupsManage, handleBGRestore)

	// ── players ───────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/players", capPlayersRead, handleGetPlayers)
	handleAPI(mux, "GET /api/v1/players/online", capPlayersRead, handleGetOnlineState)
	handleAPI(mux, "GET /api/v1/players/currency", capPlayersRead, handleGetCurrency)
	handleAPI(mux, "GET /api/v1/players/factions", capPlayersRead, handleGetFactions)
	handleAPI(mux, "GET /api/v1/players/specs", capPlayersRead, handleGetSpecs)
	handleAPI(mux, "GET /api/v1/players/summary", capPlayersRead, handleGetPlayerSummary)
	handleAPI(mux, "GET /api/v1/players/faction-trends", capPlayersRead, handleGetFactionTrends)
	handleAPI(mux, "GET /api/v1/players/templates", capPlayersRead, handleGetTemplates)
	handleAPI(mux, "GET /api/v1/players/{id}/inventory", capPlayersRead, handleGetInventory)
	handleAPI(mux, "GET /api/v1/players/{id}/journey", capPlayersRead, handleGetJourney)
	handleAPI(mux, "POST /api/v1/players/give-item", capPlayersWrite, handleGiveItem)
	handleAPI(mux, "POST /api/v1/players/give-items", capPlayersWrite, handleGiveItems)
	handleAPI(mux, "POST /api/v1/players/give-currency", capPlayersWrite, handleGiveCurrency)
	handleAPI(mux, "POST /api/v1/players/grant-live", capPlayersWrite, handleGrantLive)
	handleAPI(mux, "POST /api/v1/players/give-faction-rep", capPlayersWrite, handleGiveFactionRep)
	handleAPI(mux, "POST /api/v1/players/give-scrip", capPlayersWrite, handleGiveScrip)
	handleAPI(mux, "POST /api/v1/players/award-xp", capPlayersWrite, handleAwardXP)
	handleAPI(mux, "POST /api/v1/players/award-char-xp", capPlayersWrite, handleAwardCharXP)
	handleAPI(mux, "POST /api/v1/players/award-intel", capPlayersWrite, handleAwardIntel)
	handleAPI(mux, "POST /api/v1/players/rename", capPlayersWrite, handleRenameCharacter)
	handleAPI(mux, "POST /api/v1/players/delete", capPlayersDelete, handleDeleteCharacter)
	handleAPI(mux, "GET /api/v1/players/{id}/tags", capPlayersRead, handleGetPlayerTags)
	handleAPI(mux, "POST /api/v1/players/update-tags", capPlayersWrite, handleUpdatePlayerTags)
	handleAPI(mux, "POST /api/v1/players/returning-player-award", capPlayersWrite, handleGrantReturningPlayerAward)
	handleAPI(mux, "POST /api/v1/players/dismiss-returning-player-award", capPlayersWrite, handleDismissReturningPlayerAward)
	handleAPI(mux, "GET /api/v1/players/{id}/export", capDataExport, handleCharacterExport)
	handleAPI(mux, "POST /api/v1/players/delete-account", capPlayersWrite, handleDeleteAccount)
	handleAPI(mux, "DELETE /api/v1/players/item/{id}", capPlayersWrite, handleDeleteItem)
	handleAPI(mux, "POST /api/v1/players/reset-spec", capPlayersWrite, handleResetSpec)
	handleAPI(mux, "POST /api/v1/players/set-faction-tier", capPlayersWrite, handleSetFactionTier)
	handleAPI(mux, "POST /api/v1/players/progression-unlock", capPlayersWrite, handleProgressionUnlock)
	handleAPI(mux, "POST /api/v1/players/progression-reverse", capPlayersWrite, handleProgressionReverse)
	handleAPI(mux, "GET /api/v1/progression/presets", capPlayersRead, handleListProgressionPresets)
	handleAPI(mux, "POST /api/v1/players/progression/apply-preset", capPlayersWrite, handleApplyProgressionPreset)
	handleAPI(mux, "POST /api/v1/players/journey/complete", capPlayersWrite, handleJourneyComplete)
	handleAPI(mux, "POST /api/v1/players/journey/reset", capPlayersWrite, handleJourneyReset)
	handleAPI(mux, "POST /api/v1/players/journey/wipe", capPlayersWrite, handleJourneyWipe)
	handleAPI(mux, "POST /api/v1/players/contract/complete", capPlayersWrite, handleCompleteContract)
	handleAPI(mux, "POST /api/v1/players/contracts/complete", capPlayersWrite, handleCompleteContracts)
	handleAPI(mux, "POST /api/v1/players/contracts/reverse", capPlayersWrite, handleReverseContracts)
	handleAPI(mux, "POST /api/v1/players/grant-job-skills", capPlayersWrite, handleGrantJobSkills)
	handleAPI(mux, "POST /api/v1/players/reset-job-skills", capPlayersWrite, handleResetJobSkills)
	handleAPI(mux, "POST /api/v1/players/set-starter-class", capPlayersWrite, handleSetStarterClass)
	handleAPI(mux, "GET /api/v1/contracts", capPlayersRead, handleListContracts)
	handleAPI(mux, "POST /api/v1/players/delete-tutorials", capPlayersWrite, handleDeleteTutorials)
	handleAPI(mux, "POST /api/v1/players/wipe-codex", capPlayersWrite, handleWipeCodex)
	handleAPI(mux, "GET /api/v1/players/{id}/char-xp", capPlayersRead, handleGetCharXP)
	handleAPI(mux, "GET /api/v1/players/{id}/specs", capPlayersRead, handleGetPlayerSpecs)
	handleAPI(mux, "GET /api/v1/players/{id}/keystones", capPlayersRead, handleGetPlayerKeystones)
	handleAPI(mux, "POST /api/v1/players/grant-all-keystones", capPlayersWrite, handleGrantAllKeystones)
	handleAPI(mux, "POST /api/v1/players/reset-all-keystones", capPlayersWrite, handleResetAllKeystones)
	handleAPI(mux, "POST /api/v1/players/grant-max-spec", capPlayersWrite, handleGrantMaxSpec)
	handleAPI(mux, "GET /api/v1/players/{id}/vehicles", capPlayersRead, handleGetPlayerVehicles)
	handleAPI(mux, "POST /api/v1/players/repair-item", capPlayersWrite, handleRepairItem)
	handleAPI(mux, "POST /api/v1/players/repair-gear", capPlayersWrite, handleRepairPlayerGear)
	handleAPI(mux, "GET /api/v1/players/partitions", capPlayersRead, handleGetPartitions)
	handleAPI(mux, "POST /api/v1/players/teleport", capPlayersWrite, handleTeleportPlayer)
	handleAPI(mux, "POST /api/v1/players/teleport-coords", capPlayersWrite, handleTeleportCoords)
	handleAPI(mux, "GET /api/v1/players/{id}/position", capPlayersRead, handleGetPlayerPosition)
	handleAPI(mux, "POST /api/v1/players/teleport-to-player", capPlayersWrite, handleTeleportToPlayer)
	handleAPI(mux, "GET /api/v1/players/{id}/events", capPlayersRead, handleGetPlayerEvents)
	handleAPI(mux, "GET /api/v1/players/{id}/dungeons", capPlayersRead, handleGetPlayerDungeons)
	handleAPI(mux, "GET /api/v1/players/{id}/stats", capPlayersRead, handleGetPlayerStats)
	handleAPI(mux, "GET /api/v1/players/{id}/solaris-history", capPlayersRead, handleGetSolarisHistory)
	handleAPI(mux, "GET /api/v1/players/{id}/session-history", capPlayersRead, handleGetSessionHistory)
	handleAPI(mux, "GET /api/v1/players/{id}/stat-snapshot-history", capPlayersRead, handleGetStatSnapshotHistory)

	// ── database ──────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/database/tables", capDatabaseRead, handleDBTables)
	handleAPI(mux, "GET /api/v1/database/describe", capDatabaseRead, handleDBDescribe)
	handleAPI(mux, "GET /api/v1/database/sample", capDatabaseRead, handleDBSample)
	handleAPI(mux, "GET /api/v1/database/search", capDatabaseRead, handleDBSearch)
	handleAPI(mux, "POST /api/v1/database/sql", capDatabaseQuery, handleDBSQL)

	// ── locations (editable teleport/spawn points) ───────────────────────────
	handleAPI(mux, "GET /api/v1/locations", capWorldRead, handleListLocations)
	handleAPI(mux, "POST /api/v1/locations", capWorldWrite, handleUpsertLocation)
	handleAPI(mux, "PUT /api/v1/locations", capWorldWrite, handleRenameLocation)
	handleAPI(mux, "DELETE /api/v1/locations", capWorldWrite, handleDeleteLocation)

	// ── live map ────────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/maps", capWorldRead, handleListMaps)
	handleAPI(mux, "GET /api/v1/map/markers", capWorldRead, handleGetMapMarkers)

	// ── logs ──────────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/logs/pods", capLogsRead, handleLogPods)
	handleAPI(mux, "GET /api/v1/logs/stream", capLogsRead, handleLogStream)
	handleAPI(mux, "GET /api/v1/logs/cheats", capLogsRead, handleGetCheatLog)

	// ── notifications ────────────────────────────────────────────────────────
	handleAPI(mux, "POST /api/v1/notify", capBroadcastSend, handleNotify)

	// ── server commands (RabbitMQ, fire-and-forget) ───────────────────────────
	handleAPI(mux, "POST /api/v1/players/kick", capPlayersWrite, handleRMQKickPlayer)
	handleAPI(mux, "POST /api/v1/players/fill-water", capPlayersWrite, handleRMQFillWater)
	handleAPI(mux, "POST /api/v1/players/set-skill-points", capPlayersWrite, handleRMQSetSkillPoints)
	handleAPI(mux, "POST /api/v1/players/clean-inventory", capPlayersWrite, handleRMQCleanInventory)
	handleAPI(mux, "POST /api/v1/players/reset-progression", capPlayersWrite, handleRMQResetProgression)
	handleAPI(mux, "POST /api/v1/players/set-skill-module", capPlayersWrite, handleRMQSetSkillModule)
	handleAPI(mux, "POST /api/v1/players/give-item-live", capPlayersWrite, handleRMQGiveItem)
	handleAPI(mux, "POST /api/v1/players/cheat-script", capPlayersWrite, handleRMQCheatScript)
	handleAPI(mux, "POST /api/v1/vehicles/spawn", capServerControl, handleRMQSpawnVehicle)
	handleAPI(mux, "POST /api/v1/broadcast", capBroadcastSend, handleRMQBroadcast)
	handleAPI(mux, "POST /api/v1/broadcast/shutdown", capBroadcastSend, handleRMQBroadcastShutdown)
	handleAPI(mux, "POST /api/v1/chat/whisper", capPlayersWrite, handleRMQWhisper)
	handleAPI(mux, "GET /api/v1/players/{id}/player-ids", capPlayersRead, handlePlayerIDDebug)

	// ── storage ───────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/storage", capWorldRead, handleListStorage)
	handleAPI(mux, "GET /api/v1/storage/{id}/items", capWorldRead, handleGetStorageItems)
	handleAPI(mux, "POST /api/v1/storage/{id}/give-item", capWorldWrite, handleGiveItemToStorage)
	handleAPI(mux, "POST /api/v1/storage/{id}/give-items", capWorldWrite, handleGiveItemsToStorage)
	handleAPI(mux, "GET /api/v1/storage/{id}/owner-debug", capWorldRead, handleStorageOwnerDebug)

	// ── blueprints ────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/blueprints", capWorldRead, handleListBlueprints)
	handleAPI(mux, "GET /api/v1/blueprints/{id}/export", capDataExport, handleExportBlueprint)
	handleAPI(mux, "POST /api/v1/blueprints/import", capWorldWrite, handleImportBlueprint)

	// ── bases ─────────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/bases", capWorldRead, handleListBases)
	handleAPI(mux, "GET /api/v1/bases/{id}/export", capDataExport, handleExportBase)

	// ── guilds ──────────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/guilds", capPlayersRead, handleListGuilds)
	handleAPI(mux, "GET /api/v1/guilds/{id}", capPlayersRead, handleGetGuild)
	handleAPI(mux, "PATCH /api/v1/guilds/{id}", capPlayersWrite, handleUpdateGuild)
	handleAPI(mux, "PUT /api/v1/guilds/{id}/members/{pid}/role", capPlayersWrite, handleSetGuildMemberRole)

	// ── landsraad (read-only) ─────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/landsraad", capPlayersRead, handleGetLandsraad)

	// ── static data files (Go-first, CDN fallback on the frontend) ──────────
	handleAPI(mux, "GET /api/v1/data/{file}", capWorldRead, handleGetDataFile)

	// ── market board ─────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/market/items", capMarketRead, handleMarketItems)
	handleAPI(mux, "GET /api/v1/market/listings", capMarketRead, handleMarketListings)
	handleAPI(mux, "GET /api/v1/market/sales", capMarketRead, handleMarketSales)
	handleAPI(mux, "GET /api/v1/market/stats", capMarketRead, handleMarketStats)
	handleAPI(mux, "GET /api/v1/market/categories", capMarketRead, handleMarketCategories)
	handleAPI(mux, "GET /api/v1/market/catalog", capMarketRead, handleMarketCatalog)

	// ── market bot control ────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/market-bot/status", capMarketBotRead, handleMarketBotStatus)
	handleAPI(mux, "GET /api/v1/market-bot/config", capMarketBotRead, handleMarketBotConfig)
	handleAPI(mux, "PUT /api/v1/market-bot/config", capMarketBotManage, handleMarketBotConfig)
	handleAPI(mux, "POST /api/v1/market-bot/exec", capMarketBotManage, handleMarketBotExec)
	handleAPI(mux, "POST /api/v1/market-bot/cleanup", capMarketBotManage, handleMarketBotCleanup)
	handleAPI(mux, "GET /api/v1/market-bot/logs-ready", capMarketBotRead, handleMarketBotLogsReady)
	handleAPI(mux, "GET /api/v1/market-bot/logs", capMarketBotRead, handleMarketBotLogs)

	// ── discord ───────────────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/discord/roles", capConfigRead, handleGetDiscordRoles)
	handleAPI(mux, "GET /api/v1/discord/members/search", capConfigRead, handleSearchDiscordMembers)

	// ── welcome package ───────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/welcome-package/config", capWelcomeRead, handleGetWelcomeConfig)
	handleAPI(mux, "PUT /api/v1/welcome-package/config", capWelcomeManage, handlePutWelcomeConfig)
	handleAPI(mux, "GET /api/v1/welcome-package/grants", capWelcomeRead, handleGetWelcomeGrants)
	handleAPI(mux, "POST /api/v1/welcome-package/retry", capWelcomeManage, handleRetryWelcomeGrant)
	handleAPI(mux, "POST /api/v1/welcome-package/revoke", capWelcomeManage, handleRevokeWelcomeGrant)
	handleAPI(mux, "POST /api/v1/welcome-package/override", capWelcomeManage, handleOverrideWelcomeGrant)
	handleAPI(mux, "POST /api/v1/welcome-package/run", capWelcomeManage, handleRunWelcomePackage)

	// ── give-items packs (operator-configurable pack library) ─────────────────
	handleAPI(mux, "GET /api/v1/give-packs/config", capPlayersWrite, handleGetGivePacksConfig)
	handleAPI(mux, "PUT /api/v1/give-packs/config", capPlayersWrite, handlePutGivePacksConfig)

	// ── live events engine ────────────────────────────────────────────────────
	handleAPI(mux, "GET /api/v1/events", capEventsRead, handleListEvents)
	handleAPI(mux, "POST /api/v1/events", capEventsManage, handleCreateEvent)
	handleAPI(mux, "GET /api/v1/events/config", capEventsRead, handleGetEventsConfig)
	handleAPI(mux, "PUT /api/v1/events/config", capEventsManage, handleSaveEventsConfig)
	handleAPI(mux, "PUT /api/v1/events/{id}", capEventsManage, handleUpdateEvent)
	handleAPI(mux, "DELETE /api/v1/events/{id}", capEventsManage, handleDeleteEvent)
	handleAPI(mux, "POST /api/v1/events/{id}/enable", capEventsManage, handleSetEventEnabled)
	handleAPI(mux, "GET /api/v1/events/{id}/status", capEventsRead, handleGetEventStatus)
	handleAPI(mux, "POST /api/v1/events/{id}/reset", capEventsManage, handleResetEvent)
	handleAPI(mux, "POST /api/v1/events/{id}/claims/{account_id}/grant", capEventsManage, handleGrantEventClaim)

	// ── battlepass (intel-point reward track) ─────────────────────────────────
	handleAPI(mux, "GET /api/v1/battlepass/tiers", capBattlepassTrack, handleListBattlepassTiers)
	handleAPI(mux, "POST /api/v1/battlepass/tiers", capBattlepassManage, handleCreateBattlepassTier)
	handleAPI(mux, "PUT /api/v1/battlepass/tiers/{id}", capBattlepassManage, handleUpdateBattlepassTier)
	handleAPI(mux, "POST /api/v1/battlepass/tiers/bulk", capBattlepassManage, handleBattlepassTiersBulk)
	handleAPI(mux, "GET /api/v1/battlepass/export", capDataExport, handleExportBattlepassCatalog)
	handleAPI(mux, "POST /api/v1/battlepass/import", capBattlepassManage, handleImportBattlepassCatalog)
	handleAPI(mux, "GET /api/v1/battlepass/progress/{accountId}", capBattlepassRead, handleBattlepassProgress)
	handleAPI(mux, "GET /api/v1/battlepass/pending", capBattlepassRead, handleBattlepassPending)
	handleAPI(mux, "POST /api/v1/battlepass/reseed", capBattlepassManage, handleBattlepassReseed)
	handleAPI(mux, "POST /api/v1/battlepass/grant", capBattlepassManage, handleBattlepassGrant)
	handleAPI(mux, "POST /api/v1/battlepass/grant-tier", capBattlepassManage, handleBattlepassGrantTier)
	handleAPI(mux, "GET /api/v1/battlepass/config", capBattlepassRead, handleGetBattlepassConfig)
	handleAPI(mux, "PUT /api/v1/battlepass/config", capBattlepassManage, handleSaveBattlepassConfig)

	// ── swagger UI ────────────────────────────────────────────────────────────
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	// ── director reverse proxy (universal, opt-in) ──────────────────────────
	if loadedConfig.DirectorURL != "" {
		if target, err := url.Parse(loadedConfig.DirectorURL); err == nil {
			mux.HandleFunc("/director/", newDirectorProxy(target, dialThroughExecutor))
			log.Printf("Proxying /director/ → %s", loadedConfig.DirectorURL)
		}
	}

	// SPA frontend: prefer the embedded FS (release builds with -tags=embed),
	// then fall back to a local dist directory for dev/AMP deployments.
	if fsys := embeddedSPAFS(); fsys != nil {
		log.Println("Serving frontend from embedded assets")
		mux.Handle("/", staticCacheMiddleware(spaHandlerFS(fsys)))
	} else {
		for _, dir := range []string{"./dist", "./web/dist"} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				log.Printf("Serving frontend from %s", dir)
				mux.Handle("/", staticCacheMiddleware(spaHandler(dir)))
				break
			}
		}
	}

	return mux
}

func startServer(addr string) error {
	mux := buildMux()
	initAuthRuntime(loadedConfig)

	srv := &http.Server{
		Addr:              addr,
		Handler:           securityHeadersMiddleware(corsMiddleware(authMiddleware(mux, serverSelectorMiddleware(globalRegistry, mux)))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Minute, // backup/restore/download can take several minutes
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("dune-admin listening on %s", addr)
	// Return the error instead of log.Fatal so the deferred cleanup registered
	// in run() unwinds on shutdown; log.Fatal calls os.Exit, which skips defers.
	return srv.ListenAndServe()
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

// staticCacheMiddleware sets Cache-Control headers for static assets served by
// the SPA handler. Vite-hashed bundles and stable public assets get long-lived
// immutable caching; the HTML shell gets no-cache so new deploys are picked up.
func staticCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(p, "/art/"),
			strings.HasPrefix(p, "/theme/"),
			strings.HasPrefix(p, "/fonts/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasSuffix(p, ".svg"),
			strings.HasSuffix(p, ".png"),
			strings.HasSuffix(p, ".ico"),
			strings.HasSuffix(p, ".webp"),
			strings.HasSuffix(p, ".woff2"),
			strings.HasSuffix(p, ".woff"):
			w.Header().Set("Cache-Control", "public, max-age=604800, stale-while-revalidate=86400")
		default:
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
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
	shutdownAt, shutdownPending := pendingBroadcastShutdown()
	jsonOK(w, map[string]any{
		"executor":         executorType,
		"control":          controlName,
		"ssh_connected":    sshConnected(globalExecutor),
		"db_connected":     globalDB != nil,
		"pod_ns":           globalPodNS,
		"pod_ip":           globalPodIP,
		"ssh_host":         sshHost,
		"db_host":          dbHost,
		"version":          AppVersion,
		"commit":           GitCommit,
		"build_time":       BuildTime,
		"director_url":     loadedConfig.DirectorURL,
		"listen_addr":      loadedConfig.ListenAddr,
		"shutdown_pending": shutdownPending,
		"shutdown_at":      shutdownAt,
		"needs_setup":      needsSetup(),
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
	if a := globalRegistry.Active(); a != nil {
		invalidateServerHealth(a.ID) // connections rebuilt → drop stale health
	}
	handleStatus(w, r)
}
