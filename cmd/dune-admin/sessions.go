package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	// play_sessions / stat_snapshots are server-scoped via an integer FK →
	// servers(id) ON DELETE CASCADE.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS play_sessions (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id     INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			account_id    INTEGER NOT NULL,
			started_at    TEXT    NOT NULL,
			ended_at      TEXT,
			duration_secs INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_ps_account ON play_sessions(account_id);
		CREATE TABLE IF NOT EXISTS stat_snapshots (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
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
	return nil
}

// closeOrphanedSessions marks sessions left open by a previous run. Duration
// is set to 0 since we don't know when the player actually logged off.
func closeOrphanedSessions(db *sql.DB, serverID int) error {
	_, err := db.Exec(`
		UPDATE play_sessions
		SET ended_at = started_at, duration_secs = 0
		WHERE server_id = ? AND ended_at IS NULL
	`, serverID)
	return err
}

// recordSessions compares onlineIDs against currently open sessions in the
// SQLite store and opens/closes sessions as needed.
func recordSessions(ctx context.Context, onlineIDs []int64, db *sql.DB, serverID int) error {
	openSessions, err := queryOpenSessions(ctx, db, serverID)
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
				`INSERT INTO play_sessions(server_id, account_id, started_at) VALUES(?, ?, ?)`,
				serverID, id, now,
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
				WHERE server_id = ? AND account_id = ? AND ended_at IS NULL`,
				now, now, serverID, id,
			); err != nil {
				return fmt.Errorf("close session for account %d: %w", id, err)
			}
		}
	}

	return nil
}

func queryOpenSessions(ctx context.Context, db *sql.DB, serverID int) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT account_id FROM play_sessions WHERE server_id = ? AND ended_at IS NULL`, serverID)
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

func getSessionStats(ctx context.Context, db *sql.DB, serverID int, accountID int64) (sessionStats, error) {
	row := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(duration_secs), 0),
			COUNT(*),
			COALESCE(AVG(duration_secs), 0.0)
		FROM play_sessions
		WHERE server_id = ? AND account_id = ? AND ended_at IS NOT NULL
	`, serverID, accountID)

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
		componentLog("sessions").Warn().Err(err).Msg("session tracker init failed")
	}
	return cancel
}

// initSessionPoller initialises the session database and starts one background
// polling goroutine per registered server. When globalStore is available the
// unified handle is used directly; otherwise a dedicated file is opened (legacy
// / test mode). Skips gracefully when no servers have a DB connection.
func initSessionPoller(ctx context.Context) error {
	var sdb *sql.DB
	if globalStore != nil {
		sdb = globalStore
	} else {
		var err error
		sdb, err = openSessionDB(resolveSessionDBPath())
		if err != nil {
			return fmt.Errorf("open session db: %w", err)
		}
	}
	globalSessionDB = sdb

	servers := globalRegistry.All()
	started := 0
	for _, sc := range servers {
		if sc.DB == nil {
			serverLog("sessions", sc).Info().Msg("session poller: DB not connected, skipping")
			continue
		}
		if err := closeOrphanedSessions(sdb, sc.StoreScope); err != nil {
			serverLog("sessions", sc).Warn().Err(err).Msg("session poller: close orphaned sessions failed")
		}
		go startSessionPoller(ctx, sc.DB, sdb, sc.StoreScope, 5*time.Minute)
		started++
	}
	if started == 0 {
		componentLog("sessions").Info().Msg("session poller: no servers with DB connections, skipping poll loop")
	}
	return nil
}

type sessionRecord struct {
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	DurationSecs int64  `json:"duration_secs"`
}

func getSessionHistory(ctx context.Context, db *sql.DB, serverID int, accountID int64, limit int) ([]sessionRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT started_at, ended_at, duration_secs
		FROM play_sessions
		WHERE server_id = ? AND account_id = ? AND ended_at IS NOT NULL
		ORDER BY started_at ASC
		LIMIT ?
	`, serverID, accountID, limit)
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

func writeStatSnapshot(ctx context.Context, sdb *sql.DB, snap statSnapshot, serverID int) error {
	_, err := sdb.ExecContext(ctx, `
		INSERT INTO stat_snapshots(
			server_id, account_id, snapped_at,
			char_xp, skill_points, intel_points,
			combat_xp, crafting_xp, gathering_xp, exploration_xp, sabotage_xp,
			solaris_balance
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		serverID, snap.AccountID, snap.SnappedAt,
		snap.CharXP, snap.SkillPoints, snap.IntelPoints,
		snap.CombatXP, snap.CraftingXP, snap.GatheringXP, snap.ExplorationXP, snap.SabotageXP,
		snap.SolarisBalance,
	)
	if err != nil {
		return fmt.Errorf("write stat snapshot for account %d: %w", snap.AccountID, err)
	}
	return nil
}

func getStatSnapshotHistory(ctx context.Context, sdb *sql.DB, serverID int, accountID int64, limit int) ([]statSnapshot, error) {
	rows, err := sdb.QueryContext(ctx, `
		SELECT snapped_at, char_xp, skill_points, intel_points,
		       combat_xp, crafting_xp, gathering_xp, exploration_xp, sabotage_xp,
		       solaris_balance
		FROM stat_snapshots
		WHERE server_id = ? AND account_id = ?
		ORDER BY snapped_at ASC
		LIMIT ?
	`, serverID, accountID, limit)
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

// daySnap is one account's latest stat snapshot on a given UTC day, used by the
// faction-growth trend (#130 ext). Solaris/XP default to 0 when NULL.
type daySnap struct {
	AccountID int64
	Day       string
	Solaris   int64
	CharXP    int64
}

// getDailySnapshots returns the latest snapshot per (account, UTC day) within
// the last `days` days — one row per account per day, so a day's total isn't
// inflated by the 5-minute poll cadence.
func getDailySnapshots(ctx context.Context, db *sql.DB, serverID int, days int) ([]daySnap, error) {
	if days < 1 {
		days = 1
	}
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `
		SELECT account_id, day, solaris, xp FROM (
			SELECT account_id,
			       substr(snapped_at, 1, 10) AS day,
			       COALESCE(solaris_balance, 0) AS solaris,
			       COALESCE(char_xp, 0) AS xp,
			       ROW_NUMBER() OVER (PARTITION BY account_id, substr(snapped_at, 1, 10) ORDER BY snapped_at DESC) AS rn
			FROM stat_snapshots
			WHERE server_id = ? AND substr(snapped_at, 1, 10) >= ?
		) WHERE rn = 1
		ORDER BY day`, serverID, since)
	if err != nil {
		return nil, fmt.Errorf("query daily snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []daySnap
	for rows.Next() {
		var d daySnap
		if err := rows.Scan(&d.AccountID, &d.Day, &d.Solaris, &d.CharXP); err != nil {
			return nil, fmt.Errorf("scan daily snapshot: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func pollOnce(ctx context.Context, pool *pgxpool.Pool, db *sql.DB, serverID int) {
	onlineIDs, err := cmdFetchOnlineAccountIDs(ctx, pool)
	if err != nil {
		componentLog("sessions").Warn().Int("server_id", serverID).Err(err).Msg("session poller: fetch online players failed")
		return
	}
	if err := recordSessions(ctx, onlineIDs, db, serverID); err != nil {
		componentLog("sessions").Warn().Int("server_id", serverID).Err(err).Msg("session poller: record sessions failed")
	}
	snappedAt := time.Now().UTC().Format(time.RFC3339)
	for _, accountID := range onlineIDs {
		snap, err := cmdFetchPlayerSnapshot(ctx, pool, accountID, snappedAt)
		if err != nil {
			componentLog("sessions").Warn().Int("server_id", serverID).Int64("account_id", accountID).Err(err).Msg("session poller: snapshot failed")
			continue
		}
		if err := writeStatSnapshot(ctx, db, snap, serverID); err != nil {
			componentLog("sessions").Warn().Int("server_id", serverID).Int64("account_id", accountID).Err(err).Msg("session poller: write snapshot failed")
		}
	}
}

func startSessionPoller(ctx context.Context, pool *pgxpool.Pool, db *sql.DB, serverID int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, pool, db, serverID)
		}
	}
}

// activityPoint is one day's session count for the server-wide activity trend
// on the Players dashboard (#130). Day is "YYYY-MM-DD" in UTC.
type activityPoint struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// getServerPlaytimeSecs sums completed-session duration across all players.
func getServerPlaytimeSecs(ctx context.Context, db *sql.DB, serverID int) (int64, error) {
	var total int64
	row := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(duration_secs), 0) FROM play_sessions WHERE server_id = ? AND ended_at IS NOT NULL`,
		serverID)
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("server playtime: %w", err)
	}
	return total, nil
}

// getActivityTrendCounts returns a sparse day->session-count map for sessions
// started on or after sinceDay ("YYYY-MM-DD", UTC). started_at is RFC3339, so
// its first 10 chars are the UTC date — substr keeps the day-bucket comparable.
func getActivityTrendCounts(ctx context.Context, db *sql.DB, serverID int, sinceDay string) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT substr(started_at, 1, 10) AS day, COUNT(*)
		FROM play_sessions
		WHERE server_id = ? AND substr(started_at, 1, 10) >= ?
		GROUP BY day`, serverID, sinceDay)
	if err != nil {
		return nil, fmt.Errorf("query activity trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int64)
	for rows.Next() {
		var day string
		var n int64
		if err := rows.Scan(&day, &n); err != nil {
			return nil, fmt.Errorf("scan activity trend: %w", err)
		}
		counts[day] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activity trend: %w", err)
	}
	return counts, nil
}

// fillActivityTrend turns a sparse day->count map into a contiguous, ascending
// series of `days` points ending on today (UTC), zero-filling inactive days so
// the dashboard chart shows gaps as 0. today is a parameter (not time.Now) so
// the logic is deterministic and unit-testable.
func fillActivityTrend(days int, today time.Time, counts map[string]int64) []activityPoint {
	if days < 1 {
		days = 1
	}
	out := make([]activityPoint, 0, days)
	start := today.AddDate(0, 0, -(days - 1))
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, activityPoint{Day: day, Count: counts[day]})
	}
	return out
}

// sessionSummary returns the session-derived dashboard fields: total playtime
// and a zero-filled `days`-day activity trend. db may be nil (session tracking
// disabled) — then playtime is 0 and the trend is all zeros. Query failures are
// logged, not fatal: the dashboard degrades gracefully.
func sessionSummary(ctx context.Context, db *sql.DB, serverID int, days int) (int64, []activityPoint) {
	now := time.Now().UTC()
	if db == nil {
		return 0, fillActivityTrend(days, now, map[string]int64{})
	}
	var playtime int64
	if pt, err := getServerPlaytimeSecs(ctx, db, serverID); err != nil {
		componentLog("sessions").Warn().Int("server_id", serverID).Err(err).Msg("sessionSummary: playtime query failed")
	} else {
		playtime = pt
	}
	since := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	counts, err := getActivityTrendCounts(ctx, db, serverID, since)
	if err != nil {
		componentLog("sessions").Warn().Int("server_id", serverID).Err(err).Msg("sessionSummary: trend query failed")
		counts = map[string]int64{}
	}
	return playtime, fillActivityTrend(days, now, counts)
}
