package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"dune-admin/internal/marketbot"
)

// Per-server market bot lifecycle.
//
// The market bot's enable toggle is PER SERVER (ServerConfig.MarketBotEnabled);
// the rest of the configuration (intervals, thresholds, cache base path, item
// data) is GLOBAL/shared in appConfig. Each enabled server runs its own bot
// instance against its own DB pool, with per-server cache/state files so two
// bots never write the same SQLite file.

// serverMarketBotEnabled reports whether the per-server toggle is on. Unset
// (nil) means OFF — the bot is explicit opt-in per server.
func serverMarketBotEnabled(sc ServerConfig) bool {
	return sc.MarketBotEnabled != nil && *sc.MarketBotEnabled
}

// suffixPath inserts "-<id>" before the file extension (or at the end if none).
func suffixPath(path, id string) string {
	ext := filepath.Ext(path)
	return path[:len(path)-len(ext)] + "-" + id + ext
}

// serverBotPaths derives per-server cache and state file paths from the shared
// global base paths, so concurrent bots never share a SQLite cache/state file.
func serverBotPaths(gcfg appConfig, serverID string) (cacheDB, statePath string) {
	base := gcfg.MarketBotCacheDB
	if base == "" {
		base = filepath.Join(configDir(), "market-bot-cache.db")
	}
	st := gcfg.MarketBotState
	if st == "" {
		st = filepath.Join(configDir(), "market-bot-state.json")
	}
	return suffixPath(base, serverID), suffixPath(st, serverID)
}

// resolveBotItemData returns the shared item-data path, falling back to the
// standard search locations when unset or unreadable.
func resolveBotItemData(gcfg appConfig) string {
	item := gcfg.MarketBotItemData
	if item == "" {
		if itemDataPath != "" {
			item = itemDataPath
		} else {
			item = resolveItemDataPath()
		}
	}
	return usableItemDataPath(item)
}

// startServerMarketBot starts sc's embedded market bot when its per-server toggle
// is enabled and it has a live DB connection. It is a no-op when disabled or when
// no DB is connected (the fresh/unconfigured-environment case) — in the latter
// case BotConfigured is still set so status reports "configured, not running".
// Callers must stop any existing bot first (use restartServerMarketBot).
func startServerMarketBot(sc *ServerContext, gcfg appConfig) {
	if sc == nil || !serverMarketBotEnabled(sc.Cfg) {
		return
	}
	sc.BotConfigured = true
	if sc.DB == nil {
		// Enabled but no DB — never open a speculative connection here. The bot
		// starts on the next successful (re)connect for this server.
		return
	}
	cacheDB, statePath := serverBotPaths(gcfg, sc.ID)
	botCtx, botCancel := context.WithCancel(context.Background())
	inst, err := marketbot.Run(botCtx, marketbot.BotConfig{
		DBPool:       sc.DB,
		DBSchema:     sc.Cfg.DBSchema,
		CacheDB:      cacheDB,
		StatePath:    statePath,
		ItemDataPath: resolveBotItemData(gcfg),
		BuyInterval:  parseDurString(gcfg.MarketBotBuyInt, 5*time.Minute),
		ListInterval: parseDurString(gcfg.MarketBotListInt, 30*time.Minute),
		BuyThreshold: gcfg.MarketBotThresh,
		MaxBuys:      gcfg.MarketBotMaxBuys,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "market-bot[%s]: startup failed: %v\n", sc.ID, err)
		botCancel()
		return
	}
	sc.Bot = inst
	sc.BotCancel = botCancel
}

// stopServerMarketBot cancels sc's bot goroutines and clears its bot fields.
func stopServerMarketBot(sc *ServerContext) {
	if sc == nil {
		return
	}
	if sc.BotCancel != nil {
		sc.BotCancel()
		sc.BotCancel = nil
	}
	sc.Bot = nil
}

// restartServerMarketBot stops then (re)starts sc's bot, picking up the latest
// per-server toggle and global tuning.
func restartServerMarketBot(sc *ServerContext, gcfg appConfig) {
	stopServerMarketBot(sc)
	startServerMarketBot(sc, gcfg)
}

// restartAllServerMarketBots restarts every registered server's bot. Called at
// boot and after a global market-bot config change so new tuning takes effect.
func restartAllServerMarketBots(gcfg appConfig) {
	for _, sc := range globalRegistry.All() {
		restartServerMarketBot(sc, gcfg)
	}
}

// stopAllServerMarketBots stops every registered server's bot (process shutdown,
// or before tearing down DB pools on a legacy reconnect).
func stopAllServerMarketBots() {
	for _, sc := range globalRegistry.All() {
		stopServerMarketBot(sc)
	}
}

// activeBot returns the market bot for the request's server context, falling back
// to the active server's bot. Nil when no bot is running for that server.
func activeBot(r *http.Request) *marketbot.Instance {
	if sc := serverFromCtx(r); sc != nil {
		return sc.Bot
	}
	if sc := globalRegistry.Active(); sc != nil {
		return sc.Bot
	}
	return nil
}

// botConfiguredFor reports whether the request's server has the bot toggle on
// (even if the instance isn't currently running).
func botConfiguredFor(r *http.Request) bool {
	if sc := serverFromCtx(r); sc != nil {
		return sc.BotConfigured
	}
	if sc := globalRegistry.Active(); sc != nil {
		return sc.BotConfigured
	}
	return false
}
