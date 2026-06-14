package main

import (
	"fmt"
	"log"
	"net/http"

	"dune-admin/internal/landsraadbot"
)

// @Summary Landsraad overview — latest term, decree catalogue, and task board
// @Tags landsraad
// @Produce json
// @Success 200 {object} landsraadOverview
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/landsraad [get]
func handleGetLandsraad(w http.ResponseWriter, r *http.Request) {
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	ov, err := cmdFetchLandsraad(r.Context(), globalDB)
	if err != nil {
		log.Printf("handleGetLandsraad: %v", err)
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
	if globalStore == nil {
		jsonErr(w, fmt.Errorf("store not connected"), http.StatusServiceUnavailable)
		return
	}
	cfg, err := getLandsraadBotConfig(globalStore)
	if err != nil {
		log.Printf("handleGetLandsraadBotConfig: %v", err)
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
	if globalStore == nil {
		jsonErr(w, fmt.Errorf("store not connected"), http.StatusServiceUnavailable)
		return
	}
	var req landsraadBotConfig
	if err := decode(r, &req); err != nil {
		jsonErr(w, fmt.Errorf("invalid json"), http.StatusBadRequest)
		return
	}
	if err := saveLandsraadBotConfig(globalStore, req); err != nil {
		log.Printf("handleUpdateLandsraadBotConfig: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	// Notify the running bot to reload its config
	if globalLandsraadBot != nil {
		botCfg := landsraadbot.BotConfig{
			Enabled:              req.Enabled,
			ProgressRate:         req.ProgressRate,
			SimultaneousTargets:  req.SimultaneousTargets,
			TargetCompletionDays: req.TargetCompletionDays,
			AtreidesGuildID:      req.AtreidesGuildID,
			HarkonnenGuildID:     req.HarkonnenGuildID,
			AtreidesStrategy:     req.AtreidesStrategy,
			HarkonnenStrategy:    req.HarkonnenStrategy,
			AtreidesTargetTask:   req.AtreidesTargetTask,
			HarkonnenTargetTask:  req.HarkonnenTargetTask,
			AtreidesTargetDecree: req.AtreidesTargetDecree,
			HarkonnenTargetDecree: req.HarkonnenTargetDecree,
		}
		globalLandsraadBot.ReloadConfig(botCfg)
	} else if req.Enabled {
		startEmbeddedLandsraadBotIfEnabled()
	}
	jsonOK(w, req)
}

// @Summary Reset Landsraad Term
// @Description Force end the current landsraad term
// @Tags landsraad
// @Router /api/v1/landsraad/reset [post]
func handleResetLandsraadTerm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	term, err := fetchLandsraadTerm(ctx, globalDB)
	if err != nil {
		jsonErr(w, fmt.Errorf("fetch term: %w", err), http.StatusInternalServerError)
		return
	}

	_, err = globalDB.Exec(ctx, "SELECT dune.landsraad_force_end_term($1)", term.TermID)
	if err != nil {
		jsonErr(w, fmt.Errorf("reset term: %w", err), http.StatusInternalServerError)
		return
	}

	// Forcefully resolve the winner and election phase immediately so the game server 
	// spawns a brand new term on its next tick, instead of getting stuck in the voting phase
	_, _ = globalDB.Exec(ctx, "SELECT dune.landsraad_determine_winner($1)", term.TermID)
	_, _ = globalDB.Exec(ctx, "SELECT dune.landsraad_collect_votes($1)", term.TermID)

	jsonOK(w, map[string]interface{}{"status": "success", "reset_term_id": term.TermID})
}
