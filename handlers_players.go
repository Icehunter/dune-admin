package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchPlayers().(msgPlayers)
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
		rows = []playerInfo{}
	}
	jsonOK(w, rows)
}

func handleGetOnlineState(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchOnlineState().(msgOnlineState)
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
		rows = append(rows, onlineRow{
			PlayerID: r.PlayerID,
			Name:     r.Name,
			Map:      r.Map,
			Status:   r.Status,
			LastSeen: r.LastSeen,
		})
	}
	jsonOK(w, rows)
}

func handleGetCurrency(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchCurrency().(msgCurrency)
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

func handleGetFactions(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchFactions().(msgFactions)
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

func handleGetSpecs(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdFetchSpecs().(msgSpecs)
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

func handleGetInventory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchInventory(id)().(msgInventory)
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

func handleGetJourney(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid accountId"), 400)
		return
	}
	msg, ok := cmdFetchJourneyNodes(accountID)().(msgJourney)
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
		out = append(out, jNode{
			NodeID:           n.NodeID,
			IsComplete:       n.IsComplete,
			IsRevealed:       n.IsRevealed,
			HasPendingReward: n.HasPendingReward,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
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

	// Route online + quality-0 items through RMQ (instant, no relog needed).
	// RMQ AddItemToInventory has no Quality field so the DB path is required for quality > 0.
	if req.Quality == 0 {
		ctx := context.Background()
		if checkPlayerOffline(ctx, req.PlayerID) != nil {
			// Player is online — use RMQ path.
			if err := checkInventoryCapacity(ctx, req.PlayerID, req.Template, req.Qty); err != nil {
				jsonErr(w, err, 400)
				return
			}
			flsID, err := flsIDFromActorID(ctx, req.PlayerID)
			if err != nil {
				jsonErr(w, fmt.Errorf("resolve player: %w", err), 404)
				return
			}
			if err := rmqAddItemToInventory(flsID, req.Template, int(req.Qty), 1.0); err != nil {
				jsonErr(w, err, 500)
				return
			}
			jsonOK(w, map[string]any{
				"ok":   fmt.Sprintf("sent %d × %s to online player %d via server command (instant)", req.Qty, req.Template, req.PlayerID),
				"path": "rmq",
			})
			return
		}
	}

	// DB path: offline player or quality > 0.
	msg, ok := cmdGiveItem(req.PlayerID, req.Template, req.Qty, req.Quality)().(msgMutate)
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

func handleGiveItems(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
		Items    []struct {
			Template string `json:"template"`
			Qty      int64  `json:"qty"`
			Quality  int64  `json:"quality"`
		} `json:"items"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	type skippedItem struct {
		Template string `json:"template"`
		Reason   string `json:"reason"`
	}
	given := []string{}
	skipped := []skippedItem{}

	ctx := context.Background()
	// Determine path once for all items in the batch.
	online := req.PlayerID != 0 && checkPlayerOffline(ctx, req.PlayerID) != nil
	var flsID string
	if online {
		var err error
		flsID, err = flsIDFromActorID(ctx, req.PlayerID)
		if err != nil {
			online = false // fall back to DB path if FLS ID resolution fails
		}
	}

	for _, item := range req.Items {
		if online && item.Quality == 0 {
			if err := checkInventoryCapacity(ctx, req.PlayerID, item.Template, item.Qty); err != nil {
				skipped = append(skipped, skippedItem{Template: item.Template, Reason: err.Error()})
				continue
			}
			if err := rmqAddItemToInventory(flsID, item.Template, int(item.Qty), 1.0); err != nil {
				skipped = append(skipped, skippedItem{Template: item.Template, Reason: err.Error()})
				continue
			}
			given = append(given, item.Template)
			continue
		}
		msg, ok := cmdGiveItem(req.PlayerID, item.Template, item.Qty, item.Quality)().(msgMutate)
		if !ok || msg.err != nil {
			reason := "internal error"
			if ok && msg.err != nil {
				reason = msg.err.Error()
			}
			skipped = append(skipped, skippedItem{Template: item.Template, Reason: reason})
			continue
		}
		given = append(given, item.Template)
	}
	jsonOK(w, map[string]any{"given": given, "skipped": skipped})
}

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
	msg, ok := cmdGrantLive(req.ControllerID, req.Template, req.Amount)().(msgMutate)
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

func handleGiveCurrency(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
		Amount   int64 `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGiveCurrency(req.PlayerID, req.Amount)().(msgMutate)
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
	msg, ok := cmdGiveFactionRep(req.ActorID, req.FactionID, req.Delta)().(msgMutate)
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

func handleGiveScrip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorID int64 `json:"actor_id"`
		Delta   int32 `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGiveLandsraadScrip(req.ActorID, req.Delta)().(msgMutate)
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
	msg, ok := cmdAwardXP(req.PlayerID, req.TrackType, req.Delta)().(msgMutate)
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
	msg, ok := cmdAwardCharXP(req.PlayerID, req.Amount)().(msgMutate)
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

func handleAwardIntel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
		Amount   int64 `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdAwardIntel(req.PlayerID, req.Amount)().(msgMutate)
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

func handleRenameCharacter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Name      string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRenameCharacter(req.AccountID, req.Name)().(msgMutate)
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

func handleGetPlayerTags(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetPlayerTags(id)().(msgTags)
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

func handleUpdatePlayerTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64    `json:"account_id"`
		Add       []string `json:"add"`
		Remove    []string `json:"remove"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdUpdatePlayerTags(req.AccountID, req.Add, req.Remove)().(msgMutate)
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

func handleDismissReturningPlayerAward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdDismissReturningPlayerAward(req.AccountID)().(msgMutate)
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

func handleGrantReturningPlayerAward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantReturningPlayerAward(req.AccountID)().(msgMutate)
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

func handleCharacterExport(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	accountID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	ctx := r.Context()
	rawID, err := rawFuncomID(ctx, accountID)
	if err != nil {
		jsonErr(w, fmt.Errorf("account not found: %w", err), 404)
		return
	}
	var result string
	err = globalDB.QueryRow(ctx, `SELECT dune.character_transfer_export($1)::text`, rawID).Scan(&result)
	if err != nil {
		jsonErr(w, fmt.Errorf("export failed: %w", err), 500)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="character-%d.json"`, accountID))
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, result)
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Reason    string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdDeleteAccount(req.AccountID, req.Reason)().(msgMutate)
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

func handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdDeleteItem(id)().(msgMutate)
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

func handleResetSpec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64  `json:"player_id"`
		TrackType string `json:"track_type"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetSpecializations(req.PlayerID, req.TrackType)().(msgMutate)
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
	msg, ok := cmdSetFactionTier(req.ActorID, req.FactionID, req.Tier)().(msgMutate)
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
	msg, ok := cmdProgressionUnlock(req.PlayerID, req.Faction, req.Preset)().(msgMutate)
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
	msg, ok := cmdReverseProgressionUnlock(req.PlayerID, req.Faction, req.Preset)().(msgMutate)
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

func handleJourneyComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		NodeID    string `json:"node_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteJourneyNode(req.AccountID, req.NodeID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

func handleCompleteContract(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID  int64  `json:"account_id"`
		ContractID string `json:"contract_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteContract(req.AccountID, req.ContractID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

func handleResetJobSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetJobSkills(req.AccountID, req.Job)().(msgMutate)
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

func handleSetStarterClass(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdSetStarterClass(req.AccountID, req.Job)().(msgMutate)
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

func handleGrantJobSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		Job       string `json:"job"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantJobSkills(req.AccountID, req.Job)().(msgMutate)
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

func handleCompleteContracts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID   int64    `json:"account_id"`
		ContractIDs []string `json:"contract_ids"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdCompleteContracts(req.AccountID, req.ContractIDs)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

func handleReverseContracts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID   int64    `json:"account_id"`
		ContractIDs []string `json:"contract_ids"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdReverseContracts(req.AccountID, req.ContractIDs)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// handleListContracts returns the catalog of known contracts (id → tag count)
// so the frontend can render a picker without shipping the full tag dump.
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

func handleJourneyReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64  `json:"account_id"`
		NodeID    string `json:"node_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetJourneyNode(req.AccountID, req.NodeID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

func handleJourneyWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdWipeJourneyNodes(req.AccountID)().(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	invalidateJourneyCache(req.AccountID)
	jsonOK(w, map[string]string{"ok": msg.ok})
}

func handleDeleteTutorials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	// db.go names this cmdDeleteAllTutorials
	msg, ok := cmdDeleteAllTutorials(req.PlayerID)().(msgMutate)
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

func handleWipeCodex(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdWipeCodex(req.AccountID)().(msgMutate)
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

func handleGetCharXP(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchCharXP(id)().(msgCharXP)
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

func handleGrantAllKeystones(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantAllKeystones(req.PlayerID)().(msgMutate)
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

func handleResetAllKeystones(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdResetAllKeystones(req.PlayerID)().(msgMutate)
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

func handleGetPlayerKeystones(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerKeystones(id)().(msgKeystones)
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

func handleGetPlayerSpecs(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerSpecs(id)().(msgSpecs)
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

func handleGrantMaxSpec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64  `json:"player_id"`
		TrackType string `json:"track_type"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdGrantMaxSpec(req.PlayerID, req.TrackType)().(msgMutate)
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

func handleGetPlayerVehicles(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetPlayerVehicles(id)().(msgVehicles)
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

func handleRepairItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRepairItem(req.ID)().(msgMutate)
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

func handleRepairPlayerGear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID int64 `json:"player_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRepairPlayerGear(req.PlayerID)().(msgRepairGear)
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

func handleRepairVehicle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64 `json:"player_id"`
		VehicleID int64 `json:"vehicle_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRepairVehicle(req.PlayerID, req.VehicleID)().(msgRepairVehicle)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]any{"repaired": msg.repaired, "skipped": msg.skipped, "total": msg.total})
}

func handleRefuelVehicle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  int64 `json:"player_id"`
		VehicleID int64 `json:"vehicle_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	msg, ok := cmdRefuelVehicle(req.PlayerID, req.VehicleID)().(msgMutate)
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

func handleGetPartitions(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListPartitions()().(msgPartitions)
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

	var loc teleportLocation
	for _, l := range cheatLocations {
		if l.Name == req.Location {
			loc = l
			break
		}
	}
	if loc.Name == "" {
		jsonErr(w, fmt.Errorf("unknown location: %s", req.Location), 400)
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
	msg, ok := cmdTeleportPlayer(displayName, req.Location)().(msgMutate)
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

func handleGetPlayerEvents(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchEventLog(id)().(msgEvents)
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

func handleGetPlayerDungeons(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdFetchPlayerDungeons(id)().(msgDungeons)
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
