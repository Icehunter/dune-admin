package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// givePacksConfigResponse is the shape of the GET and PUT /give-packs/config
// endpoints. The whole pack library is transferred in one payload.
type givePacksConfigResponse struct {
	Packs []givePack `json:"packs"`
}

// handleGetGivePacksConfig returns the current operator-configured pack library.
// When the store has no row (first boot after seeding was skipped or failed),
// it returns an empty list rather than erroring.
func handleGetGivePacksConfig(w http.ResponseWriter, _ *http.Request) {
	if givePacksStoreDB == nil {
		jsonErr(w, fmt.Errorf("give-packs store not available"), http.StatusServiceUnavailable)
		return
	}
	_, packsJSON, ok, err := givePacksStoreDB.loadConfig()
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("load give-packs config failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	packs := make([]givePack, 0)
	if ok && packsJSON != "" && packsJSON != "null" {
		if err := json.Unmarshal([]byte(packsJSON), &packs); err != nil {
			componentLog("handlers").Error().Err(err).Msg("unmarshal give-packs failed")
			jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
			return
		}
	}
	jsonOK(w, givePacksConfigResponse{Packs: packs})
}

// handlePutGivePacksConfig replaces the operator's pack library. Validates the
// incoming packs and persists with base_packs_loaded=true so startup never
// re-seeds (requirement: deleting all packs must stay empty).
func handlePutGivePacksConfig(w http.ResponseWriter, r *http.Request) {
	if givePacksStoreDB == nil {
		jsonErr(w, fmt.Errorf("give-packs store not available"), http.StatusServiceUnavailable)
		return
	}
	var req givePacksConfigResponse
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if req.Packs == nil {
		req.Packs = []givePack{}
	}
	if err := validateGivePacks(req.Packs); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	packsJSON, err := json.Marshal(req.Packs)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("marshal give-packs failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	// Always persist with base_packs_loaded=true — this is a deliberate operator
	// action, including an explicit empty list.
	if err := givePacksStoreDB.saveConfig(string(packsJSON), true); err != nil {
		componentLog("handlers").Error().Err(err).Msg("save give-packs config failed")
		jsonErr(w, fmt.Errorf("failed to save packs"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, givePacksConfigResponse{Packs: req.Packs})
}
