package main

import (
	"fmt"
	"net/http"
	"strconv"
)

type playerStats struct {
	SolarisBal        int64  `json:"solaris_balance"`
	ScripBal          int64  `json:"scrip_balance"`
	SolarisEarned     int64  `json:"solaris_earned"`
	SolarisSpent      int64  `json:"solaris_spent"`
	POIsDiscovered    int    `json:"pois_discovered"`
	StoryMilestones   int    `json:"story_milestones"`
	MaxFactionTier    int    `json:"max_faction_tier"`
	Faction           string `json:"faction"`
	CharXP            int64  `json:"char_xp"`
	SkillPoints       int    `json:"skill_points"`
	TotalPlaytimeSecs int64  `json:"total_playtime_secs"`
	SessionCount      int64  `json:"session_count"`
	AvgSessionSecs    int64  `json:"avg_session_secs"`
	LastSeen          any    `json:"last_seen"`
}

func buildPlayerStats(pg playerPgStats, sess sessionStats) playerStats {
	return playerStats{
		SolarisBal:        pg.SolarisBal,
		ScripBal:          pg.ScripBal,
		SolarisEarned:     pg.SolarisEarned,
		SolarisSpent:      pg.SolarisSpent,
		POIsDiscovered:    pg.POIsDiscovered,
		StoryMilestones:   pg.StoryMilestones,
		MaxFactionTier:    pg.MaxFactionTier,
		Faction:           pg.Faction,
		CharXP:            pg.CharXP,
		SkillPoints:       pg.SkillPoints,
		TotalPlaytimeSecs: sess.TotalPlaytimeSecs,
		SessionCount:      sess.SessionCount,
		AvgSessionSecs:    sess.AvgSessionSecs,
		LastSeen:          pg.LastSeen,
	}
}

func handleGetPlayerStats(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}

	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
		return
	}

	pg, err := cmdFetchPlayerPgStats(r.Context(), db, accountID)
	if err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("fetch player pg stats failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}

	var sess sessionStats
	if globalSessionDB != nil {
		sess, err = getSessionStats(r.Context(), globalSessionDB, storeScopeFromCtx(r), accountID)
		if err != nil {
			componentLog("handlers").Warn().Int64("account_id", accountID).Err(err).Msg("fetch session stats failed")
		}
	}

	jsonOK(w, buildPlayerStats(pg, sess))
}

func handleGetSolarisHistory(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}

	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
		return
	}

	points, err := cmdFetchSolarisHistory(r.Context(), db, accountID)
	if err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("fetch solaris history failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}

	jsonOK(w, points)
}

func handleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	if globalSessionDB == nil {
		jsonErr(w, fmt.Errorf("session tracker not available"), http.StatusServiceUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
		return
	}
	recs, err := getSessionHistory(r.Context(), globalSessionDB, storeScopeFromCtx(r), accountID, 200)
	if err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("fetch session history failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, recs)
}

func handleGetStatSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	if globalSessionDB == nil {
		jsonErr(w, fmt.Errorf("session tracker not available"), http.StatusServiceUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
		return
	}
	snaps, err := getStatSnapshotHistory(r.Context(), globalSessionDB, storeScopeFromCtx(r), accountID, 500)
	if err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("fetch stat snapshot history failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, snaps)
}
