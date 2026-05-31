package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

var globalSessionDB *sql.DB

type sessionStats struct {
	TotalPlaytimeSecs int64 `json:"total_playtime_secs"`
	SessionCount      int64 `json:"session_count"`
	AvgSessionSecs    int64 `json:"avg_session_secs"`
}

func resolveSessionDBPath() string {
	if p := os.Getenv("DUNE_ADMIN_SESSIONS_DB"); p != "" {
		return p
	}
	return filepath.Join(configDir(), "sessions.db")
}

func openSessionDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session db: %w", err)
	}
	if err := initSessionSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init session schema: %w", err)
	}
	return db, nil
}

func initSessionSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS play_sessions (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id    INTEGER NOT NULL,
			started_at    TEXT    NOT NULL,
			ended_at      TEXT,
			duration_secs INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_ps_account ON play_sessions(account_id);
		CREATE TABLE IF NOT EXISTS stat_snapshots (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id      INTEGER NOT NULL,
			snapped_at      TEXT    NOT NULL,
			char_xp         INTEGER,
			skill_points    INTEGER,
			intel_points    INTEGER,
			combat_xp       INTEGER,
			crafting_xp     INTEGER,
			gathering_xp    INTEGER,
			exploration_xp  INTEGER,
			sabotage_xp     INTEGER,
			solaris_balance INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_ss_account ON stat_snapshots(account_id, snapped_at);
	`); err != nil {
		return err
	}
	// Migration: add solaris_balance to pre-existing databases that lack it.
	// SQLite returns a "duplicate column name" error if it already exists (e.g.
	// on a fresh DB where CREATE TABLE above already includes the column).
	_, err := db.Exec(`ALTER TABLE stat_snapshots ADD COLUMN solaris_balance INTEGER`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("migrate stat_snapshots.solaris_balance: %w", err)
	}
	return nil
}

// closeOrphanedSessions marks sessions left open by a previous run. Duration
// is set to 0 since we don't know when the player actually logged off.
func closeOrphanedSessions(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE play_sessions
		SET ended_at = started_at, duration_secs = 0
		WHERE ended_at IS NULL
	`)
	return err
}

// recordSessions compares onlineIDs against currently open sessions in the
// SQLite store and opens/closes sessions as needed.
func recordSessions(ctx context.Context, onlineIDs []int64, db *sql.DB) error {
	openSessions, err := queryOpenSessions(ctx, db)
	if err != nil {
		return err
	}

	onlineSet := make(map[int64]bool, len(onlineIDs))
	for _, id := range onlineIDs {
		onlineSet[id] = true
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, id := range onlineIDs {
		if !openSessions[id] {
			if _, err := db.ExecContext(ctx,
				`INSERT INTO play_sessions(account_id, started_at) VALUES(?, ?)`,
				id, now,
			); err != nil {
				return fmt.Errorf("start session for account %d: %w", id, err)
			}
		}
	}

	for id := range openSessions {
		if !onlineSet[id] {
			if _, err := db.ExecContext(ctx, `
				UPDATE play_sessions
				SET ended_at = ?,
				    duration_secs = CAST((julianday(?) - julianday(started_at)) * 86400 AS INTEGER)
				WHERE account_id = ? AND ended_at IS NULL`,
				now, now, id,
			); err != nil {
				return fmt.Errorf("close session for account %d: %w", id, err)
			}
		}
	}

	return nil
}

func queryOpenSessions(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT account_id FROM play_sessions WHERE ended_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query open sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	open := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan open session: %w", err)
		}
		open[id] = true
	}
	return open, rows.Err()
}

func getSessionStats(ctx context.Context, db *sql.DB, accountID int64) (sessionStats, error) {
	row := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(duration_secs), 0),
			COUNT(*),
			COALESCE(AVG(duration_secs), 0.0)
		FROM play_sessions
		WHERE account_id = ? AND ended_at IS NOT NULL
	`, accountID)

	var stats sessionStats
	var avg float64
	if err := row.Scan(&stats.TotalPlaytimeSecs, &stats.SessionCount, &avg); err != nil {
		return sessionStats{}, fmt.Errorf("get session stats for account %d: %w", accountID, err)
	}
	stats.AvgSessionSecs = int64(avg)
	return stats, nil
}

// startSessionTracking opens the session DB and starts the poller. Returns a
// cancel func that stops the poller. Errors are logged, not fatal.
func startSessionTracking() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	if err := initSessionPoller(ctx); err != nil {
		log.Printf("session tracker: %v", err)
	}
	return cancel
}

// initSessionPoller opens (or creates) the session database and starts the
// background polling goroutine. Skips gracefully when globalDB is not yet
// connected so the server still starts in degraded mode.
func initSessionPoller(ctx context.Context) error {
	sdb, err := openSessionDB(resolveSessionDBPath())
	if err != nil {
		return fmt.Errorf("open session db: %w", err)
	}
	if err := closeOrphanedSessions(sdb); err != nil {
		log.Printf("session poller: close orphaned sessions: %v", err)
	}
	globalSessionDB = sdb

	if globalDB == nil {
		log.Printf("session poller: DB not connected, skipping poll loop")
		return nil
	}

	go startSessionPoller(ctx, globalDB, sdb, 5*time.Minute)
	return nil
}

type sessionRecord struct {
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	DurationSecs int64  `json:"duration_secs"`
}

func getSessionHistory(ctx context.Context, db *sql.DB, accountID int64, limit int) ([]sessionRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT started_at, ended_at, duration_secs
		FROM play_sessions
		WHERE account_id = ? AND ended_at IS NOT NULL
		ORDER BY started_at ASC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("query session history for account %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []sessionRecord
	for rows.Next() {
		var r sessionRecord
		if err := rows.Scan(&r.StartedAt, &r.EndedAt, &r.DurationSecs); err != nil {
			return nil, fmt.Errorf("scan session record: %w", err)
		}
		out = append(out, r)
	}
	if out == nil {
		out = []sessionRecord{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session history for account %d: %w", accountID, err)
	}
	return out, nil
}

type statSnapshot struct {
	AccountID      int64  `json:"account_id"`
	SnappedAt      string `json:"snapped_at"`
	CharXP         *int64 `json:"char_xp"`
	SkillPoints    *int   `json:"skill_points"`
	IntelPoints    *int   `json:"intel_points"`
	CombatXP       *int   `json:"combat_xp"`
	CraftingXP     *int   `json:"crafting_xp"`
	GatheringXP    *int   `json:"gathering_xp"`
	ExplorationXP  *int   `json:"exploration_xp"`
	SabotageXP     *int   `json:"sabotage_xp"`
	SolarisBalance *int64 `json:"solaris_balance"`
}

func writeStatSnapshot(ctx context.Context, sdb *sql.DB, snap statSnapshot) error {
	_, err := sdb.ExecContext(ctx, `
		INSERT INTO stat_snapshots(
			account_id, snapped_at,
			char_xp, skill_points, intel_points,
			combat_xp, crafting_xp, gathering_xp, exploration_xp, sabotage_xp,
			solaris_balance
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		snap.AccountID, snap.SnappedAt,
		snap.CharXP, snap.SkillPoints, snap.IntelPoints,
		snap.CombatXP, snap.CraftingXP, snap.GatheringXP, snap.ExplorationXP, snap.SabotageXP,
		snap.SolarisBalance,
	)
	if err != nil {
		return fmt.Errorf("write stat snapshot for account %d: %w", snap.AccountID, err)
	}
	return nil
}

func getStatSnapshotHistory(ctx context.Context, sdb *sql.DB, accountID int64, limit int) ([]statSnapshot, error) {
	rows, err := sdb.QueryContext(ctx, `
		SELECT snapped_at, char_xp, skill_points, intel_points,
		       combat_xp, crafting_xp, gathering_xp, exploration_xp, sabotage_xp,
		       solaris_balance
		FROM stat_snapshots
		WHERE account_id = ?
		ORDER BY snapped_at ASC
		LIMIT ?
	`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("query stat snapshot history for account %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []statSnapshot
	for rows.Next() {
		s := statSnapshot{AccountID: accountID}
		if err := rows.Scan(&s.SnappedAt, &s.CharXP, &s.SkillPoints, &s.IntelPoints,
			&s.CombatXP, &s.CraftingXP, &s.GatheringXP, &s.ExplorationXP, &s.SabotageXP,
			&s.SolarisBalance,
		); err != nil {
			return nil, fmt.Errorf("scan stat snapshot: %w", err)
		}
		out = append(out, s)
	}
	if out == nil {
		out = []statSnapshot{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stat snapshots for account %d: %w", accountID, err)
	}
	return out, nil
}

func pollOnce(ctx context.Context, pool *pgxpool.Pool, db *sql.DB) {
	onlineIDs, err := cmdFetchOnlineAccountIDs(ctx, pool)
	if err != nil {
		log.Printf("session poller: fetch online players: %v", err)
		return
	}
	if err := recordSessions(ctx, onlineIDs, db); err != nil {
		log.Printf("session poller: record sessions: %v", err)
	}
	snappedAt := time.Now().UTC().Format(time.RFC3339)
	for _, accountID := range onlineIDs {
		snap, err := cmdFetchPlayerSnapshot(ctx, pool, accountID, snappedAt)
		if err != nil {
			log.Printf("session poller: snapshot account %d: %v", accountID, err)
			continue
		}
		if err := writeStatSnapshot(ctx, db, snap); err != nil {
			log.Printf("session poller: write snapshot account %d: %v", accountID, err)
		}
	}
}

func startSessionPoller(ctx context.Context, pool *pgxpool.Pool, db *sql.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, pool, db)
		}
	}
}
