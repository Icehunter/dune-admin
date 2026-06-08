package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// @Summary Get the scheduled-restart config + next restart time
// @Tags scheduled-restarts
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/scheduled-restarts [get]
func handleGetScheduledRestarts(w http.ResponseWriter, _ *http.Request) {
	cfg := getScheduledRestartConfig()
	resp := map[string]any{
		"enabled":      cfg.Enabled,
		"timezone":     cfg.Timezone,
		"rules":        cfg.Rules,
		"warn_minutes": cfg.WarnMinutes,
		"last_fired":   cfg.LastFired,
	}
	if cfg.Enabled {
		if next, ok := nextRestartAt(time.Now(), cfg.Rules, restartLocation(cfg.Timezone)); ok {
			resp["next_restart"] = next.Format(time.RFC3339)
		}
	}
	jsonOK(w, resp)
}

func validateRestartRules(rules []restartRule) error {
	for _, r := range rules {
		if _, _, ok := parseHHMM(r.Time); !ok {
			return fmt.Errorf("invalid time %q (expected HH:MM)", r.Time)
		}
		if len(r.Days) == 0 {
			return fmt.Errorf("a restart rule has no days selected")
		}
		for _, d := range r.Days {
			if d < 0 || d > 6 {
				return fmt.Errorf("invalid weekday %d (expected 0-6)", d)
			}
		}
	}
	return nil
}

// @Summary Update the scheduled-restart config
// @Tags scheduled-restarts
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/scheduled-restarts [put]
func handleUpdateScheduledRestarts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled     bool          `json:"enabled"`
		Timezone    string        `json:"timezone"`
		Rules       []restartRule `json:"rules"`
		WarnMinutes int           `json:"warn_minutes"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := validateRestartRules(body.Rules); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.Timezone != "" {
		if _, err := time.LoadLocation(body.Timezone); err != nil {
			jsonErr(w, fmt.Errorf("invalid timezone %q", body.Timezone), http.StatusBadRequest)
			return
		}
	}
	cur := getScheduledRestartConfig() // preserve last_fired watermark
	cur.Enabled = body.Enabled
	cur.Timezone = body.Timezone
	cur.Rules = body.Rules
	cur.WarnMinutes = body.WarnMinutes
	if err := saveScheduledRestartConfig(cur); err != nil {
		log.Printf("handleUpdateScheduledRestarts: %v", err)
		jsonErr(w, fmt.Errorf("could not save schedule"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "schedule saved"})
}

// @Summary Skip the next scheduled restart (without disabling the schedule)
// @Tags scheduled-restarts
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/scheduled-restarts/skip-next [post]
func handleSkipNextRestart(w http.ResponseWriter, _ *http.Request) {
	cfg := getScheduledRestartConfig()
	next, ok := nextRestartAt(time.Now(), cfg.Rules, restartLocation(cfg.Timezone))
	if !ok {
		jsonErr(w, fmt.Errorf("no upcoming restart to skip"), http.StatusBadRequest)
		return
	}
	// Advancing the watermark to the next occurrence makes the scheduler treat it
	// as already handled — neither warned nor fired.
	setRestartLastFired(next.Unix())
	jsonOK(w, map[string]string{"ok": "next restart skipped"})
}
