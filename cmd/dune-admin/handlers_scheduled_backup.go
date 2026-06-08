package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// @Summary Get the scheduled-backup config + next backup time
// @Tags scheduled-backups
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/scheduled-backups [get]
func handleGetScheduledBackups(w http.ResponseWriter, _ *http.Request) {
	cfg := getScheduledBackupConfig()
	resp := map[string]any{
		"enabled":    cfg.Enabled,
		"timezone":   cfg.Timezone,
		"rules":      cfg.Rules,
		"keep_n":     cfg.KeepN,
		"last_fired": cfg.LastFired,
	}
	if cfg.Enabled {
		if next, ok := nextBackupAt(time.Now(), cfg.Rules, restartLocation(cfg.Timezone)); ok {
			resp["next_backup"] = next.Format(time.RFC3339)
		}
	}
	jsonOK(w, resp)
}

func validateBackupRules(rules []backupRule) error {
	for _, r := range rules {
		if _, _, ok := parseHHMM(r.Time); !ok {
			return fmt.Errorf("invalid time %q (expected HH:MM)", r.Time)
		}
		if len(r.Days) == 0 {
			return fmt.Errorf("a backup rule has no days selected")
		}
		for _, d := range r.Days {
			if d < 0 || d > 6 {
				return fmt.Errorf("invalid weekday %d (expected 0-6)", d)
			}
		}
	}
	return nil
}

// @Summary Update the scheduled-backup config
// @Tags scheduled-backups
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/scheduled-backups [put]
func handleUpdateScheduledBackups(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled  bool         `json:"enabled"`
		Timezone string       `json:"timezone"`
		Rules    []backupRule `json:"rules"`
		KeepN    int          `json:"keep_n"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := validateBackupRules(body.Rules); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.Timezone != "" {
		if _, err := time.LoadLocation(body.Timezone); err != nil {
			jsonErr(w, fmt.Errorf("invalid timezone %q", body.Timezone), http.StatusBadRequest)
			return
		}
	}
	cur := getScheduledBackupConfig() // preserve last_fired watermark
	cur.Enabled = body.Enabled
	cur.Timezone = body.Timezone
	cur.Rules = body.Rules
	cur.KeepN = body.KeepN
	if err := saveScheduledBackupConfig(cur); err != nil {
		log.Printf("handleUpdateScheduledBackups: %v", err)
		jsonErr(w, fmt.Errorf("could not save schedule"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "schedule saved"})
}
