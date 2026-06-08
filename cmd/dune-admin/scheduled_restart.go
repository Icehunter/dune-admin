package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

var (
	restartMu      sync.RWMutex
	restartCfg     = scheduledRestartConfig{WarnMinutes: defaultWarnMinutes}
	restartCfgPath string // overridable in tests
)

func scheduledRestartPath() string {
	if restartCfgPath != "" {
		return restartCfgPath
	}
	return filepath.Join(configDir(), "scheduled-restarts.json")
}

func loadScheduledRestartConfig() {
	data, err := os.ReadFile(scheduledRestartPath())
	if err != nil {
		return // no file yet → defaults (disabled)
	}
	var c scheduledRestartConfig
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("scheduled-restarts: config parse: %v", err)
		return
	}
	if c.WarnMinutes <= 0 {
		c.WarnMinutes = defaultWarnMinutes
	}
	restartMu.Lock()
	restartCfg = c
	restartMu.Unlock()
}

func getScheduledRestartConfig() scheduledRestartConfig {
	restartMu.RLock()
	defer restartMu.RUnlock()
	return restartCfg
}

// persistRestartConfigLocked writes the in-memory config to disk. Caller holds restartMu.
func persistRestartConfigLocked() error {
	data, err := json.MarshalIndent(restartCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(scheduledRestartPath(), data, 0600)
}

func saveScheduledRestartConfig(c scheduledRestartConfig) error {
	if c.WarnMinutes <= 0 {
		c.WarnMinutes = defaultWarnMinutes
	}
	restartMu.Lock()
	defer restartMu.Unlock()
	restartCfg = c
	return persistRestartConfigLocked()
}

func setRestartLastFired(ts int64) {
	restartMu.Lock()
	defer restartMu.Unlock()
	restartCfg.LastFired = ts
	if err := persistRestartConfigLocked(); err != nil {
		log.Printf("scheduled-restarts: persist last_fired: %v", err)
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
	var warnedFor time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			warnedFor = restartSchedulerTickOnce(ctx, time.Now(), warnedFor)
		}
	}
}

func restartSchedulerTickOnce(ctx context.Context, now, warnedFor time.Time) time.Time {
	cfg := getScheduledRestartConfig()
	act, target := restartDecision(now, cfg, restartLocation(cfg.Timezone), warnedFor)
	switch act {
	case restartFire:
		fireScheduledRestart(ctx, target)
	case restartWarn:
		broadcastRestartWarning(cfg, target, now)
		return target
	}
	return warnedFor
}

func fireScheduledRestart(ctx context.Context, at time.Time) {
	// Watermark first so a failed/looping restart can't re-fire the same occurrence.
	setRestartLastFired(at.Unix())
	log.Printf("scheduled-restarts: firing restart scheduled for %s", at.Format(time.RFC3339))
	if globalControl == nil || globalExecutor == nil {
		log.Printf("scheduled-restarts: control plane not connected; restart skipped")
		return
	}
	if _, err := globalControl.ExecCommand(ctx, globalExecutor, "restart"); err != nil {
		log.Printf("scheduled-restarts: restart failed: %v", err)
	}
}

func broadcastRestartWarning(_ scheduledRestartConfig, at, now time.Time) {
	mins := int(at.Sub(now).Round(time.Minute) / time.Minute)
	if mins < 1 {
		mins = 1
	}
	log.Printf("scheduled-restarts: broadcasting %d-min restart warning", mins)
	// shutdownType, timestamp (when), frequency (re-announce sec), duration (countdown sec), cancel.
	if err := rmqServiceBroadcastShutdown("Restart", at.Unix(), 60, mins*60, false); err != nil {
		log.Printf("scheduled-restarts: warning broadcast failed: %v", err)
	}
}
