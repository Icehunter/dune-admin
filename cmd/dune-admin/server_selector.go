package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// contextKey is an unexported type for request-context keys to avoid collisions.
type contextKey int

const serverContextKey contextKey = 1

// serverSelectorMiddleware reads the optional X-Dune-Server request header and
// stashes the matching ServerContext in the request context. When the header is
// absent the active server from reg is used. When the header names an unknown
// server the request is rejected with 404.
func serverSelectorMiddleware(reg *serverRegistry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Dune-Server")
		var sc *ServerContext
		if id != "" {
			sc = reg.Get(id)
			if sc == nil {
				jsonErr(w, fmt.Errorf("server %q not found", id), http.StatusNotFound)
				return
			}
		} else {
			sc = reg.Active() // may be nil for empty registry
		}
		if sc != nil {
			r = r.WithContext(context.WithValue(r.Context(), serverContextKey, sc))
		}
		next.ServeHTTP(w, r)
	})
}

// serverFromCtx retrieves the ServerContext stashed by serverSelectorMiddleware.
// Returns nil if no server was stashed (empty registry or middleware not in chain).
func serverFromCtx(r *http.Request) *ServerContext {
	sc, _ := r.Context().Value(serverContextKey).(*ServerContext)
	return sc
}

// scopeFromReq returns the per-server cache/scope key for the request, or
// "default" when unscoped (legacy single-server / no server context). Used to
// key per-server caches by server first, then the query.
func scopeFromReq(r *http.Request) string {
	if sc := serverFromCtx(r); sc != nil {
		return sc.ID
	}
	return "default"
}

// dbFromCtx returns the pgx pool for the request's server context. When no
// server is stashed (middleware not in chain, or empty registry) it falls back
// to globalDB so existing tests and legacy call-sites keep working during the
// Phase-3 incremental conversion. Phase 6 removes the fallback.
func dbFromCtx(r *http.Request) *pgxpool.Pool {
	if sc := serverFromCtx(r); sc != nil {
		return sc.DB
	}
	return globalDB
}

// storeScopeFromCtx returns the SQLite server_id scope (servers.id) for the
// request's server context. Falls back to the default server id so callers that
// skip the middleware are safe.
func storeScopeFromCtx(r *http.Request) int {
	sc := serverFromCtx(r)
	if sc == nil {
		return defaultServerID
	}
	return sc.StoreScope
}

// controlFromCtx returns the ControlPlane for the request's server context.
// Falls back to globalControl during the Phase-3 incremental conversion.
// Phase 6 removes the fallback.
func controlFromCtx(r *http.Request) ControlPlane {
	if sc := serverFromCtx(r); sc != nil {
		return sc.Control
	}
	return globalControl
}

// executorFromCtx returns the Executor for the request's server context.
// Falls back to globalExecutor during the Phase-3 incremental conversion.
// Phase 6 removes the fallback.
func executorFromCtx(r *http.Request) Executor {
	if sc := serverFromCtx(r); sc != nil {
		return sc.Executor
	}
	return globalExecutor
}

// ── /api/v1/servers handlers ─────────────────────────────────────────────────

// serverListItem is the JSON shape returned by GET /api/v1/servers. The id is
// the numeric server id (real JSON number).
type serverListItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// handleListServers returns all registered servers and marks the active one.
func handleListServers(w http.ResponseWriter, _ *http.Request) {
	activeID := globalRegistry.ActiveID()
	all := globalRegistry.All()
	items := make([]serverListItem, 0, len(all))
	for _, sc := range all {
		items = append(items, serverListItem{
			ID:     sc.Cfg.ID,
			Name:   sc.Name,
			Active: sc.ID == activeID,
		})
	}
	jsonOK(w, items)
}

// handleSetActiveServer switches the process-wide active server and persists the
// choice so it survives restart. Body: {"id":<numericServerID>}
func handleSetActiveServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.ID == 0 {
		jsonErr(w, fmt.Errorf("id is required"), http.StatusBadRequest)
		return
	}
	scope := serverScope(body.ID)
	if err := globalRegistry.SetActive(scope); err != nil {
		jsonErr(w, err, http.StatusNotFound)
		return
	}
	// Update global aliases so background workers and legacy callers see the switch.
	if active := globalRegistry.Active(); active != nil {
		globalDB = active.DB
		globalControl = active.Control
		globalExecutor = active.Executor
	}
	persistActiveServer(scope)
	jsonOK(w, map[string]int{"active": body.ID})
}

// handleDeleteServer removes a server from the registry, purges its stored data,
// and persists the updated config. Deleting the active server is allowed: the
// registry reassigns active to the next server (and the global aliases follow).
// Deleting the last remaining server clears the flat connection config so the
// SPA returns to the Setup Wizard. The destructive confirm dialog in the UI is
// the guard against accidental deletion.
func handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id, scope, err := parseServerID(r)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	// Stop the server's market bot before removing it so its goroutines and DB
	// references are released.
	stopServerMarketBot(globalRegistry.Get(scope))
	if !globalRegistry.Remove(scope) {
		jsonErr(w, fmt.Errorf("server %d not found", id), http.StatusNotFound)
		return
	}
	// DB is the source of truth: drop the row; the server_id FK ON DELETE CASCADE
	// purges every scoped child row automatically (welcome/give/event/battlepass/
	// sessions/discord-status, and the server's discord_servers link +
	// discord_user_links). Guilds themselves are NOT deleted — a guild can still
	// hold other servers.
	if globalServersStore != nil {
		if derr := globalServersStore.deleteServer(id); derr != nil {
			componentLog("server_selector").Error().Err(derr).Int("server_id", id).Msg("delete server failed")
		}
	}

	reassignActiveAfterDelete()
	removeServerFromMirror(id)
	invalidateServerHealth(scope) // drop the removed server's cached health

	// Refresh status loops: the deleted server's discord_servers link is gone via
	// FK cascade, so its loop must stop. Slash commands stay (guilds are not
	// deleted by a server delete).
	applyDiscordStatusLoops()

	jsonOK(w, map[string]bool{"deleted": true})
}

// reassignActiveAfterDelete re-points the global aliases at the new active
// server, or tears them down when no server remains so background workers stop
// using a dead pool. The active selection is persisted to the store.
func reassignActiveAfterDelete() {
	if active := globalRegistry.Active(); active != nil {
		globalDB = active.DB
		globalControl = active.Control
		globalExecutor = active.Executor
		persistActiveServer(active.ID)
		return
	}
	resetRuntimeConnections()
	persistActiveServer("")
}

// persistActiveServer records the active server's scope in the store so the
// choice survives a restart. No-op when the store is unavailable.
func persistActiveServer(scope string) {
	if globalStore == nil {
		return
	}
	if err := metaSet(globalStore, activeServerMetaKey, scope); err != nil {
		componentLog("server_selector").Error().Err(err).Str("scope", scope).Msg("persist active server failed")
	}
}

// removeServerFromMirror drops the server with id from the in-memory Servers[]
// mirror. The DB is authoritative (needsSetup() reads hasAnyServer()); the
// mirror just keeps loadedConfig consistent for in-process reads.
func removeServerFromMirror(id int) {
	filtered := make([]ServerConfig, 0, len(loadedConfig.Servers))
	for _, sc := range loadedConfig.Servers {
		if sc.ID != id {
			filtered = append(filtered, sc)
		}
	}
	loadedConfig.Servers = filtered
}

// parseServerID extracts the numeric server id from the request path and returns
// it together with its string scope form (used as the registry key and the
// per-feature server_id column value).
func parseServerID(r *http.Request) (int, string, error) {
	raw := r.PathValue("id")
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "", fmt.Errorf("invalid server id %q", raw)
	}
	return id, serverScope(id), nil
}

// clearFlatConnectionConfig blanks the top-level connection fields so a
// configured install reverts to "needs setup" without touching global settings
// (auth, Discord, listen address, market bot).
func clearFlatConnectionConfig(cfg *appConfig) {
	cfg.SSHHost = ""
	cfg.SSHUser = ""
	cfg.SSHKey = ""
	cfg.SSHMode = ""
	cfg.SSHExtraOpts = ""
	cfg.AutoDiscover = false
	cfg.DBHost = ""
	cfg.DBPort = 0
	cfg.DBUser = ""
	cfg.DBPass = ""
	cfg.DBName = ""
	cfg.DBSchema = ""
	cfg.Control = ""
	cfg.ControlNamespace = ""
}

// handleAddServer registers a new server from a posted ServerConfig body.
// The server is connected immediately; the config is persisted on success.
func handleAddServer(w http.ResponseWriter, r *http.Request) {
	if globalServersStore == nil {
		jsonErr(w, fmt.Errorf("server store unavailable"), http.StatusServiceUnavailable)
		return
	}
	var cfg ServerConfig
	if err := decode(r, &cfg); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	// The id is DB-assigned (autoincrement); any client-supplied id is ignored.
	cfg.ID = 0
	cfg.LegacyID = ""
	// Apply control-plane defaults to blank fields so the persisted entry matches
	// what we connect with (and what a console setup would have used).
	applyServerConfigDefaults(&cfg)
	pos, err := globalServersStore.nextPosition()
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	newID, err := globalServersStore.insertServer(cfg, pos)
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	cfg.ID = newID
	scope := serverScope(newID)
	sc, err := connectServer(cfg)
	if err != nil {
		componentLog("server_selector").Error().Err(err).Int("server_id", newID).Msg("add server connect failed")
		// Register with what we have — caller can reconnect later.
		sc = &ServerContext{ID: scope, Name: cfg.Name, Cfg: cfg, StoreScope: storeScopeForID(newID)}
	}
	globalRegistry.Register(sc)
	// Start the market bot if this server has it enabled and a live DB.
	startServerMarketBot(sc, loadedConfig)
	// Keep the in-memory Servers[] mirror in sync (DB is authoritative).
	loadedConfig.Servers = append(loadedConfig.Servers, cfg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(serverListItem{ID: newID, Name: sc.Name, Active: globalRegistry.ActiveID() == sc.ID})
}

// handleReconnectServer re-establishes the connection for one server identified
// by its ID in the path. The reconnected ServerContext replaces the existing
// registry entry; if it was the active server, the global aliases are updated.
func handleReconnectServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc := globalRegistry.Get(id)
	if sc == nil {
		jsonErr(w, fmt.Errorf("server %q not found", id), http.StatusNotFound)
		return
	}
	// Stop the old instance's bot before replacing the context.
	stopServerMarketBot(sc)
	newSC, err := connectServer(sc.Cfg)
	if err != nil {
		componentLog("server_selector").Error().Err(err).Str("server_id", id).Msg("reconnect server failed")
		jsonErr(w, fmt.Errorf("reconnect failed: %v", err), http.StatusServiceUnavailable)
		return
	}
	globalRegistry.Register(newSC)
	if globalRegistry.ActiveID() == id {
		globalDB = newSC.DB
		globalControl = newSC.Control
		globalExecutor = newSC.Executor
	}
	if newSC.DB != nil {
		ensureDBSchema(newSC.DB)
	}
	// Now that the DB is (re)connected, start the bot if enabled.
	startServerMarketBot(newSC, loadedConfig)
	invalidateServerHealth(id) // status changed → drop stale cached health
	jsonOK(w, map[string]bool{"connected": true})
}

// ── per-server config (GET/PUT /api/v1/servers/{id}/config) ──────────────────

// maskServerSecrets replaces secret fields with the display placeholder so the
// real values never leave the backend.
func maskServerSecrets(c *ServerConfig) {
	if c.DBPass != "" {
		c.DBPass = masked
	}
	if c.BrokerPass != "" {
		c.BrokerPass = masked
	}
	if c.BrokerJWTSecret != "" {
		c.BrokerJWTSecret = masked
	}
	if c.AmpAPIPass != "" {
		c.AmpAPIPass = masked
	}
}

// preserveServerMaskedSecrets restores real secret values from old when the
// client echoed back the display placeholder (i.e. left the field unchanged).
func preserveServerMaskedSecrets(next *ServerConfig, old ServerConfig) {
	if next.DBPass == masked {
		next.DBPass = old.DBPass
	}
	if next.BrokerPass == masked {
		next.BrokerPass = old.BrokerPass
	}
	if next.BrokerJWTSecret == masked {
		next.BrokerJWTSecret = old.BrokerJWTSecret
	}
	if next.AmpAPIPass == masked {
		next.AmpAPIPass = old.AmpAPIPass
	}
}

// preserveServerConnectionFields keeps connection fields the client left blank on
// an update from wiping the persisted value. Currently the only such field is
// ssh_mode: a client that omits it (e.g. a UI build without the ssh_mode field)
// must not silently reset a "command" server back to the library default. An
// explicit value always wins, so a deliberate change to "library"/"command" is
// honored. New blank-preserving connection fields go here (hence the plural name).
func preserveServerConnectionFields(next *ServerConfig, old ServerConfig) {
	if next.SSHMode == "" {
		next.SSHMode = old.SSHMode
	}
}

// handleGetServerConfig returns one server's ServerConfig with secrets masked.
func handleGetServerConfig(w http.ResponseWriter, r *http.Request) {
	id, scope, err := parseServerID(r)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	sc := globalRegistry.Get(scope)
	if sc == nil {
		jsonErr(w, fmt.Errorf("server %d not found", id), http.StatusNotFound)
		return
	}
	cfg := sc.Cfg
	maskServerSecrets(&cfg)
	jsonOK(w, cfg)
}

// handleUpdateServerConfig persists an edited ServerConfig, reconnects that
// server, and re-points the global aliases when the edited server is active.
func handleUpdateServerConfig(w http.ResponseWriter, r *http.Request) {
	id, scope, err := parseServerID(r)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	sc := globalRegistry.Get(scope)
	if sc == nil {
		jsonErr(w, fmt.Errorf("server %d not found", id), http.StatusNotFound)
		return
	}
	var next ServerConfig
	if err := decode(r, &next); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	next.ID = id // path wins — never let the body rename the server
	next.LegacyID = ""
	if next.Name == "" {
		next.Name = sc.Name
	}
	preserveServerMaskedSecrets(&next, sc.Cfg)
	preserveServerConnectionFields(&next, sc.Cfg)
	applyServerConfigDefaults(&next)

	if err := persistServerConfig(id, next); err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}

	// Stop the old instance's bot before replacing the context.
	stopServerMarketBot(sc)

	// Reconnect with the new config. Best-effort: register a control-plane-only
	// (or even bare) context on failure so the edit still takes effect.
	newSC, err := connectServer(next)
	if err != nil {
		componentLog("server_selector").Error().Err(err).Int("server_id", id).Msg("update server config reconnect failed")
	}
	globalRegistry.Register(newSC)
	if globalRegistry.ActiveID() == scope {
		globalDB = newSC.DB
		globalControl = newSC.Control
		globalExecutor = newSC.Executor
	}
	if newSC.DB != nil {
		ensureDBSchema(newSC.DB)
	}
	// Apply the (possibly changed) per-server market-bot toggle.
	startServerMarketBot(newSC, loadedConfig)
	invalidateServerHealth(scope) // config edit may change status → drop stale health

	out := next
	maskServerSecrets(&out)
	jsonOK(w, out)
}

// persistServerConfig writes the updated per-server config to the DB (the
// source of truth) and keeps the in-memory Servers[] mirror in sync.
func persistServerConfig(id int, sc ServerConfig) error {
	sc.ID = id
	if globalServersStore != nil {
		if err := globalServersStore.updateServer(sc); err != nil {
			return fmt.Errorf("persist server config: %w", err)
		}
	}
	updated := make([]ServerConfig, 0, len(loadedConfig.Servers)+1)
	found := false
	for _, e := range loadedConfig.Servers {
		if e.ID == id {
			updated = append(updated, sc)
			found = true
		} else {
			updated = append(updated, e)
		}
	}
	if !found {
		updated = append(updated, sc)
	}
	loadedConfig.Servers = updated
	return nil
}
