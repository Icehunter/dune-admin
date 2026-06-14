package main

import (
	"context"
	"fmt"
	"os"

	"dune-admin/internal/landsraadbot"
)

var globalLandsraadBot *landsraadbot.Instance

func startEmbeddedLandsraadBotIfEnabled() {
	if globalStore == nil || globalDB == nil {
		return
	}
	cfg, err := getLandsraadBotConfig(globalStore)
	if err != nil {
		fmt.Fprintf(os.Stderr, "landsraadbot: config load failed: %v\n", err)
		return
	}
	if !cfg.Enabled {
		return
	}
	
	// Create bot config mapped from DB
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
	}

	inst, err := landsraadbot.Run(context.Background(), globalDB, botCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "landsraadbot: startup failed: %v\n", err)
		return
	}
	globalLandsraadBot = inst
}

func stopLandsraadBot() {
	if globalLandsraadBot != nil {
		globalLandsraadBot.Stop()
	}
}
