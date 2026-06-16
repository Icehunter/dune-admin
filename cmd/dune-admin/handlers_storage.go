package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// @Summary List all storage containers
// @Tags storage
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/storage [get]
func handleListStorage(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListStorageContainers(dbFromCtx(r)).(msgStorageContainers)
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

// @Summary Get items inside a storage container
// @Tags storage
// @Produce json
// @Param id path int true "Container ID"
// @Success 200 {array} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/storage/{id}/items [get]
func handleGetStorageItems(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	msg, ok := cmdGetContainerInventory(dbFromCtx(r), id)().(msgContainerInventory)
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

// @Summary Give a single item to a storage container
// @Tags storage
// @Accept json
// @Produce json
// @Param id path int true "Container ID"
// @Param body body object true "Item to give" SchemaExample({"template": "ItemTemplate_C", "qty": 1, "quality": 0})
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/storage/{id}/give-item [post]
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

	msg, ok := cmdGiveItemToContainer(dbFromCtx(r), id, req.Template, req.Qty, req.Quality)().(msgMutate)
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

// @Summary Give multiple items to a storage container
// @Tags storage
// @Accept json
// @Produce json
// @Param id path int true "Container ID"
// @Param body body object true "Items to give" SchemaExample({"items": [{"template": "ItemTemplate_C", "qty": 1, "quality": 0}]})
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/storage/{id}/give-items [post]
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
		msg, ok := cmdGiveItemToContainer(dbFromCtx(r), id, item.Template, qty, item.Quality)().(msgMutate)
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

// @Summary Debug ownership chain for a storage container
// @Tags storage
// @Produce json
// @Param id path int true "Container ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/storage/{id}/owner-debug [get]
func handleStorageOwnerDebug(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("not connected"), 500)
		return
	}
	ctx := context.Background()

	var ownerEntityID int64
	_ = db.QueryRow(ctx, `SELECT COALESCE(owner_entity_id,0) FROM dune.placeables WHERE id = $1`, id).Scan(&ownerEntityID)

	var afeEntityID, afeActorID int64
	_ = db.QueryRow(ctx, `SELECT entity_id, actor_id FROM dune.actor_fgl_entities WHERE entity_id = $1 LIMIT 1`, ownerEntityID).Scan(&afeEntityID, &afeActorID)

	var ownerAccountID int64
	_ = db.QueryRow(ctx, `SELECT COALESCE(owner_account_id,0) FROM dune.actors WHERE id = $1`, afeActorID).Scan(&ownerAccountID)

	// Alternate path: permission_actor_rank links container actor → player actor
	var parPlayerID int64
	_ = db.QueryRow(ctx, `SELECT COALESCE(player_id,0) FROM dune.permission_actor_rank WHERE permission_actor_id = $1 LIMIT 1`, afeActorID).Scan(&parPlayerID)

	var parAccountID int64
	_ = db.QueryRow(ctx, `SELECT COALESCE(owner_account_id,0) FROM dune.actors WHERE id = $1`, parPlayerID).Scan(&parAccountID)

	var characterName, funcomID, hexID string
	accountID := ownerAccountID
	if accountID == 0 {
		accountID = parAccountID
	}
	_ = db.QueryRow(ctx, `SELECT COALESCE(character_name,'') FROM dune.player_state WHERE account_id = $1`, accountID).Scan(&characterName)
	_ = db.QueryRow(ctx, `SELECT COALESCE(convert_from(encrypted_funcom_id,'UTF8'),'') FROM dune.encrypted_accounts WHERE id = $1`, accountID).Scan(&funcomID)
	_ = db.QueryRow(ctx, `SELECT COALESCE("user",'') FROM dune.accounts WHERE id = $1`, accountID).Scan(&hexID)

	jsonOK(w, map[string]any{
		"container_id":     id,
		"owner_entity_id":  ownerEntityID,
		"afe_entity_id":    afeEntityID,
		"afe_actor_id":     afeActorID,
		"owner_account_id": ownerAccountID,
		"par_player_id":    parPlayerID,
		"par_account_id":   parAccountID,
		"resolved_account": accountID,
		"character_name":   characterName,
		"funcom_id":        funcomID,
		"hex_id":           hexID,
	})
}
