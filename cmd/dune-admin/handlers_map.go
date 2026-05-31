package main

import (
	"fmt"
	"log"
	"net/http"
)

// handleGetMapMarkers returns the Live Map markers (players + vehicles, plus
// bases in Phase 2b) for the requested map. The ?map= input is validated before
// the DB is touched, so bad input fails fast with 400 and a valid map with no DB
// connection surfaces 503.
//
// @Summary Live Map markers for a map
// @Tags map
// @Produce json
// @Param map query string true "Map key (HaggaBasin | DeepDesert)"
// @Success 200 {array} mapMarker
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/map/markers [get]
func handleGetMapMarkers(w http.ResponseWriter, r *http.Request) {
	mapKey := r.URL.Query().Get("map")
	if err := validateMapKey(mapKey); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	markers, err := cmdFetchMapMarkers(r.Context(), globalDB, mapKey)
	if err != nil {
		log.Printf("handleGetMapMarkers: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, markers)
}
