package main

import (
	"context"
	"path/filepath"
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

// backupCfgPath is the legacy scheduled-backups.json path (overridable in
// tests). It is now read only by the one-time file→DB migration.
var backupCfgPath string

func scheduledBackupPath() string {
	if backupCfgPath != "" {
		return backupCfgPath
	}
	return filepath.Join(configDir(), "scheduled-backups.json")
}

// getScheduledBackupConfig loads the backup schedule for serverID from the DB.
// A missing row (or no store) yields the disabled default.
func getScheduledBackupConfig(serverID int) scheduledBackupConfig {
	if globalStore == nil {
		return scheduledBackupConfig{}
	}
	cfg, ok, err := loadBackupSchedule(globalStore, serverID)
	if err != nil {
		componentLog("scheduled_backup").Error().Err(err).Int("server", serverID).Msg("load schedule failed")
		return scheduledBackupConfig{}
	}
	if !ok {
		return scheduledBackupConfig{}
	}
	return cfg
}

func saveScheduledBackupConfig(serverID int, c scheduledBackupConfig) error {
	if globalStore == nil {
		return errStoreUnavailable
	}
	return saveBackupSchedule(globalStore, serverID, c)
}

// setBackupLastFired persists the watermark for serverID, preserving the rest
// of that server's schedule.
func setBackupLastFired(serverID int, ts int64) {
	cur := getScheduledBackupConfig(serverID)
	cur.LastFired = ts
	if err := saveScheduledBackupConfig(serverID, cur); err != nil {
		componentLog("scheduled_backup").Error().Err(err).Int("server", serverID).Msg("persist last_fired failed")
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
			backupSchedulerTickOnce(now())
		}
	}
}

// backupSchedulerTickOnce evaluates every registered server's backup schedule
// and fires the ones that are due, each acting on its own control/executor.
func backupSchedulerTickOnce(at time.Time) {
	for _, sc := range globalRegistry.All() {
		if sc == nil {
			continue
		}
		cfg := getScheduledBackupConfig(sc.StoreScope)
		if fireAt, fire := backupShouldFire(at, cfg, restartLocation(cfg.Timezone)); fire {
			fireScheduledBackup(sc, cfg, fireAt)
		}
	}
}

// fireScheduledBackup runs a backup for one server using that server's control
// plane / executor / config, then prunes per that server's keep_n.
func fireScheduledBackup(sc *ServerContext, cfg scheduledBackupConfig, at time.Time) {
	// Watermark first so a failing backup can't re-fire the same occurrence every tick.
	setBackupLastFired(sc.StoreScope, at.Unix())
	log := componentLog("scheduled_backup").Info().Str("server", sc.ID).Str("occurrence", at.Format(time.RFC3339))
	log.Msg("firing backup")
	if sc.Control == nil || sc.Executor == nil {
		componentLog("scheduled_backup").Warn().Str("server", sc.ID).Msg("control plane not connected; backup skipped")
		return
	}
	prov, ok := sc.Control.(dbBackupProvider)
	if !ok {
		componentLog("scheduled_backup").Warn().Str("server", sc.ID).Str("control_plane", sc.Control.Name()).Msg("control plane has no DB backup support; skipped")
		return
	}
	dir, err := dbBackupDirFor(sc.Cfg)
	if err != nil {
		componentLog("scheduled_backup").Error().Err(err).Str("server", sc.ID).Msg("backup dir unavailable")
		return
	}
	name := dbBackupFilename(time.Now())
	dest := filepath.Join(dir, name)
	if out, err := prov.BackupDatabase(sc.Executor, dbBackupConnFor(sc.Cfg), dest); err != nil {
		componentLog("scheduled_backup").Error().Err(err).Str("server", sc.ID).Str("output", out).Msg("backup failed")
		return
	}
	componentLog("scheduled_backup").Info().Str("server", sc.ID).Str("name", name).Msg("backup written")
	pruneOldBackups(sc.Cfg, cfg.KeepN)
}

// pruneOldBackups enforces the keep-N retention policy for one server's backup
// dir, deleting the oldest dumps beyond the limit.
func pruneOldBackups(cfg ServerConfig, keepN int) {
	if keepN <= 0 {
		return
	}
	files, err := listDBBackupsIn(cfg)
	if err != nil {
		componentLog("scheduled_backup").Error().Err(err).Msg("prune list failed")
		return
	}
	names := make([]string, len(files))
	for i := range files {
		names[i] = files[i].Name
	}
	for _, n := range backupsToPrune(names, keepN) {
		if err := deleteDBBackupIn(cfg, n); err != nil {
			componentLog("scheduled_backup").Error().Err(err).Str("name", n).Msg("prune failed")
		} else {
			componentLog("scheduled_backup").Info().Str("name", n).Msg("pruned old backup")
		}
	}
}

// now is the clock seam for tests; defaults to time.Now.
var now = time.Now
