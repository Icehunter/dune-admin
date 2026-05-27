package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

func handleListStorage(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListStorageContainers().(msgStorageContainers)
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
		rows = []storageContainerRow{}
	}
	jsonOK(w, rows)
}

func handleGetStorageItems(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetContainerInventory(id)().(msgContainerInventory)
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

func handleGiveItemToStorage(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	var req struct {
		Template string `json:"template"`
		Qty      int64  `json:"qty"`
		Quality  int64  `json:"quality"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	msg, ok := cmdGiveItemToContainer(id, req.Template, req.Qty, req.Quality)().(msgMutate)
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

func handleGiveItemsToStorage(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	var req struct {
		Items []struct {
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

	for _, item := range req.Items {
		qty := item.Qty
		if qty <= 0 {
			qty = 1
		}
		msg, ok := cmdGiveItemToContainer(id, item.Template, qty, item.Quality)().(msgMutate)
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

func handleStorageOwnerDebug(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("not connected"), 500)
		return
	}
	ctx := context.Background()

	var ownerEntityID int64
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(owner_entity_id,0) FROM dune.placeables WHERE id = $1`, id).Scan(&ownerEntityID)

	var afeEntityID, afeActorID int64
	_ = globalDB.QueryRow(ctx, `SELECT entity_id, actor_id FROM dune.actor_fgl_entities WHERE entity_id = $1 LIMIT 1`, ownerEntityID).Scan(&afeEntityID, &afeActorID)

	var ownerAccountID int64
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(owner_account_id,0) FROM dune.actors WHERE id = $1`, afeActorID).Scan(&ownerAccountID)

	// Alternate path: permission_actor_rank links container actor → player actor
	var parPlayerID int64
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(player_id,0) FROM dune.permission_actor_rank WHERE permission_actor_id = $1 LIMIT 1`, afeActorID).Scan(&parPlayerID)

	var parAccountID int64
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(owner_account_id,0) FROM dune.actors WHERE id = $1`, parPlayerID).Scan(&parAccountID)

	var characterName, funcomID, hexID string
	accountID := ownerAccountID
	if accountID == 0 {
		accountID = parAccountID
	}
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(character_name,'') FROM dune.player_state WHERE account_id = $1`, accountID).Scan(&characterName)
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE(convert_from(encrypted_funcom_id,'UTF8'),'') FROM dune.encrypted_accounts WHERE id = $1`, accountID).Scan(&funcomID)
	_ = globalDB.QueryRow(ctx, `SELECT COALESCE("user",'') FROM dune.accounts WHERE id = $1`, accountID).Scan(&hexID)

	jsonOK(w, map[string]any{
		"container_id":      id,
		"owner_entity_id":   ownerEntityID,
		"afe_entity_id":     afeEntityID,
		"afe_actor_id":      afeActorID,
		"owner_account_id":  ownerAccountID,
		"par_player_id":     parPlayerID,
		"par_account_id":    parAccountID,
		"resolved_account":  accountID,
		"character_name":    characterName,
		"funcom_id":         funcomID,
		"hex_id":            hexID,
	})
}
