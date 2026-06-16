package main

import (
	"fmt"
	"net/http"
)

// handleListMaps returns the distinct map names present in dune.actors, for use
// as a dropdown in the event editor and other forms.
func handleListMaps(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	maps, err := cmdFetchDistinctMaps(r.Context(), db)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("fetch distinct maps failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, maps)
}

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
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	markers, err := cmdFetchMapMarkers(r.Context(), db, mapKey)
	if err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("fetch map markers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, markers)
}
