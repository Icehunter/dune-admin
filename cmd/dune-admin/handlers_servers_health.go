package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// serverHealth is the dashboard health summary for one registered server.
type serverHealth struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	Control       string `json:"control"`
	Running       bool   `json:"running"`
	Phase         string `json:"phase"`
	UptimeSeconds int    `json:"uptime_seconds"`
	PlayersOnline int    `json:"players_online"`
	DBConnected   bool   `json:"db_connected"`
	Error         string `json:"error,omitempty"`
}

// assembleServerHealth builds a best-effort health summary for one server.
// Control-plane status drives running/phase/uptime; the game DB drives the
// online player count (falling back to summing control-plane rows when the DB
// pool is unavailable). Never panics — failures land in the Error field.
func assembleServerHealth(ctx context.Context, sc *ServerContext) serverHealth {
	h := serverHealth{
		ID:          sc.Cfg.ID,
		Name:        sc.Name,
		Active:      globalRegistry.ActiveID() == sc.ID,
		Control:     sc.Cfg.Control,
		DBConnected: sc.DB != nil,
	}
	if h.Control == "" {
		h.Control = "local"
	}

	if sc.Control == nil {
		h.Error = "not connected"
		return h
	}

	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	st, err := sc.Control.GetStatus(cctx, sc.Executor)
	if err != nil || st == nil {
		// A control plane that simply can't report status (e.g. local without
		// cmd_status) is not an error worth surfacing on the card — leave the
		// status Unknown. Only real failures populate Error.
		if err != nil && !strings.Contains(err.Error(), "does not support GetStatus") {
			h.Error = err.Error()
		}
		return h
	}

	h.Phase = st.Phase
	h.Running = st.Phase == "Running"
	rowPlayers := 0
	for _, row := range st.Servers {
		if row.Ready {
			h.Running = true
		}
		if row.AgeSeconds > h.UptimeSeconds {
			h.UptimeSeconds = row.AgeSeconds
		}
		rowPlayers += row.Players
	}

	// Prefer the authoritative DB online count; fall back to control-plane rows.
	h.PlayersOnline = rowPlayers
	if sc.DB != nil {
		if stats, serr := cmdFetchServerStats(cctx, sc.DB); serr == nil {
			h.PlayersOnline = int(stats.OnlinePlayers)
		}
	}
	return h
}

// handleServersHealth returns a health summary for every registered server, in
// registration order. Best-effort per server — one server's failure doesn't
// fail the whole response.
//
// @Summary Per-server health summary for the dashboard
// @Tags servers
// @Produce json
// @Success 200 {array} serverHealth
// @Router /api/v1/servers/health [get]
func handleServersHealth(w http.ResponseWriter, r *http.Request) {
	all := globalRegistry.All()
	out := make([]serverHealth, 0, len(all))
	for _, sc := range all {
		out = append(out, cachedServerHealth(r.Context(), sc))
	}
	jsonOK(w, out)
}

// cachedServerHealth returns a server's health from the cache (re-assembling
// live on a miss). The expensive control-plane + DB fan-out in
// assembleServerHealth is what's cached; the cheap Active flag is recomputed on
// every read so switching the active server is reflected immediately even from
// a cache hit.
func cachedServerHealth(ctx context.Context, sc *ServerContext) serverHealth {
	if globalHealthCache == nil {
		return assembleServerHealth(ctx, sc)
	}
	h, _ := globalHealthCache.GetOrLoad(ctx, cacheKey(sc.ID, "health"), healthCacheTTL,
		func(ctx context.Context) (serverHealth, error) {
			return assembleServerHealth(ctx, sc), nil
		})
	h.Active = globalRegistry.ActiveID() == sc.ID
	return h
}
