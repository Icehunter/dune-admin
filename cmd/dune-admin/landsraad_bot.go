package main

import (
	"context"
	"encoding/json"

	"dune-admin/internal/landsraadbot"
)

// serverLandsraadBotEnabled reports whether the bot is enabled for the server.
func serverLandsraadBotEnabled(sc *ServerContext) bool {
	if sc == nil || sc.DB == nil {
		return false
	}
	cfg, err := getLandsraadBotConfig(globalStore, sc.StoreScope)
	if err != nil {
		componentLog("landsraadbot").Error().Err(err).Msgf("failed to get config for server %s", sc.ID)
		return false
	}
	return cfg.Enabled
}

// startServerLandsraadBot starts sc's embedded landsraad bot when its per-server toggle
// is enabled and it has a live DB connection.
func startServerLandsraadBot(sc *ServerContext) {
	if sc == nil {
		return
	}
	if sc.DB == nil {
		return
	}
	
	cfg, err := getLandsraadBotConfig(globalStore, sc.StoreScope)
	if err != nil {
		componentLog("landsraadbot").Error().Err(err).Msgf("failed to load bot config for server %s", sc.ID)
		return
	}
	sc.LandsraadBotConfigured = cfg.Enabled
	
	if !cfg.Enabled {
		return
	}
	
	botCtx, botCancel := context.WithCancel(context.Background())
	
	botCfg := landsraadbot.BotConfig{
		Enabled:              cfg.Enabled,
		ProgressRate:         cfg.ProgressRate,
		SimultaneousTargets:  cfg.SimultaneousTargets,
		TargetCompletionDays: cfg.TargetCompletionDays,
		AtreidesGuildID:      cfg.AtreidesGuildID,
		HarkonnenGuildID:     cfg.HarkonnenGuildID,
		AtreidesStrategy:     cfg.AtreidesStrategy,
		HarkonnenStrategy:    cfg.HarkonnenStrategy,
		AtreidesTargetTask:   cfg.AtreidesTargetTask,
		HarkonnenTargetTask:  cfg.HarkonnenTargetTask,
		AtreidesTargetDecree: cfg.AtreidesTargetDecree,
		HarkonnenTargetDecree: cfg.HarkonnenTargetDecree,
		TickIntervalSeconds:   cfg.TickIntervalSeconds,
		TickJitterSeconds:     cfg.TickJitterSeconds,
		AtreidesTargets:       cfg.AtreidesTargets,
		HarkonnenTargets:      cfg.HarkonnenTargets,
		SaveTargets: func(ctx context.Context, atreides []int, harkonnen []int) {
			bAtreides, _ := json.Marshal(atreides)
			bHarkonnen, _ := json.Marshal(harkonnen)
			if bAtreides == nil {
				bAtreides = []byte("[]")
			}
			if bHarkonnen == nil {
				bHarkonnen = []byte("[]")
			}
			_, err := globalStore.ExecContext(ctx, `
				UPDATE landsraad_bot_config 
				SET atreides_targets = $1, harkonnen_targets = $2 
				WHERE server_id = $3
			`, string(bAtreides), string(bHarkonnen), sc.StoreScope)
			if err != nil {
				componentLog("landsraadbot").Error().Err(err).Msgf("failed to save targets for server %s", sc.ID)
			}
		},
	}

	inst, err := landsraadbot.Run(botCtx, sc.DB, botCfg)
	if err != nil {
		componentLog("landsraadbot").Error().Err(err).Msgf("failed to start bot for server %s", sc.ID)
		botCancel()
		return
	}
	
	sc.LandsraadBot = inst
	sc.LandsraadBotCancel = botCancel
}

// stopServerLandsraadBot cancels sc's bot goroutines and clears its bot fields.
func stopServerLandsraadBot(sc *ServerContext) {
	if sc == nil {
		return
	}
	if sc.LandsraadBotCancel != nil {
		sc.LandsraadBotCancel()
		sc.LandsraadBotCancel = nil
	}
	if sc.LandsraadBot != nil {
		sc.LandsraadBot.Stop()
		sc.LandsraadBot = nil
	}
}

// restartServerLandsraadBot stops then (re)starts sc's bot.
func restartServerLandsraadBot(sc *ServerContext) {
	stopServerLandsraadBot(sc)
	startServerLandsraadBot(sc)
}

// restartAllServerLandsraadBots restarts every registered server's bot.
func restartAllServerLandsraadBots() {
	for _, sc := range globalRegistry.All() {
		restartServerLandsraadBot(sc)
	}
}

// stopAllServerLandsraadBots stops every registered server's bot.
func stopAllServerLandsraadBots() {
	for _, sc := range globalRegistry.All() {
		stopServerLandsraadBot(sc)
	}
}
