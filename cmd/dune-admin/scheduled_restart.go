package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ── Scheduled restarts (#145) ───────────────────────────────────────────────
// Weekday+time rules trigger a control-plane restart, preceded by the game's
// native shutdown-countdown broadcast (rmqServiceBroadcastShutdown). Config is a
// small JSON file in configDir; a goroutine ticks every restartSchedulerTick.
// "Skip next" is implemented by advancing LastFired to the next occurrence, so
// no separate flag is needed in the tick logic.

type restartRule struct {
	Days []int  `json:"days"` // 0=Sun .. 6=Sat
	Time string `json:"time"` // "HH:MM" 24h, in the configured timezone
}

type scheduledRestartConfig struct {
	Enabled     bool          `json:"enabled"`
	Timezone    string        `json:"timezone"` // IANA name; "" = host local
	Rules       []restartRule `json:"rules"`
	WarnMinutes int           `json:"warn_minutes"` // pre-restart countdown lead (default 10)
	LastFired   int64         `json:"last_fired"`   // unix seconds of the last fired/skipped restart
}

const (
	restartSchedulerTick = 30 * time.Second
	restartFireGrace     = 5 * time.Minute // don't fire a restart missed by more than this
	defaultWarnMinutes   = 10
)

// restartCfgPath is the legacy scheduled-restarts.json path (overridable in
// tests). It is now read only by the one-time file→DB migration.
var restartCfgPath string

func scheduledRestartPath() string {
	if restartCfgPath != "" {
		return restartCfgPath
	}
	return filepath.Join(configDir(), "scheduled-restarts.json")
}

// getScheduledRestartConfig loads the restart schedule for serverID from the DB.
// A missing row (or no store) yields the disabled default with the default warn
// lead so the UI / scheduler see a sensible value.
func getScheduledRestartConfig(serverID int) scheduledRestartConfig {
	def := scheduledRestartConfig{WarnMinutes: defaultWarnMinutes}
	if globalStore == nil {
		return def
	}
	cfg, ok, err := loadRestartSchedule(globalStore, serverID)
	if err != nil {
		componentLog("scheduled_restart").Error().Err(err).Int("server", serverID).Msg("load schedule failed")
		return def
	}
	if !ok {
		return def
	}
	if cfg.WarnMinutes <= 0 {
		cfg.WarnMinutes = defaultWarnMinutes
	}
	return cfg
}

func saveScheduledRestartConfig(serverID int, c scheduledRestartConfig) error {
	if c.WarnMinutes <= 0 {
		c.WarnMinutes = defaultWarnMinutes
	}
	if globalStore == nil {
		return errStoreUnavailable
	}
	return saveRestartSchedule(globalStore, serverID, c)
}

// setRestartLastFired persists the watermark for serverID, preserving the rest
// of that server's schedule.
func setRestartLastFired(serverID int, ts int64) {
	cur := getScheduledRestartConfig(serverID)
	cur.LastFired = ts
	if err := saveScheduledRestartConfig(serverID, cur); err != nil {
		componentLog("scheduled_restart").Error().Err(err).Int("server", serverID).Msg("persist last_fired failed")
	}
}

// ── pure scheduling logic (testable) ────────────────────────────────────────

func parseHHMM(s string) (h, m int, ok bool) {
	var hh, mm int
	if n, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &hh, &mm); err != nil || n != 2 {
		return 0, 0, false
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

func restartLocation(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// nextDayOccurrence returns the first datetime strictly after nowL on weekday d
// at h:m (searching forward up to 8 days), or ok=false.
func nextDayOccurrence(nowL time.Time, d, h, m int, loc *time.Location) (time.Time, bool) {
	for off := 0; off <= 7; off++ {
		cand := time.Date(nowL.Year(), nowL.Month(), nowL.Day()+off, h, m, 0, 0, loc)
		if int(cand.Weekday()) == d && cand.After(nowL) {
			return cand, true
		}
	}
	return time.Time{}, false
}

// prevDayOccurrence returns the most recent datetime at/before nowL on weekday d
// at h:m (searching back up to 8 days), or ok=false.
func prevDayOccurrence(nowL time.Time, d, h, m int, loc *time.Location) (time.Time, bool) {
	for off := 0; off <= 7; off++ {
		cand := time.Date(nowL.Year(), nowL.Month(), nowL.Day()-off, h, m, 0, 0, loc)
		if int(cand.Weekday()) == d && !cand.After(nowL) {
			return cand, true
		}
	}
	return time.Time{}, false
}

// ruleDays yields the valid (day, hour, minute) tuples of a rule.
func ruleDays(r restartRule) (h, m int, days []int, ok bool) {
	h, m, ok = parseHHMM(r.Time)
	if !ok {
		return 0, 0, nil, false
	}
	for _, d := range r.Days {
		if d >= 0 && d <= 6 {
			days = append(days, d)
		}
	}
	return h, m, days, true
}

// nextRestartAt returns the soonest restart strictly after now across all rules.
func nextRestartAt(now time.Time, rules []restartRule, loc *time.Location) (time.Time, bool) {
	nowL := now.In(loc)
	var best time.Time
	found := false
	for _, r := range rules {
		h, m, days, ok := ruleDays(r)
		if !ok {
			continue
		}
		for _, d := range days {
			if cand, ok := nextDayOccurrence(nowL, d, h, m, loc); ok && (!found || cand.Before(best)) {
				best, found = cand, true
			}
		}
	}
	return best, found
}

// prevRestartAt returns the most recent restart at/before now across all rules.
func prevRestartAt(now time.Time, rules []restartRule, loc *time.Location) (time.Time, bool) {
	nowL := now.In(loc)
	var best time.Time
	found := false
	for _, r := range rules {
		h, m, days, ok := ruleDays(r)
		if !ok {
			continue
		}
		for _, d := range days {
			if cand, ok := prevDayOccurrence(nowL, d, h, m, loc); ok && (!found || cand.After(best)) {
				best, found = cand, true
			}
		}
	}
	return best, found
}

type restartAction int

const (
	restartNone restartAction = iota
	restartWarn
	restartFire
)

// restartDecision is the pure tick decision: should we fire a restart now, warn
// of an upcoming one, or do nothing? warnedFor is the target we've already warned
// for (so we don't re-warn every tick). Returns the action + its target time.
func restartDecision(now time.Time, cfg scheduledRestartConfig, loc *time.Location, warnedFor time.Time) (restartAction, time.Time) {
	if !cfg.Enabled || len(cfg.Rules) == 0 {
		return restartNone, time.Time{}
	}
	if prevAt, ok := prevRestartAt(now, cfg.Rules, loc); ok &&
		prevAt.Unix() > cfg.LastFired && now.Sub(prevAt) <= restartFireGrace {
		return restartFire, prevAt
	}
	if nextAt, ok := nextRestartAt(now, cfg.Rules, loc); ok {
		lead := time.Duration(cfg.WarnMinutes) * time.Minute
		if nextAt.Sub(now) <= lead && nextAt.Unix() > cfg.LastFired && !nextAt.Equal(warnedFor) {
			return restartWarn, nextAt
		}
	}
	return restartNone, time.Time{}
}

// ── scheduler goroutine + side effects ──────────────────────────────────────

func runRestartScheduler(ctx context.Context) {
	t := time.NewTicker(restartSchedulerTick)
	defer t.Stop()
	// warnedFor tracks the last-warned target per server (keyed by StoreScope) so
	// we don't re-broadcast the same upcoming restart every tick.
	warnedFor := map[int]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			restartSchedulerTickOnce(ctx, now(), warnedFor)
		}
	}
}

// restartSchedulerTickOnce evaluates every registered server's restart schedule,
// firing or warning per server. warnedFor is updated in place.
func restartSchedulerTickOnce(ctx context.Context, at time.Time, warnedFor map[int]time.Time) {
	for _, sc := range globalRegistry.All() {
		if sc == nil {
			continue
		}
		cfg := getScheduledRestartConfig(sc.StoreScope)
		act, target := restartDecision(at, cfg, restartLocation(cfg.Timezone), warnedFor[sc.StoreScope])
		switch act {
		case restartFire:
			fireScheduledRestart(ctx, sc, target)
		case restartWarn:
			broadcastRestartWarning(cfg, target, at)
			warnedFor[sc.StoreScope] = target
		}
	}
}

func fireScheduledRestart(ctx context.Context, sc *ServerContext, at time.Time) {
	// Watermark first so a failed/looping restart can't re-fire the same occurrence.
	setRestartLastFired(sc.StoreScope, at.Unix())
	componentLog("scheduled_restart").Info().Str("server", sc.ID).Str("scheduled_for", at.Format(time.RFC3339)).Msg("firing restart")
	if sc.Control == nil || sc.Executor == nil {
		componentLog("scheduled_restart").Warn().Str("server", sc.ID).Msg("control plane not connected; restart skipped")
		return
	}
	if _, err := sc.Control.ExecCommand(ctx, sc.Executor, "restart"); err != nil {
		componentLog("scheduled_restart").Error().Err(err).Str("server", sc.ID).Msg("restart failed")
	}
}

func broadcastRestartWarning(_ scheduledRestartConfig, at, now time.Time) {
	mins := int(at.Sub(now).Round(time.Minute) / time.Minute)
	if mins < 1 {
		mins = 1
	}
	componentLog("scheduled_restart").Info().Int("warn_minutes", mins).Msg("broadcasting restart warning")
	// shutdownType, timestamp (when), frequency (re-announce sec), duration (countdown sec), cancel.
	if err := rmqServiceBroadcastShutdown("Restart", at.Unix(), 60, mins*60, false); err != nil {
		componentLog("scheduled_restart").Error().Err(err).Msg("warning broadcast failed")
	}
}
