package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	msg, ok := cmdGiveItem(req.PlayerID, req.Template, req.Qty, req.Quality)().(msgMutate)
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
		PlayerID int64 `json:"player_id"`
		Amount   int64 `json:"amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
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
	jsonOK(w, map[string]string{"ok": msg.ok})
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
	jsonOK(w, map[string]string{"ok": msg.ok})
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
	msg, ok := cmdTeleportPlayer(req.FLSID, req.Location)().(msgMutate)
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

