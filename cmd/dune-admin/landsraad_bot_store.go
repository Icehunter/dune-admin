package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type landsraadBotConfig struct {
	Enabled               bool    `json:"enabled"`
	ProgressRate          float64 `json:"progress_rate"`
	SimultaneousTargets   int     `json:"simultaneous_targets"`
	TargetCompletionDays  float64 `json:"target_completion_days"`
	AtreidesGuildID       int64   `json:"atreides_guild_id"`
	HarkonnenGuildID      int64   `json:"harkonnen_guild_id"`
	AtreidesStrategy      string  `json:"atreides_strategy"`
	HarkonnenStrategy     string  `json:"harkonnen_strategy"`
	AtreidesTargetTask    int     `json:"atreides_target_task"`
	HarkonnenTargetTask   int     `json:"harkonnen_target_task"`
	AtreidesTargetDecree  int     `json:"atreides_target_decree"`
	HarkonnenTargetDecree int     `json:"harkonnen_target_decree"`
	TickIntervalSeconds   int     `json:"tick_interval_seconds"`
	TickJitterSeconds     int     `json:"tick_jitter_seconds"`
	EnableRaidHours       bool    `json:"enable_raid_hours"`
	RaidStartHour         int     `json:"raid_start_hour"`
	RaidDurationHours     int     `json:"raid_duration_hours"`
	AtreidesTargets       []int   `json:"atreides_targets"`
	HarkonnenTargets      []int   `json:"harkonnen_targets"`
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
		AtreidesTargets:      make([]int, 0),
		HarkonnenTargets:     make([]int, 0),
	}
}

func initLandsraadBotSchema(db *sql.DB) error {
	// Drop old JSON table if it exists
	var hasLegacy bool
	_ = db.QueryRow(`SELECT 1 FROM pragma_table_info('landsraad_bot_config') WHERE name = 'config_json'`).Scan(&hasLegacy)
	if hasLegacy {
		if _, err := db.Exec(`DROP TABLE landsraad_bot_config`); err != nil {
			return fmt.Errorf("drop legacy landsraad_bot_config: %w", err)
		}
	}

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS landsraad_bot_config (
			server_id INTEGER PRIMARY KEY,
			enabled BOOLEAN NOT NULL DEFAULT 0,
			progress_rate REAL NOT NULL DEFAULT 100.0,
			simultaneous_targets INTEGER NOT NULL DEFAULT 1,
			target_completion_days REAL NOT NULL DEFAULT 3.0,
			atreides_guild_id INTEGER NOT NULL DEFAULT 0,
			harkonnen_guild_id INTEGER NOT NULL DEFAULT 0,
			atreides_strategy TEXT NOT NULL DEFAULT 'auto',
			harkonnen_strategy TEXT NOT NULL DEFAULT 'auto',
			atreides_target_task INTEGER NOT NULL DEFAULT 0,
			harkonnen_target_task INTEGER NOT NULL DEFAULT 0,
			atreides_target_decree INTEGER NOT NULL DEFAULT 0,
			harkonnen_target_decree INTEGER NOT NULL DEFAULT 0,
			tick_interval_seconds INTEGER NOT NULL DEFAULT 300,
			tick_jitter_seconds INTEGER NOT NULL DEFAULT 10,
			enable_raid_hours BOOLEAN NOT NULL DEFAULT 0,
			raid_start_hour INTEGER NOT NULL DEFAULT 18,
			raid_duration_hours INTEGER NOT NULL DEFAULT 4,
			FOREIGN KEY(server_id) REFERENCES servers(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return err
	}

	var hasAtreidesTargets bool
	_ = db.QueryRow(`SELECT 1 FROM pragma_table_info('landsraad_bot_config') WHERE name = 'atreides_targets'`).Scan(&hasAtreidesTargets)
	if !hasAtreidesTargets {
		if _, err := db.Exec(`ALTER TABLE landsraad_bot_config ADD COLUMN atreides_targets TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("add atreides_targets: %w", err)
		}
		if _, err := db.Exec(`ALTER TABLE landsraad_bot_config ADD COLUMN harkonnen_targets TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("add harkonnen_targets: %w", err)
		}
	}

	return nil
}

func getLandsraadBotConfig(db *sql.DB, serverID int) (landsraadBotConfig, error) {
	var cfg landsraadBotConfig
	var atreidesTargetsJSON, harkonnenTargetsJSON string
	err := db.QueryRow(`
		SELECT 
			enabled, progress_rate, simultaneous_targets, target_completion_days,
			atreides_guild_id, harkonnen_guild_id, atreides_strategy, harkonnen_strategy,
			atreides_target_task, harkonnen_target_task, atreides_target_decree, harkonnen_target_decree,
			tick_interval_seconds, tick_jitter_seconds, enable_raid_hours, raid_start_hour, raid_duration_hours,
			atreides_targets, harkonnen_targets
		FROM landsraad_bot_config 
		WHERE server_id = $1
	`, serverID).Scan(
		&cfg.Enabled, &cfg.ProgressRate, &cfg.SimultaneousTargets, &cfg.TargetCompletionDays,
		&cfg.AtreidesGuildID, &cfg.HarkonnenGuildID, &cfg.AtreidesStrategy, &cfg.HarkonnenStrategy,
		&cfg.AtreidesTargetTask, &cfg.HarkonnenTargetTask, &cfg.AtreidesTargetDecree, &cfg.HarkonnenTargetDecree,
		&cfg.TickIntervalSeconds, &cfg.TickJitterSeconds, &cfg.EnableRaidHours, &cfg.RaidStartHour, &cfg.RaidDurationHours,
		&atreidesTargetsJSON, &harkonnenTargetsJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultLandsraadBotConfig(), nil
		}
		return cfg, fmt.Errorf("read landsraad_bot_config: %w", err)
	}

	if atreidesTargetsJSON != "" {
		_ = json.Unmarshal([]byte(atreidesTargetsJSON), &cfg.AtreidesTargets)
	}
	if harkonnenTargetsJSON != "" {
		_ = json.Unmarshal([]byte(harkonnenTargetsJSON), &cfg.HarkonnenTargets)
	}

	if cfg.AtreidesTargets == nil {
		cfg.AtreidesTargets = make([]int, 0)
	}
	if cfg.HarkonnenTargets == nil {
		cfg.HarkonnenTargets = make([]int, 0)
	}

	return cfg, nil
}

func saveLandsraadBotConfig(db *sql.DB, serverID int, cfg landsraadBotConfig) error {
	bAtreides, _ := json.Marshal(cfg.AtreidesTargets)
	bHarkonnen, _ := json.Marshal(cfg.HarkonnenTargets)
	if bAtreides == nil {
		bAtreides = []byte("[]")
	}
	if bHarkonnen == nil {
		bHarkonnen = []byte("[]")
	}

	_, err := db.Exec(`
		INSERT INTO landsraad_bot_config (
			server_id, enabled, progress_rate, simultaneous_targets, target_completion_days,
			atreides_guild_id, harkonnen_guild_id, atreides_strategy, harkonnen_strategy,
			atreides_target_task, harkonnen_target_task, atreides_target_decree, harkonnen_target_decree,
			tick_interval_seconds, tick_jitter_seconds, enable_raid_hours, raid_start_hour, raid_duration_hours,
			atreides_targets, harkonnen_targets
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT(server_id) DO UPDATE SET 
			enabled = excluded.enabled,
			progress_rate = excluded.progress_rate,
			simultaneous_targets = excluded.simultaneous_targets,
			target_completion_days = excluded.target_completion_days,
			atreides_guild_id = excluded.atreides_guild_id,
			harkonnen_guild_id = excluded.harkonnen_guild_id,
			atreides_strategy = excluded.atreides_strategy,
			harkonnen_strategy = excluded.harkonnen_strategy,
			atreides_target_task = excluded.atreides_target_task,
			harkonnen_target_task = excluded.harkonnen_target_task,
			atreides_target_decree = excluded.atreides_target_decree,
			harkonnen_target_decree = excluded.harkonnen_target_decree,
			tick_interval_seconds = excluded.tick_interval_seconds,
			tick_jitter_seconds = excluded.tick_jitter_seconds,
			enable_raid_hours = excluded.enable_raid_hours,
			raid_start_hour = excluded.raid_start_hour,
			raid_duration_hours = excluded.raid_duration_hours,
			atreides_targets = excluded.atreides_targets,
			harkonnen_targets = excluded.harkonnen_targets
	`,
		serverID, cfg.Enabled, cfg.ProgressRate, cfg.SimultaneousTargets, cfg.TargetCompletionDays,
		cfg.AtreidesGuildID, cfg.HarkonnenGuildID, cfg.AtreidesStrategy, cfg.HarkonnenStrategy,
		cfg.AtreidesTargetTask, cfg.HarkonnenTargetTask, cfg.AtreidesTargetDecree, cfg.HarkonnenTargetDecree,
		cfg.TickIntervalSeconds, cfg.TickJitterSeconds, cfg.EnableRaidHours, cfg.RaidStartHour, cfg.RaidDurationHours,
		string(bAtreides), string(bHarkonnen),
	)
	return err
}
