package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

// parseDimensionParam reads the optional ?dimension= query parameter shared by
// /map/markers and /map/dimensions (#274). Absence (or an empty string) means
// "all dimensions" and returns (nil, nil) — preserving pre-#274 behaviour for
// any caller that omits the param. A present value must be a non-negative
// integer; dimension 0 is a real, distinct selection, not "unset".
func parseDimensionParam(r *http.Request) (*int, error) {
	raw := r.URL.Query().Get("dimension")
	if raw == "" {
		return nil, nil
	}
	d, err := strconv.Atoi(raw)
	if err != nil || d < 0 {
		return nil, fmt.Errorf("invalid dimension: %q", raw)
	}
	return &d, nil
}

// handleGetMapMarkers returns the Live Map markers (players + vehicles, plus
// bases in Phase 2b) for the requested map. The ?map= input is validated before
// the DB is touched, so bad input fails fast with 400 and a valid map with no DB
// connection surfaces 503. The optional ?dimension= narrows results to a single
// dimension_index (#274); omitting it preserves the pre-#274 all-dimensions
// behaviour.
//
// @Summary Live Map markers for a map
// @Tags map
// @Produce json
// @Param map query string true "Map key (HaggaBasin | DeepDesert)"
// @Param dimension query int false "Dimension index to filter to (omit for all dimensions)"
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
	dimension, err := parseDimensionParam(r)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	markers, err := cmdFetchMapMarkers(r.Context(), db, mapKey, dimension)
	if err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("fetch map markers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, markers)
}

// handleGetMapDimensions returns the distinct dimension indices available for
// the requested map, so the frontend can populate a dimension selector (#274).
//
// @Summary Live Map available dimensions for a map
// @Tags map
// @Produce json
// @Param map query string true "Map key (HaggaBasin | DeepDesert)"
// @Success 200 {array} int
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/map/dimensions [get]
func handleGetMapDimensions(w http.ResponseWriter, r *http.Request) {
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
	dims, err := cmdFetchMapDimensions(r.Context(), db, mapKey)
	if err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("fetch map dimensions failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, dims)
}

func handleGetMapCalibration(w http.ResponseWriter, r *http.Request) {
	mapKey := r.URL.Query().Get("map")
	if err := validateMapKey(mapKey); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if globalStore == nil {
		jsonErr(w, errors.New("store unavailable"), http.StatusServiceUnavailable)
		return
	}
	serverID := storeScopeFromCtx(r)
	c, ok, err := loadMapCalibration(globalStore, serverID, mapKey)
	if err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("load map calibration failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonErr(w, fmt.Errorf("no calibration for %s", mapKey), http.StatusNotFound)
		return
	}
	jsonOK(w, c)
}

func handlePutMapCalibration(w http.ResponseWriter, r *http.Request) {
	mapKey := r.URL.Query().Get("map")
	if err := validateMapKey(mapKey); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if globalStore == nil {
		jsonErr(w, errors.New("store unavailable"), http.StatusServiceUnavailable)
		return
	}
	var c mapCalibration
	if err := decode(r, &c); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	c.MapKey = mapKey
	serverID := storeScopeFromCtx(r)
	if err := saveMapCalibration(globalStore, serverID, c); err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("save map calibration failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, c)
}

// handleDeleteMapCalibration removes a saved calibration so the map reverts to
// its built-in default bounds. Deleting a map with no saved calibration is a
// no-op and still returns 200.
func handleDeleteMapCalibration(w http.ResponseWriter, r *http.Request) {
	mapKey := r.URL.Query().Get("map")
	if err := validateMapKey(mapKey); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if globalStore == nil {
		jsonErr(w, errors.New("store unavailable"), http.StatusServiceUnavailable)
		return
	}
	serverID := storeScopeFromCtx(r)
	if err := deleteMapCalibration(globalStore, serverID, mapKey); err != nil {
		componentLog("handlers").Error().Str("map_key", mapKey).Err(err).Msg("delete map calibration failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]bool{"deleted": true})
}
