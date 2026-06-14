package main

import (
	"fmt"
	"net/http"
	"sync"
)

// capability is a permission group covering a set of API routes. Discord
// roles map to capability sets in the permissions matrix; every /api/v1
// route is assigned exactly one capability at registration time.
type capability string

const (
	capPlayersRead      capability = "players:read"
	capPlayersWrite     capability = "players:write"
	capPlayersDelete    capability = "players:delete"
	capWorldRead        capability = "world:read"
	capWorldWrite       capability = "world:write"
	capDataExport       capability = "data:export"
	capServerRead       capability = "server:read"
	capServerControl    capability = "server:control"
	capBroadcastSend    capability = "broadcast:send"
	capRestartsRead     capability = "restarts:read"
	capRestartsManage   capability = "restarts:manage"
	capConfigRead       capability = "config:read"
	capConfigWrite      capability = "config:write"
	capMarketRead       capability = "market:read"
	capMarketBotRead    capability = "market-bot:read"
	capMarketBotManage  capability = "market-bot:manage"
	capEventsRead       capability = "events:read"
	capEventsManage     capability = "events:manage"
	capWelcomeRead      capability = "welcome:read"
	capWelcomeManage    capability = "welcome:manage"
	capBattlepassTrack  capability = "battlepass:track"
	capBattlepassRead   capability = "battlepass:read"
	capBattlepassManage capability = "battlepass:manage"
	capLogsRead         capability = "logs:read"
	capDatabaseRead     capability = "database:read"
	capDatabaseQuery    capability = "database:query"
	capBackupsRead      capability = "backups:read"
	capBackupsManage    capability = "backups:manage"
	capAuthManage       capability = "auth:manage"
)

// allCapabilities is the authoritative capability set, used to validate the
// permissions matrix and to render the Permissions tab.
var allCapabilities = map[capability]string{
	capPlayersRead:      "View players, guilds, contracts, and progression",
	capPlayersWrite:     "Modify players: items, currency, XP, teleport, kick, guild edits",
	capPlayersDelete:    "Permanently delete characters from the server (irreversible)",
	capWorldRead:        "View storage, blueprints, bases, maps, and locations",
	capWorldWrite:       "Modify storage contents, import blueprints, edit locations",
	capDataExport:       "Export characters, blueprints, bases, and battlepass catalogs",
	capServerRead:       "View server status, processes, and version info",
	capServerControl:    "Start/stop/restart the server, apply updates, spawn vehicles",
	capBroadcastSend:    "Send broadcasts, shutdown warnings, and notifications",
	capRestartsRead:     "View the scheduled restart configuration",
	capRestartsManage:   "Edit scheduled restarts and skip the next restart",
	capConfigRead:       "View dune-admin and server configuration",
	capConfigWrite:      "Edit dune-admin and server configuration",
	capMarketRead:       "View market listings, sales, and stats",
	capMarketBotRead:    "View market bot status, configuration, and logs",
	capMarketBotManage:  "Control the market bot and edit its configuration",
	capEventsRead:       "View live events and their status",
	capEventsManage:     "Create, edit, and reset live events",
	capWelcomeRead:      "View welcome kit configuration and grant history",
	capWelcomeManage:    "Edit welcome kits, retry/revoke grants",
	capBattlepassTrack:  "View the battlepass reward track",
	capBattlepassRead:   "View battlepass progress, pending grants, and config",
	capBattlepassManage: "Edit battlepass tiers, grant rewards, import catalogs",
	capLogsRead:         "Stream server logs and view cheat logs",
	capDatabaseRead:     "Browse database tables and run searches",
	capDatabaseQuery:    "Run read-only SQL queries",
	capBackupsRead:      "List and download backups",
	capBackupsManage:    "Create, restore, and delete backups",
	capAuthManage:       "Manage the permissions matrix and local users",
}

// Pseudo-role keys usable in the permissions matrix alongside Discord role
// IDs: "guest" extends anonymous guest sessions, "default" extends every
// authenticated non-owner session.
const (
	pseudoRoleGuest   = "guest"
	pseudoRoleDefault = "default"
)

// routeCapabilities maps every registered route pattern to its capability.
// Populated exclusively by handleAPI so a route cannot exist without one.
var (
	routeCapabilitiesMu sync.RWMutex
	routeCapabilities   = map[string]capability{}
)

// handleAPI registers an API route on mux and records its capability.
// Panics on an empty capability or a conflicting re-registration — both are
// programmer errors that must fail at startup, not at request time.
func handleAPI(mux *http.ServeMux, pattern string, cap capability, h http.HandlerFunc) {
	if cap == "" {
		panic(fmt.Sprintf("handleAPI: route %q registered without a capability", pattern))
	}
	if _, ok := allCapabilities[cap]; !ok {
		panic(fmt.Sprintf("handleAPI: route %q uses unknown capability %q", pattern, cap))
	}
	routeCapabilitiesMu.Lock()
	if existing, ok := routeCapabilities[pattern]; ok && existing != cap {
		routeCapabilitiesMu.Unlock()
		panic(fmt.Sprintf("handleAPI: route %q already mapped to %q, refusing %q", pattern, existing, cap))
	}
	routeCapabilities[pattern] = cap
	routeCapabilitiesMu.Unlock()
	mux.HandleFunc(pattern, h)
}

// capabilityForRequest resolves the capability required for a request by
// recovering the matched route pattern from the mux. Returns false when the
// route is not in the capability table (fail closed at the caller).
func capabilityForRequest(mux *http.ServeMux, r *http.Request) (capability, bool) {
	_, pattern := mux.Handler(r)
	if pattern == "" {
		return "", false
	}
	routeCapabilitiesMu.RLock()
	defer routeCapabilitiesMu.RUnlock()
	cap, ok := routeCapabilities[pattern]
	return cap, ok
}

// No capability is granted implicitly: a session has exactly what the
// permissions matrix (and, for local DB users, their assigned list) maps to
// its roles. Absence of a capability is denial. The only carve-outs are the
// /api/v1/auth/* endpoints (exempt so login works) and the bare
// /api/v1/status heartbeat the SPA shell polls.
