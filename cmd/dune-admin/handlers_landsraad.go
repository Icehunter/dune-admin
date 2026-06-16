package main

import (
	"fmt"
	"net/http"
)

// @Summary Landsraad overview — latest term, decree catalogue, and task board
// @Tags landsraad
// @Produce json
// @Success 200 {object} landsraadOverview
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/landsraad [get]
func handleGetLandsraad(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	ov, err := cmdFetchLandsraad(r.Context(), db)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("fetch landsraad failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, ov)
}

// @Summary Get Landsraad Bot configuration
// @Tags landsraad
// @Produce json
// @Success 200 {object} landsraadBotConfig
// @Failure 500 {object} map[string]string
// @Router /api/v1/landsraad/bot/config [get]
func handleGetLandsraadBotConfig(w http.ResponseWriter, r *http.Request) {
	sc := serverFromCtx(r)
	if sc == nil || sc.DB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	cfg, err := getLandsraadBotConfig(globalStore, sc.StoreScope)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("handleGetLandsraadBotConfig failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, cfg)
}

// @Summary Update Landsraad Bot configuration
// @Tags landsraad
// @Accept json
// @Produce json
// @Param body body landsraadBotConfig true "Bot configuration"
// @Success 200 {object} landsraadBotConfig
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/landsraad/bot/config [put]
func handleUpdateLandsraadBotConfig(w http.ResponseWriter, r *http.Request) {
	sc := serverFromCtx(r)
	if sc == nil || sc.DB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	var req landsraadBotConfig
	if err := decode(r, &req); err != nil {
		jsonErr(w, fmt.Errorf("invalid json"), http.StatusBadRequest)
		return
	}
	if err := saveLandsraadBotConfig(globalStore, sc.StoreScope, req); err != nil {
		componentLog("handlers").Error().Err(err).Msg("handleUpdateLandsraadBotConfig failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	
	// Restart the bot to apply config
	restartServerLandsraadBot(sc)
	
	jsonOK(w, req)
}

// @Summary Reset Landsraad Term
// @Description Force end the current landsraad term
// @Tags landsraad
// @Router /api/v1/landsraad/reset [post]
func handleResetLandsraadTerm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc := serverFromCtx(r)
	if sc == nil || sc.DB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	
	term, err := fetchLandsraadTerm(ctx, sc.DB)
	if err != nil {
		jsonErr(w, fmt.Errorf("fetch term: %w", err), http.StatusInternalServerError)
		return
	}

	_, err = sc.DB.Exec(ctx, "SELECT dune.landsraad_force_end_term($1)", term.TermID)
	if err != nil {
		jsonErr(w, fmt.Errorf("reset term: %w", err), http.StatusInternalServerError)
		return
	}

	// Forcefully resolve the winner and election phase immediately so the game server 
	// spawns a brand new term on its next tick, instead of getting stuck in the voting phase
	_, err = sc.DB.Exec(ctx, "SELECT dune.landsraad_determine_winner($1)", term.TermID)
	if err != nil {
		jsonErr(w, fmt.Errorf("determine winner: %w", err), http.StatusInternalServerError)
		return
	}
	
	_, err = sc.DB.Exec(ctx, "SELECT dune.landsraad_collect_votes($1)", term.TermID)
	if err != nil {
		jsonErr(w, fmt.Errorf("collect votes: %w", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{"status": "success", "reset_term_id": term.TermID})
}
