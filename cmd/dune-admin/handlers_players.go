package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// @Summary List all players
// @Tags players
// @Produce json
// @Success 200 {array} playerInfo
// @Failure 500 {object} map[string]string
// @Router /api/v1/players [get]
func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := cachedPlayers(r)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if rows == nil {
		rows = []playerInfo{}
	}
	jsonOK(w, rows)
}

// cachedPlayers serves the player list from the per-server cache (busted on any
// player-write — see handleAPI), loading live on a miss. Falls back to a live
// fetch when there's no server context or the cache is unavailable.
func cachedPlayers(r *http.Request) ([]playerInfo, error) {
	load := func(context.Context) ([]playerInfo, error) {
		msg, ok := cmdFetchPlayers(dbFromCtx(r)).(msgPlayers)
		if !ok {
			return nil, fmt.Errorf("internal error")
		}
		if msg.err != nil {
			return nil, msg.err
		}
		// Enrich Discord identity from SQLite (one global link per user), scoped
		// to the server whose pool was queried. The query is per-server, so the
		// store scope of the request's server context selects the right links.
		return enrichPlayersWithDiscord(msg.rows, storeScopeFromCtx(r)), nil
	}
	sc := serverFromCtx(r)
	if sc == nil || globalPlayersCache == nil {
		return load(r.Context())
	}
	return globalPlayersCache.GetOrLoad(r.Context(), cacheKey(sc.ID, "players"), playersCacheTTL, load)
}

// summaryTrendDays is the activity-trend window for the Players dashboard (#130).
const summaryTrendDays = 14

// @Summary Server-wide player summary dashboard
// @Tags players
// @Produce json
// @Success 200 {object} serverSummary
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/players/summary [get]
func handleGetPlayerSummary(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	stats, err := cmdFetchServerStats(r.Context(), db)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("fetch server stats failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	playtime, trend := sessionSummary(r.Context(), globalSessionDB, storeScopeFromCtx(r), summaryTrendDays)
	// Average character level is best-effort: a query failure degrades to 0
	// (averageLevel(nil)) rather than failing the whole dashboard.
	xps, err := cmdFetchCharXPList(r.Context(), db)
	if err != nil {
		componentLog("handlers").Warn().Err(err).Msg("fetch char xp list failed")
	}
	jsonOK(w, serverSummary{
		TotalPlayers:      stats.TotalPlayers,
		OnlinePlayers:     stats.OnlinePlayers,
		ByMap:             stats.ByMap,
		ByFaction:         stats.ByFaction,
		TotalSolaris:      stats.TotalSolaris,
		TotalScrip:        stats.TotalScrip,
		AvgCharLevel:      averageLevel(xps),
		TotalPlaytimeSecs: playtime,
		ActivityTrend:     trend,
		TrendDays:         summaryTrendDays,
	})
}

// factionTrendDays is the look-back window for the faction-growth chart (#130 ext).
const factionTrendDays = 30

// @Summary Faction-growth time series (Solaris or avg level per faction)
// @Tags players
// @Produce json
// @Param metric query string false "solaris (default) | level"
// @Success 200 {object} factionTrends
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/players/faction-trends [get]
func handleGetFactionTrends(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	metric := r.URL.Query().Get("metric")
	if metric != "level" {
		metric = "solaris"
	}
	acctFaction, err := cmdFetchAccountFactions(r.Context(), db)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("fetch account factions failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	// Snapshots are best-effort: a missing/failed session DB yields an empty
	// series rather than failing the request.
	var snaps []daySnap
	if globalSessionDB != nil {
		if snaps, err = getDailySnapshots(r.Context(), globalSessionDB, storeScopeFromCtx(r), factionTrendDays); err != nil {
			componentLog("handlers").Warn().Err(err).Msg("fetch daily snapshots failed")
		}
	}
	jsonOK(w, bucketFactionTrends(snaps, acctFaction, metric))
}

// @Summary Get online state for all players
// @Tags players
// @Produce json
// @Success 200 {array} object
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/online [get]
func handleGetOnlineState(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchOnlineState(dbFromCtx(r)).(msgOnlineState)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	// Serialize as JSON-friendly structs
	type onlineRow struct {
		PlayerID int64  `json:"player_id"`
		Name     string `json:"name"`
		Map      string `json:"map"`
		Status   string `json:"status"`
		LastSeen string `json:"last_seen"`
	}
	rows := make([]onlineRow, 0, len(msg.rows))
	for _, r := range msg.rows {
		rows = append(rows, onlineRow(r))
	}
	jsonOK(w, rows)
}

// @Summary List currency balances for all players
// @Tags players
// @Produce json
// @Success 200 {array} currencyRow
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/currency [get]
func handleGetCurrency(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchCurrency(dbFromCtx(r)).(msgCurrency)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []currencyRow{}
	}
	jsonOK(w, rows)
}

// @Summary List faction reputation for all players
// @Tags players
// @Produce json
// @Success 200 {array} factionRep
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/factions [get]
func handleGetFactions(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchFactions(dbFromCtx(r)).(msgFactions)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []factionRep{}
	}
	jsonOK(w, rows)
}

// @Summary List specialization tracks for all players
// @Tags players
// @Produce json
// @Success 200 {array} specTrack
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/specs [get]
func handleGetSpecs(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchSpecs(dbFromCtx(r)).(msgSpecs)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []specTrack{}
	}
	jsonOK(w, rows)
}

// @Summary List all known item templates
// @Tags players
// @Produce json
// @Success 200 {array} object
// @Router /api/v1/players/templates [get]
func handleGetTemplates(w http.ResponseWriter, r *http.Request) {
	type templateOut struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	rows := make([]templateOut, len(dbItemTemplates))
	for i, t := range dbItemTemplates {
		name := ""
		name = itemData.Names[strings.ToLower(t)]
		rows[i] = templateOut{ID: t, Name: name}
	}
	jsonOK(w, rows)
}

// @Summary Get inventory for a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} itemInfo
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/inventory [get]
func handleGetInventory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchInventory(dbFromCtx(r), id)().(msgInventory)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []itemInfo{}
	}
	jsonOK(w, rows)
}

// @Summary Get journey node state for a character
// @Tags players
// @Produce json
// @Param id path int true "Player (character/actor) ID"
// @Success 200 {array} object
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/journey [get]
func handleGetJourney(w http.ResponseWriter, r *http.Request) {
	playerIDStr := r.PathValue("id")
	playerID, err := strconv.ParseInt(playerIDStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchJourneyNodes(dbFromCtx(r), scopeFromReq(r), playerID)().(msgJourney)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []journeyNode{}
	}
	// Serialize with JSON tags (journeyNode fields are unexported-named but have tags in new model.go)
	type jNode struct {
		NodeID           string `json:"node_id"`
		IsComplete       bool   `json:"is_complete"`
		IsRevealed       bool   `json:"is_revealed"`
		HasPendingReward bool   `json:"has_pending_reward"`
	}
	out := make([]jNode, 0, len(rows))
	for _, n := range rows {
		out = append(out, jNode(n))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// @Summary Give a single item to a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, template, qty, quality"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-item [post]
// tryGiveItemViaRMQ attempts to deliver an item to an online player via the RMQ
// server command path (instant, no relog needed). Returns true if the request
// was handled (success or error written to w), false if the caller should fall
// through to the DB path. RMQ is skipped for quality > 0 or for item types
// (schematics, augments) where grade is stored as quality_level in the DB.
func tryGiveItemViaRMQ(w http.ResponseWriter, db *pgxpool.Pool, playerID int64, template string, qty int64) bool {
	ctx := context.Background()
	if checkPlayerOfflinePool(ctx, db, playerID) == nil {
		return false
	}
	if err := checkInventoryCapacity(ctx, playerID, template, qty); err != nil {
		jsonErr(w, err, 400)
		return true
	}
	flsID, err := flsIDFromActorID(ctx, playerID)
	if err != nil {
		jsonErr(w, fmt.Errorf("resolve player: %w", err), 404)
		return true
	}
	if err := rmqAddItemToInventory(flsID, template, int(qty), 1.0); err != nil {
		jsonErr(w, err, 500)
		return true
	}
	jsonOK(w, map[string]any{
		"ok":   fmt.Sprintf("sent %d × %s to online player %d via server command (instant)", qty, template, playerID),
		"path": "rmq",
	})
	return true
}

func handleGiveItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64  `json:"player_id"`
		Template string `json:"template"`
		Qty      int64  `json:"qty"`
		Quality  int64  `json:"quality"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}

	quality, err := normalizeGrantQuality(req.Template, req.Quality)
	if err != nil {
		jsonErr(w, err, 400)
		return
	}

	// RMQ path: online player, quality=0, and item grade doesn't matter.
	// Schematics and augment items are excluded — their quality_level must be
	// stored explicitly in the DB; the RMQ command has no grade field.
	if quality == 0 && !itemNeedsDBPath(req.Template) {
		if tryGiveItemViaRMQ(w, dbFromCtx(r), req.PlayerID, req.Template, req.Qty) {
			return
		}
	}

	// DB path: offline player, quality > 0, or grade-sensitive item.
	msg, ok := cmdGiveItem(dbFromCtx(r), req.PlayerID, req.Template, req.Qty, quality)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"ok": msg.ok, "path": "db"})
}

type giveItemInput struct {
	Template string `json:"template"`
	Qty      int64  `json:"qty"`
	Quality  int64  `json:"quality"`
}

type giveItemsRequest struct {
	PlayerID int64           `json:"player_id"`
	Items    []giveItemInput `json:"items"`
}

type skippedItem struct {
	Template string `json:"template"`
	Reason   string `json:"reason"`
}

type giveItemsDeps struct {
	checkCapacity    func(context.Context, int64, string, int64) error
	rmqAdd           func(string, string, int, float64) error
	dbGive           func(int64, string, int64, int64) (msgMutate, bool)
	needsDBPath      func(string) bool
	normalizeQuality func(string, int64) (int64, error)
}

func resolveGiveItemsOnlinePath(
	ctx context.Context,
	playerID int64,
	isOffline func(context.Context, int64) error,
	resolveFLS func(context.Context, int64) (string, error),
) (bool, string) {
	online := playerID != 0 && isOffline(ctx, playerID) != nil
	if !online {
		return false, ""
	}
	flsID, err := resolveFLS(ctx, playerID)
	if err != nil {
		return false, ""
	}
	return true, flsID
}

// itemNeedsDBPath reports whether the item must bypass the RMQ path so that its
// quality_level is stored explicitly. Schematics and augmentation-category items
// have grade-dependent behaviour that the RMQ AddItemToInventory command cannot
// express (it has no quality/grade field).
func itemNeedsDBPath(template string) bool {
	rule, ok := itemRuleLookup(template)
	if !ok {
		return false
	}
	return rule.IsSchematic || strings.Contains(strings.ToLower(rule.Category), "augment")
}

// normalizeGrantQuality validates and adjusts the requested quality for a grant:
//   - Schematics are blocked entirely: the game does not reconstruct research-cost
//     data from quality_level alone, so a granted schematic always ends up broken
//     regardless of grade. Admins should grant the finished augment instead.
//   - Gradeable items require quality ≥ min_quality_level (or ≥ 1 if unset). A
//     quality of 0 would produce an item with 0% stats; clamp up to the minimum.
//   - All other items pass through unchanged.
func normalizeGrantQuality(template string, quality int64) (int64, error) {
	rule, ok := itemRuleLookup(template)
	if !ok {
		return quality, nil
	}
	if rule.IsSchematic {
		return 0, fmt.Errorf("schematics cannot be granted directly — they do not carry valid research data when inserted via the DB path; grant the finished augment/item instead")
	}
	if rule.IsGradeable {
		minQ := int64(1)
		if rule.MinQualityLevel != nil {
			minQ = *rule.MinQualityLevel
		}
		if quality < minQ {
			return minQ, nil
		}
	}
	return quality, nil
}

func processOneGiveItem(ctx context.Context, playerID int64, item giveItemInput, online bool, flsID string, deps giveItemsDeps) (string, *skippedItem) {
	quality := item.Quality
	if deps.normalizeQuality != nil {
		var err error
		quality, err = deps.normalizeQuality(item.Template, quality)
		if err != nil {
			return "", &skippedItem{Template: item.Template, Reason: err.Error()}
		}
	}
	needsDB := deps.needsDBPath != nil && deps.needsDBPath(item.Template)
	if online && quality == 0 && !needsDB {
		if err := deps.checkCapacity(ctx, playerID, item.Template, item.Qty); err != nil {
			return "", &skippedItem{Template: item.Template, Reason: err.Error()}
		}
		if err := deps.rmqAdd(flsID, item.Template, int(item.Qty), 1.0); err != nil {
			return "", &skippedItem{Template: item.Template, Reason: err.Error()}
		}
		return item.Template, nil
	}
	msg, ok := deps.dbGive(playerID, item.Template, item.Qty, quality)
	if !ok || msg.err != nil {
		reason := "internal error"
		if ok && msg.err != nil {
			reason = msg.err.Error()
		}
		return "", &skippedItem{Template: item.Template, Reason: reason}
	}
	return item.Template, nil
}

func processGiveItems(
	ctx context.Context,
	req giveItemsRequest,
	online bool,
	flsID string,
	deps giveItemsDeps,
) ([]string, []skippedItem) {
	given := make([]string, 0, len(req.Items))
	skipped := make([]skippedItem, 0)
	for _, item := range req.Items {
		g, s := processOneGiveItem(ctx, req.PlayerID, item, online, flsID, deps)
		if s != nil {
			skipped = append(skipped, *s)
			continue
		}
		given = append(given, g)
	}
	return given, skipped
}

// @Summary Give multiple items to a player in a single request
// @Tags players
// @Accept json
// @Produce json
// @Param body body giveItemsRequest true "player_id and items list"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-items [post]
func handleGiveItems(w http.ResponseWriter, r *http.Request) {
	var req giveItemsRequest
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	ctx := context.Background()
	online, flsID := resolveGiveItemsOnlinePath(ctx, req.PlayerID, func(ctx context.Context, id int64) error { return checkPlayerOfflinePool(ctx, dbFromCtx(r), id) }, flsIDFromActorID)
	given, skipped := processGiveItems(ctx, req, online, flsID, giveItemsDeps{
		checkCapacity: checkInventoryCapacity,
		rmqAdd:        rmqAddItemToInventory,
		dbGive: func(playerID int64, template string, qty, quality int64) (msgMutate, bool) {
			msg, ok := cmdGiveItem(dbFromCtx(r), playerID, template, qty, quality)().(msgMutate)
			return msg, ok
		},
		needsDBPath:      itemNeedsDBPath,
		normalizeQuality: normalizeGrantQuality,
	})

	jsonOK(w, map[string]any{"given": given, "skipped": skipped})
}

// @Summary Grant an item to a player via FLS/PlayFab live path
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "controller_id, template, amount"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/grant-live [post]
func handleGrantLive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ControllerID int64  `json:"controller_id"`
		Template     string `json:"template"`
		Amount       int64  `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.Template == "" {
		jsonErr(w, fmt.Errorf("template required"), 400)
		return
	}
	if req.Amount <= 0 {
		req.Amount = 1
	}
	msg, ok := cmdGrantLive(dbFromCtx(r), req.ControllerID, req.Template, req.Amount)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Give currency to a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, amount"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-currency [post]
func handleGiveCurrency(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
		Amount   int64 `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGiveCurrency(dbFromCtx(r), req.PlayerID, req.Amount)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Adjust faction reputation for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "actor_id, faction_id, delta"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-faction-rep [post]
func handleGiveFactionRep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorID   int64 `json:"actor_id"`
		FactionID int16 `json:"faction_id"`
		Delta     int32 `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGiveFactionRep(dbFromCtx(r), req.ActorID, req.FactionID, req.Delta)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Adjust Landsraad scrip for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "actor_id, delta"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-scrip [post]
func handleGiveScrip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorID int64 `json:"actor_id"`
		Delta   int32 `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGiveLandsraadScrip(dbFromCtx(r), req.ActorID, req.Delta)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Award XP on a specialization track for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, track_type, delta"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/award-xp [post]
func handleAwardXP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64  `json:"player_id"`
		TrackType string `json:"track_type"`
		Delta     int32  `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.PlayerID == 0 {
		jsonErr(w, fmt.Errorf("player_id required"), 400)
		return
	}
	msg, ok := cmdAwardXP(dbFromCtx(r), req.PlayerID, req.TrackType, req.Delta)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Award character XP to a player (live RMQ or DB fallback)
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "fls_id (optional), player_id, amount"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/award-char-xp [post]
func handleAwardCharXP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID    string `json:"fls_id"`    // optional; triggers live online check
		PlayerID int64  `json:"player_id"` // required for DB path
		Amount   int64  `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	ctx := context.Background()
	if req.FlsID != "" && isHexIDOnline(ctx, req.FlsID) {
		if err := rmqAwardXP(req.FlsID, "Combat", int(req.Amount)); err != nil {
			jsonErr(w, err, 500)
			return
		}
		jsonOK(w, map[string]any{
			"ok":   fmt.Sprintf("live award %d char XP sent", req.Amount),
			"path": "rmq",
		})
		return
	}
	if req.PlayerID == 0 {
		jsonErr(w, fmt.Errorf("player_id required"), 400)
		return
	}
	msg, ok := cmdAwardCharXP(dbFromCtx(r), req.PlayerID, req.Amount)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"ok": msg.ok, "path": "db"})
}

// @Summary Award intel points to a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, amount"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/award-intel [post]
func handleAwardIntel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
		Amount   int64 `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdAwardIntel(dbFromCtx(r), req.PlayerID, req.Amount)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Rename a player's character
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id, name"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/rename [post]
func handleRenameCharacter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Name      string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRenameCharacter(dbFromCtx(r), req.AccountID, req.Name)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Permanently delete a character
// @Description Deletes a character via dune.delete_account: removes player
// @Description actors, respawn beacons, and the account row, writes an audit
// @Description log entry, and notifies the live server. Irreversible.
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Account ID and reason"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/delete [post]
func handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Reason    string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.AccountID == 0 {
		jsonErr(w, fmt.Errorf("account_id required"), 400)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		jsonErr(w, fmt.Errorf("reason required"), 400)
		return
	}
	msg, ok := cmdDeleteCharacter(dbFromCtx(r), req.AccountID, req.Reason)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		componentLog("handlers").Error().Int64("account_id", req.AccountID).Err(msg.err).Msg("delete character failed")
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Get tags assigned to a character
// @Tags players
// @Produce json
// @Param id path int true "Player (character/actor) ID"
// @Success 200 {array} string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/tags [get]
func handleGetPlayerTags(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetPlayerTags(dbFromCtx(r), id)().(msgTags)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	tags := msg.rows
	if tags == nil {
		tags = []string{}
	}
	jsonOK(w, tags)
}

// @Summary Add or remove tags on a character
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, add ([]string), remove ([]string)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/update-tags [post]
func handleUpdatePlayerTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64    `json:"player_id"`
		Add      []string `json:"add"`
		Remove   []string `json:"remove"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdUpdatePlayerTags(dbFromCtx(r), req.PlayerID, req.Add, req.Remove)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Dismiss the returning-player award for an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/dismiss-returning-player-award [post]
func handleDismissReturningPlayerAward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdDismissReturningPlayerAward(dbFromCtx(r), req.AccountID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Grant the returning-player award to an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/returning-player-award [post]
func handleGrantReturningPlayerAward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantReturningPlayerAward(dbFromCtx(r), req.AccountID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Export a character's data as a JSON attachment
// @Tags players
// @Produce application/octet-stream
// @Param id path int true "Account ID"
// @Success 200 {string} string "character-{id}.json attachment"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/export [get]
func handleCharacterExport(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	accountID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	db := dbFromCtx(r)
	ctx := r.Context()
	rawID, err := rawFuncomID(ctx, db, accountID)
	if err != nil {
		jsonErr(w, fmt.Errorf("account not found: %w", err), 404)
		return
	}
	var result string
	err = db.QueryRow(ctx, `SELECT dune.character_transfer_export($1)::text`, rawID).Scan(&result)
	if err != nil {
		jsonErr(w, fmt.Errorf("export failed: %w", err), 500)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="character-%d.json"`, accountID))
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, result)
}

// @Summary Delete a player account and all associated data
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id, reason"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/delete-account [post]
func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Reason    string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdDeleteAccount(dbFromCtx(r), req.AccountID, req.Reason)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Delete a specific inventory item by item ID
// @Tags players
// @Produce json
// @Param id path int true "Item ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/item/{id} [delete]
func handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdDeleteItem(dbFromCtx(r), id)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Reset a specialization track for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, track_type"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/reset-spec [post]
func handleResetSpec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64  `json:"player_id"`
		TrackType string `json:"track_type"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetSpecializations(dbFromCtx(r), req.PlayerID, req.TrackType)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Set a player's tier within a faction
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "actor_id, faction_id, tier"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/set-faction-tier [post]
func handleSetFactionTier(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorID   int64 `json:"actor_id"`
		FactionID int16 `json:"faction_id"`
		Tier      int   `json:"tier"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdSetFactionTier(dbFromCtx(r), req.ActorID, req.FactionID, req.Tier)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Unlock a progression preset for a player
// @Tags progression
// @Accept json
// @Produce json
// @Param body body object true "player_id, faction, preset"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/progression-unlock [post]
func handleProgressionUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64  `json:"player_id"`
		Faction  string `json:"faction"`
		Preset   string `json:"preset"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdProgressionUnlock(dbFromCtx(r), req.PlayerID, req.Faction, req.Preset)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Reverse a previously unlocked progression preset for a player
// @Tags progression
// @Accept json
// @Produce json
// @Param body body object true "player_id, faction, preset"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/progression-reverse [post]
func handleProgressionReverse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64  `json:"player_id"`
		Faction  string `json:"faction"`
		Preset   string `json:"preset"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdReverseProgressionUnlock(dbFromCtx(r), req.PlayerID, req.Faction, req.Preset)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Mark a journey node as complete for a character
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, node_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/journey/complete [post]
func handleJourneyComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64  `json:"player_id"`
		NodeID   string `json:"node_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteJourneyNode(dbFromCtx(r), req.PlayerID, req.NodeID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Complete a single contract for a character
// @Tags contracts
// @Accept json
// @Produce json
// @Param body body object true "player_id, contract_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/contract/complete [post]
func handleCompleteContract(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   int64  `json:"player_id"`
		ContractID string `json:"contract_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteContract(dbFromCtx(r), req.PlayerID, req.ContractID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Reset job skills for an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id, job"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/reset-job-skills [post]
func handleResetJobSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetJobSkills(dbFromCtx(r), req.AccountID, req.Job)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Set the starter class (job) for an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id, job"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/set-starter-class [post]
func handleSetStarterClass(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdSetStarterClass(dbFromCtx(r), req.AccountID, req.Job)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Grant job skills for a specific job to an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id, job"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/grant-job-skills [post]
func handleGrantJobSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantJobSkills(dbFromCtx(r), req.AccountID, req.Job)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Complete multiple contracts for a character
// @Tags contracts
// @Accept json
// @Produce json
// @Param body body object true "player_id, contract_ids ([]string)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/contracts/complete [post]
func handleCompleteContracts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID    int64    `json:"player_id"`
		ContractIDs []string `json:"contract_ids"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteContracts(dbFromCtx(r), req.PlayerID, req.ContractIDs)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Reverse (undo) multiple completed contracts for a character
// @Tags contracts
// @Accept json
// @Produce json
// @Param body body object true "player_id, contract_ids ([]string)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/contracts/reverse [post]
func handleReverseContracts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID    int64    `json:"player_id"`
		ContractIDs []string `json:"contract_ids"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdReverseContracts(dbFromCtx(r), req.PlayerID, req.ContractIDs)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// handleListContracts returns the catalog of known contracts (id → tag count)
// so the frontend can render a picker without shipping the full tag dump.
// @Summary List the catalog of known contracts
// @Tags contracts
// @Produce json
// @Success 200 {array} object
// @Router /api/v1/contracts [get]
func handleListContracts(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID       string `json:"id"`
		Alias    string `json:"alias"`
		TagCount int    `json:"tag_count"`
	}
	out := make([]row, 0, len(tagsData.ContractTags))
	// Build reverse alias lookup: full name → short alias.
	revAlias := make(map[string]string, len(tagsData.ContractAliases))
	for short, full := range tagsData.ContractAliases {
		revAlias[full] = short
	}
	for id, tags := range tagsData.ContractTags {
		out = append(out, row{ID: id, Alias: revAlias[id], TagCount: len(tags)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	jsonOK(w, out)
}

// @Summary Reset (incomplete) a journey node for a character
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, node_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/journey/reset [post]
func handleJourneyReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64  `json:"player_id"`
		NodeID   string `json:"node_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetJourneyNode(dbFromCtx(r), req.PlayerID, req.NodeID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Wipe all journey nodes for a character
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/journey/wipe [post]
func handleJourneyWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdWipeJourneyNodes(dbFromCtx(r), req.PlayerID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateAllJourneyCache()
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Delete all tutorial records for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/delete-tutorials [post]
func handleDeleteTutorials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	// db.go names this cmdDeleteAllTutorials
	msg, ok := cmdDeleteAllTutorials(dbFromCtx(r), req.PlayerID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Wipe the codex for an account
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "account_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/wipe-codex [post]
func handleWipeCodex(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdWipeCodex(dbFromCtx(r), req.AccountID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Get character XP and level for a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/char-xp [get]
func handleGetCharXP(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchCharXP(dbFromCtx(r), id)().(msgCharXP)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"xp": msg.xp, "level": msg.level})
}

// @Summary Grant all keystones to a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/grant-all-keystones [post]
func handleGrantAllKeystones(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantAllKeystones(dbFromCtx(r), req.PlayerID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Reset all keystones for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/reset-all-keystones [post]
func handleResetAllKeystones(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetAllKeystones(dbFromCtx(r), req.PlayerID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Get keystones owned by a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} object
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/keystones [get]
func handleGetPlayerKeystones(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerKeystones(dbFromCtx(r), id)().(msgKeystones)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	type keystoneRow struct {
		ID    int16  `json:"id"`
		Track string `json:"track"`
		Name  string `json:"name"`
		Level int    `json:"level"`
		Cost  int    `json:"cost"`
	}
	var result []keystoneRow
	for _, id := range msg.ids {
		if info, ok := keystoneMap[id]; ok {
			result = append(result, keystoneRow{
				ID:    id,
				Track: info.Track,
				Name:  info.Name,
				Level: info.Level,
				Cost:  info.Cost,
			})
		}
	}
	if result == nil {
		result = []keystoneRow{}
	}
	jsonOK(w, result)
}

// @Summary Get specialization tracks for a specific player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} specTrack
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/specs [get]
func handleGetPlayerSpecs(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerSpecs(dbFromCtx(r), id)().(msgSpecs)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []specTrack{}
	}
	jsonOK(w, rows)
}

// @Summary Grant max level on a specialization track for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id, track_type"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/grant-max-spec [post]
func handleGrantMaxSpec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64  `json:"player_id"`
		TrackType string `json:"track_type"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantMaxSpec(dbFromCtx(r), req.PlayerID, req.TrackType)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Get vehicles owned by a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} vehicleRow
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/vehicles [get]
func handleGetPlayerVehicles(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetPlayerVehicles(dbFromCtx(r), id)().(msgVehicles)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []vehicleRow{}
	}
	jsonOK(w, rows)
}

// @Summary Repair a single inventory item to full durability
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "id (item ID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/repair-item [post]
func handleRepairItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRepairItem(dbFromCtx(r), req.ID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// @Summary Repair all equipped gear for a player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "player_id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/repair-gear [post]
func handleRepairPlayerGear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRepairPlayerGear(dbFromCtx(r), req.PlayerID)().(msgRepairGear)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"repaired": msg.repaired, "scanned": msg.scanned})
}

// @Summary List available teleport partition locations
// @Tags players
// @Produce json
// @Success 200 {array} teleportLocation
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/partitions [get]
func handleGetPartitions(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListPartitions(dbFromCtx(r))().(msgPartitions)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []teleportLocation{}
	}
	jsonOK(w, rows)
}

// @Summary Teleport a player to a named partition location
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "fls_id, partition_label"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/teleport [post]
func handleTeleportPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FLSID    string `json:"fls_id"`
		Location string `json:"partition_label"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FLSID == "" || req.Location == "" {
		jsonErr(w, fmt.Errorf("fls_id and partition_label required"), 400)
		return
	}

	loc, locErr := resolveLocation(req.Location)
	if locErr != nil {
		jsonErr(w, locErr, 400)
		return
	}

	ctx := context.Background()

	// Online players: send via RMQ for immediate effect.
	if isHexIDOnline(ctx, req.FLSID) {
		if err := rmqTeleportTo(req.FLSID, loc.X, loc.Y, loc.Z); err != nil {
			jsonErr(w, err, 500)
			return
		}
		jsonOK(w, map[string]any{"ok": fmt.Sprintf("teleported to %s (live)", loc.Name), "path": "rmq"})
		return
	}

	// Offline players: write via DB (takes effect on next login).
	displayName, err := displayNameFromHexID(ctx, req.FLSID)
	if err != nil {
		jsonErr(w, fmt.Errorf("resolve player: %w", err), 404)
		return
	}
	msg, ok := cmdTeleportPlayer(dbFromCtx(r), displayName, req.Location)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"ok": msg.ok, "path": "db"})
}

// handleGetPlayerPosition returns the current world coordinates of a player's
// character (dune.actors.id). Used by the "teleport to player" UI to look up
// the target's position before publishing the teleport command.
// @Summary Get world position of a player's character
// @Tags players
// @Produce json
// @Param id path int true "Player ID (actor ID)"
// @Success 200 {object} object
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/position [get]
func handleGetPlayerPosition(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetPlayerPosition(dbFromCtx(r), id)().(msgPlayerPosition)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, msg.pos)
}

// handleTeleportToPlayer moves a source player to a target player's exact
// position. Online sources use TeleportToExact via RMQ; offline sources fall
// through to the DB write at the target's partition_id.
// @Summary Teleport a player to another player's current position
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "source_fls_id, target_id (actor ID)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/teleport-to-player [post]
func handleTeleportToPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceFLSID string `json:"source_fls_id"`
		TargetID    int64  `json:"target_id"` // actor id of the target character
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.SourceFLSID == "" || req.TargetID == 0 {
		jsonErr(w, fmt.Errorf("source_fls_id and target_id required"), 400)
		return
	}

	posMsg, ok := cmdGetPlayerPosition(dbFromCtx(r), req.TargetID)().(msgPlayerPosition)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if posMsg.err != nil {
		jsonErr(w, fmt.Errorf("target position: %w", posMsg.err), 404)
		return
	}
	target := posMsg.pos

	ctx := context.Background()
	if isHexIDOnline(ctx, req.SourceFLSID) {
		if err := rmqTeleportToExact(req.SourceFLSID, target.X, target.Y, target.Z); err != nil {
			jsonErr(w, fmt.Errorf("rmq teleport: %w", err), 500)
			return
		}
		jsonOK(w, map[string]any{
			"ok":   "teleported to target (live, exact)",
			"path": "rmq",
			"x":    target.X, "y": target.Y, "z": target.Z,
		})
		return
	}

	// Offline: write directly to DB at the target's partition.
	msg, ok := cmdTeleportPlayerToCoords(dbFromCtx(r), req.SourceFLSID, target.PartitionID, target.X, target.Y, target.Z)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{
		"ok":   msg.ok,
		"path": "db",
		"x":    target.X, "y": target.Y, "z": target.Z,
	})
}

// @Summary Get event log entries for a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} gameEvent
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/events [get]
func handleGetPlayerEvents(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchEventLog(dbFromCtx(r), id)().(msgEvents)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []gameEvent{}
	}
	jsonOK(w, rows)
}

// @Summary Get dungeon run records for a player
// @Tags players
// @Produce json
// @Param id path int true "Player ID"
// @Success 200 {array} dungeonRecord
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/dungeons [get]
func handleGetPlayerDungeons(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerDungeons(dbFromCtx(r), id)().(msgDungeons)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []dungeonRecord{}
	}
	jsonOK(w, rows)
}

// teleportCoordsParams bundles the injectable dependencies for processTeleportCoords
// so the online/offline branching can be unit-tested without a live DB or broker.
type teleportCoordsParams struct {
	flsID       string
	x, y, z     float64
	partitionID int64
	isOnline    func(flsID string) bool
	sendRMQ     func(flsID string, x, y, z float64) error
	writeDB     func(flsID string, partitionID int64, x, y, z float64) error
}

// processTeleportCoords routes a teleport-to-coords request: online players
// receive an immediate RMQ command; offline players get a DB write that takes
// effect on their next login.
func processTeleportCoords(p teleportCoordsParams) error {
	if p.isOnline(p.flsID) {
		return p.sendRMQ(p.flsID, p.x, p.y, p.z)
	}
	return p.writeDB(p.flsID, p.partitionID, p.x, p.y, p.z)
}

// @Summary Teleport a player to arbitrary XYZ coordinates
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "fls_id, x, y, z, partition_id (optional)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/teleport-coords [post]
// POST /api/v1/players/teleport-coords
func handleTeleportCoords(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID       string  `json:"fls_id"`
		X           float64 `json:"x"`
		Y           float64 `json:"y"`
		Z           float64 `json:"z"`
		PartitionID int64   `json:"partition_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	pool := dbFromCtx(r)
	err := processTeleportCoords(teleportCoordsParams{
		flsID:       req.FlsID,
		x:           req.X,
		y:           req.Y,
		z:           req.Z,
		partitionID: req.PartitionID,
		isOnline:    func(id string) bool { return isHexIDOnline(ctx, id) },
		sendRMQ:     func(id string, x, y, z float64) error { return rmqTeleportTo(id, x, y, z) },
		writeDB: func(id string, pid int64, x, y, z float64) error {
			msg, ok := cmdTeleportPlayerToCoords(pool, id, pid, x, y, z)().(msgMutate)
			if !ok {
				return fmt.Errorf("internal error")
			}
			return msg.err
		},
	})
	if err != nil {
		componentLog("handlers").Error().Str("fls_id", req.FlsID).Err(err).Msg("teleport coords failed")
		jsonErr(w, fmt.Errorf("teleport failed: %w", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": fmt.Sprintf("teleported %s to (%.0f, %.0f, %.0f)", req.FlsID, req.X, req.Y, req.Z)})
}
