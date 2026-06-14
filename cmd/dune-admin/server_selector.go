package main

import (
	"context"
	"fmt"
	"net/http"

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

// storeScopeFromCtx returns the SQLite server_id scope for the request's server
// context. Falls back to "default" so callers that skip the middleware are safe.
func storeScopeFromCtx(r *http.Request) string {
	sc := serverFromCtx(r)
	if sc == nil {
		return "default"
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

// serverListItem is the JSON shape returned by GET /api/v1/servers.
type serverListItem struct {
	ID     string `json:"id"`
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
			ID:     sc.ID,
			Name:   sc.Name,
			Active: sc.ID == activeID,
		})
	}
	jsonOK(w, items)
}

// handleSetActiveServer switches the process-wide active server.
// Body: {"id":"<serverID>"}
func handleSetActiveServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		jsonErr(w, fmt.Errorf("id is required"), http.StatusBadRequest)
		return
	}
	if err := globalRegistry.SetActive(body.ID); err != nil {
		jsonErr(w, err, http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]string{"active": body.ID})
}
