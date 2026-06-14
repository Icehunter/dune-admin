package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type landsraadBotConfig struct {
	Enabled              bool    `json:"enabled"`
	ProgressRate         float64 `json:"progress_rate"`
	SimultaneousTargets  int     `json:"simultaneous_targets"`
	TargetCompletionDays float64 `json:"target_completion_days"`
	AtreidesGuildID      int64   `json:"atreides_guild_id"`
	HarkonnenGuildID     int64   `json:"harkonnen_guild_id"`
	AtreidesStrategy     string  `json:"atreides_strategy"`
	HarkonnenStrategy    string  `json:"harkonnen_strategy"`
	AtreidesTargetTask   int     `json:"atreides_target_task"`
	HarkonnenTargetTask  int     `json:"harkonnen_target_task"`
	AtreidesTargetDecree int     `json:"atreides_target_decree"`
	HarkonnenTargetDecree int    `json:"harkonnen_target_decree"`
	TickIntervalSeconds   int    `json:"tick_interval_seconds"`
	TickJitterSeconds     int    `json:"tick_jitter_seconds"`
	EnableRaidHours       bool   `json:"enable_raid_hours"`
	RaidStartHour         int    `json:"raid_start_hour"`
	RaidDurationHours     int    `json:"raid_duration_hours"`
}

func defaultLandsraadBotConfig() landsraadBotConfig {
	return landsraadBotConfig{
		Enabled:              false,
		ProgressRate:         100.0,
		SimultaneousTargets:  1,
		TargetCompletionDays: 3.0,
		AtreidesStrategy:     "auto",
		HarkonnenStrategy:    "auto",
		TickIntervalSeconds:  300,
		TickJitterSeconds:    10,
		EnableRaidHours:      false,
		RaidStartHour:        18,
		RaidDurationHours:    4,
	}
}

func initLandsraadBotSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS landsraad_bot_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			config_json TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	
	// Seed default config if empty
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM landsraad_bot_config`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		b, _ := json.Marshal(defaultLandsraadBotConfig())
		_, err = db.Exec(`INSERT INTO landsraad_bot_config (id, config_json) VALUES (1, ?)`, string(b))
		if err != nil {
			return err
		}
	}
	return nil
}

func getLandsraadBotConfig(db *sql.DB) (landsraadBotConfig, error) {
	var cfg landsraadBotConfig
	var j string
	err := db.QueryRow(`SELECT config_json FROM landsraad_bot_config WHERE id = 1`).Scan(&j)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultLandsraadBotConfig(), nil
		}
		return cfg, fmt.Errorf("read landsraad_bot_config: %w", err)
	}
	if err := json.Unmarshal([]byte(j), &cfg); err != nil {
		return cfg, fmt.Errorf("parse landsraad_bot_config: %w", err)
	}
	return cfg, nil
}

func saveLandsraadBotConfig(db *sql.DB, cfg landsraadBotConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO landsraad_bot_config (id, config_json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json
	`, string(b))
	return err
}
