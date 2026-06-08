package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ── Scheduled database backups (#150) ───────────────────────────────────────
// Weekday+time rules trigger a pg_dump (via the control plane's dbBackupProvider)
// followed by keep-N retention pruning. Mirrors the scheduled-restarts pattern
// (#145) but is self-contained — it shares only the generic, restart-agnostic
// date helpers (parseHHMM / nextDayOccurrence / prevDayOccurrence / restartLocation).
// Backups are non-disruptive (pg_dump is an MVCC snapshot), so there is no
// pre-warning — the tick just fires.

type backupRule struct {
	Days []int  `json:"days"` // 0=Sun .. 6=Sat
	Time string `json:"time"` // "HH:MM" 24h, in the configured timezone
}

type scheduledBackupConfig struct {
	Enabled   bool         `json:"enabled"`
	Timezone  string       `json:"timezone"` // IANA name; "" = host local
	Rules     []backupRule `json:"rules"`
	KeepN     int          `json:"keep_n"`     // retention; <=0 keeps all
	LastFired int64        `json:"last_fired"` // unix seconds of the last fired backup
}

const (
	backupSchedulerTick = 60 * time.Second
	backupFireGrace     = 10 * time.Minute // don't fire a backup missed by more than this
)

var (
	backupMu      sync.RWMutex
	backupCfg     scheduledBackupConfig
	backupCfgPath string // overridable in tests
)

func scheduledBackupPath() string {
	if backupCfgPath != "" {
		return backupCfgPath
	}
	return filepath.Join(configDir(), "scheduled-backups.json")
}

func loadScheduledBackupConfig() {
	data, err := os.ReadFile(scheduledBackupPath())
	if err != nil {
		return // no file yet → defaults (disabled)
	}
	var c scheduledBackupConfig
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("scheduled-backups: config parse: %v", err)
		return
	}
	backupMu.Lock()
	backupCfg = c
	backupMu.Unlock()
}

func getScheduledBackupConfig() scheduledBackupConfig {
	backupMu.RLock()
	defer backupMu.RUnlock()
	return backupCfg
}

// persistBackupConfigLocked writes the in-memory config to disk. Caller holds backupMu.
func persistBackupConfigLocked() error {
	data, err := json.MarshalIndent(backupCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0o750); err != nil {
		return err
	}
	return os.WriteFile(scheduledBackupPath(), data, 0o600)
}

func saveScheduledBackupConfig(c scheduledBackupConfig) error {
	backupMu.Lock()
	defer backupMu.Unlock()
	backupCfg = c
	return persistBackupConfigLocked()
}

func setBackupLastFired(ts int64) {
	backupMu.Lock()
	defer backupMu.Unlock()
	backupCfg.LastFired = ts
	if err := persistBackupConfigLocked(); err != nil {
		log.Printf("scheduled-backups: persist last_fired: %v", err)
	}
}

// ── pure scheduling logic (testable) ────────────────────────────────────────

// backupRuleDays yields the valid (hour, minute, days) of a rule. Shares
// parseHHMM with the restart scheduler.
func backupRuleDays(r backupRule) (h, m int, days []int, ok bool) {
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

// prevBackupAt returns the most recent scheduled backup at/before now across all rules.
func prevBackupAt(now time.Time, rules []backupRule, loc *time.Location) (time.Time, bool) {
	nowL := now.In(loc)
	var best time.Time
	found := false
	for _, r := range rules {
		h, m, days, ok := backupRuleDays(r)
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

// nextBackupAt returns the soonest scheduled backup strictly after now across all rules.
func nextBackupAt(now time.Time, rules []backupRule, loc *time.Location) (time.Time, bool) {
	nowL := now.In(loc)
	var best time.Time
	found := false
	for _, r := range rules {
		h, m, days, ok := backupRuleDays(r)
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

// backupShouldFire is the pure tick decision: fire a backup for the most recent
// occurrence if it's newer than LastFired and within the grace window.
func backupShouldFire(now time.Time, cfg scheduledBackupConfig, loc *time.Location) (time.Time, bool) {
	if !cfg.Enabled || len(cfg.Rules) == 0 {
		return time.Time{}, false
	}
	if prevAt, ok := prevBackupAt(now, cfg.Rules, loc); ok &&
		prevAt.Unix() > cfg.LastFired && now.Sub(prevAt) <= backupFireGrace {
		return prevAt, true
	}
	return time.Time{}, false
}

// ── scheduler goroutine + side effects ──────────────────────────────────────

func runBackupScheduler(ctx context.Context) {
	t := time.NewTicker(backupSchedulerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			backupSchedulerTickOnce(ctx, time.Now())
		}
	}
}

func backupSchedulerTickOnce(_ context.Context, now time.Time) {
	cfg := getScheduledBackupConfig()
	if at, fire := backupShouldFire(now, cfg, restartLocation(cfg.Timezone)); fire {
		fireScheduledBackup(at)
	}
}

func fireScheduledBackup(at time.Time) {
	// Watermark first so a failing backup can't re-fire the same occurrence every tick.
	setBackupLastFired(at.Unix())
	log.Printf("scheduled-backups: firing backup for occurrence %s", at.Format(time.RFC3339))
	if globalControl == nil || globalExecutor == nil {
		log.Printf("scheduled-backups: control plane not connected; backup skipped")
		return
	}
	prov, ok := globalControl.(dbBackupProvider)
	if !ok {
		log.Printf("scheduled-backups: control plane %q has no DB backup support; skipped", globalControl.Name())
		return
	}
	dir, err := dbBackupDir()
	if err != nil {
		log.Printf("scheduled-backups: %v", err)
		return
	}
	name := dbBackupFilename(time.Now())
	dest := filepath.Join(dir, name)
	if out, err := prov.BackupDatabase(globalExecutor, dbBackupConn(), dest); err != nil {
		log.Printf("scheduled-backups: backup failed: %v (%s)", err, out)
		return
	}
	log.Printf("scheduled-backups: wrote %s", name)
	pruneOldBackups()
}

// pruneOldBackups enforces the keep-N retention policy, deleting the oldest
// dumps beyond the limit.
func pruneOldBackups() {
	cfg := getScheduledBackupConfig()
	if cfg.KeepN <= 0 {
		return
	}
	files, err := listDBBackups()
	if err != nil {
		log.Printf("scheduled-backups: prune list: %v", err)
		return
	}
	names := make([]string, len(files))
	for i := range files {
		names[i] = files[i].Name
	}
	for _, n := range backupsToPrune(names, cfg.KeepN) {
		if err := deleteDBBackup(n); err != nil {
			log.Printf("scheduled-backups: prune %s: %v", n, err)
		} else {
			log.Printf("scheduled-backups: pruned old backup %s", n)
		}
	}
}
