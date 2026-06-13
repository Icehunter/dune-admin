package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// eventType is the kind of live event. Only "zone_race" and "milestone" exist in v1.
type eventType string

const (
	eventTypeZoneRace  eventType = "zone_race"
	eventTypeMilestone eventType = "milestone"
)

// eventDefinition is one row from event_definitions.
type eventDefinition struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Type              eventType `json:"type"`
	Enabled           bool      `json:"enabled"`
	Version           int       `json:"version"`
	Config            string    `json:"config"`
	Reward            string    `json:"reward"`
	AnnounceChannelID string    `json:"announce_channel_id"`
	AnnounceTemplate  string    `json:"announce_template"`
	PollSeconds       int       `json:"poll_seconds"`
	JitterSeconds     int       `json:"jitter_seconds"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
}

// eventClaimRecord is one row from event_award_claims.
type eventClaimRecord struct {
	EventID       int64  `json:"event_id"`
	Version       int    `json:"version"`
	AccountID     int64  `json:"account_id"`
	Status        string `json:"status"`
	ClaimedAt     string `json:"claimed_at"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error"`
	NextAttemptAt string `json:"next_attempt_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Claim grant lifecycle statuses.
//   - granted:   terminal success.
//   - pending:   grant failed but is still retryable; next_attempt_at holds the
//     earliest time the retry loop may try again (now+24h after each failure).
//   - exhausted: all eventGrantMaxAttempts have been used; only a manual Grant
//     action can deliver the reward.
//
// The UI also accepts the legacy "failed" status for claims written before this
// migration.
const (
	eventClaimStatusGranted   = "granted"
	eventClaimStatusPending   = "pending"
	eventClaimStatusExhausted = "exhausted"

	// eventGrantMaxAttempts is the number of grant attempts allowed before a
	// claim becomes exhausted (manual-only). Sourced from the shared
	// deferred-grant core so events and battlepass use one backoff policy.
	eventGrantMaxAttempts = deferredGrantMaxAttempts

	// eventGrantRetryBackoff is the delay added before the next automatic retry.
	eventGrantRetryBackoff = deferredGrantRetryBackoff
)

const eventsStoreSchema = `
CREATE TABLE IF NOT EXISTS event_definitions (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	name                TEXT    NOT NULL,
	type                TEXT    NOT NULL,
	enabled             INTEGER NOT NULL DEFAULT 0,
	version             INTEGER NOT NULL DEFAULT 1,
	config_json         TEXT    NOT NULL DEFAULT '{}',
	reward_json         TEXT    NOT NULL DEFAULT '',
	announce_channel_id TEXT    NOT NULL DEFAULT '',
	announce_template   TEXT    NOT NULL DEFAULT '',
	poll_seconds        INTEGER NOT NULL DEFAULT 7,
	jitter_seconds      INTEGER NOT NULL DEFAULT 3,
	created_at          TEXT    NOT NULL,
	updated_at          TEXT    NOT NULL
);
CREATE TABLE IF NOT EXISTS event_award_claims (
	event_id    INTEGER NOT NULL,
	version     INTEGER NOT NULL,
	account_id  INTEGER NOT NULL,
	status          TEXT    NOT NULL,
	claimed_at      TEXT    NOT NULL DEFAULT '',
	attempts        INTEGER NOT NULL DEFAULT 1,
	last_error      TEXT    NOT NULL DEFAULT '',
	next_attempt_at TEXT    NOT NULL DEFAULT '',
	updated_at      TEXT    NOT NULL,
	PRIMARY KEY (event_id, version, account_id)
);`

var errNotFound = fmt.Errorf("not found")

func initEventsSchema(db *sql.DB) error {
	if _, err := db.Exec(eventsStoreSchema); err != nil {
		return fmt.Errorf("init events schema: %w", err)
	}
	// Add schedule columns to existing databases that predate this migration.
	for _, stmt := range []string{
		"ALTER TABLE event_definitions ADD COLUMN poll_seconds   INTEGER NOT NULL DEFAULT 7",
		"ALTER TABLE event_definitions ADD COLUMN jitter_seconds INTEGER NOT NULL DEFAULT 3",
		"ALTER TABLE event_award_claims ADD COLUMN next_attempt_at TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate events tables: %w", err)
		}
	}
	return nil
}

type eventStore struct {
	db *sql.DB
}

func newEventStore(db *sql.DB) *eventStore {
	return &eventStore{db: db}
}

func openEventStore(path string) (*eventStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open event store: %w", err)
	}
	if err := initEventsSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &eventStore{db: db}, nil
}

func (s *eventStore) list() ([]eventDefinition, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, enabled, version, config_json, reward_json,
		       announce_channel_id, announce_template, poll_seconds, jitter_seconds,
		       created_at, updated_at
		FROM event_definitions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]eventDefinition, 0)
	for rows.Next() {
		var d eventDefinition
		var enabledInt int
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &enabledInt, &d.Version,
			&d.Config, &d.Reward, &d.AnnounceChannelID, &d.AnnounceTemplate,
			&d.PollSeconds, &d.JitterSeconds, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		d.Enabled = enabledInt != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *eventStore) get(id int64) (*eventDefinition, error) {
	var d eventDefinition
	var enabledInt int
	err := s.db.QueryRow(`
		SELECT id, name, type, enabled, version, config_json, reward_json,
		       announce_channel_id, announce_template, poll_seconds, jitter_seconds,
		       created_at, updated_at
		FROM event_definitions WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Type, &enabledInt, &d.Version,
			&d.Config, &d.Reward, &d.AnnounceChannelID, &d.AnnounceTemplate,
			&d.PollSeconds, &d.JitterSeconds, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get event %d: %w", id, err)
	}
	d.Enabled = enabledInt != 0
	return &d, nil
}

func (s *eventStore) create(d eventDefinition) (*eventDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if d.Config == "" {
		d.Config = "{}"
	}
	if d.PollSeconds <= 0 {
		d.PollSeconds = 7
	}
	if d.JitterSeconds <= 0 {
		d.JitterSeconds = 3
	}
	res, err := s.db.Exec(`
		INSERT INTO event_definitions
			(name, type, enabled, version, config_json, reward_json,
			 announce_channel_id, announce_template, poll_seconds, jitter_seconds,
			 created_at, updated_at)
		VALUES (?, ?, 0, 1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Name, string(d.Type), d.Config, d.Reward,
		d.AnnounceChannelID, d.AnnounceTemplate, d.PollSeconds, d.JitterSeconds, now, now)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.get(id)
}

func (s *eventStore) update(d eventDefinition) (*eventDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if d.PollSeconds <= 0 {
		d.PollSeconds = 7
	}
	if d.JitterSeconds <= 0 {
		d.JitterSeconds = 3
	}
	res, err := s.db.Exec(`
		UPDATE event_definitions
		SET name = ?, type = ?, config_json = ?, reward_json = ?,
		    announce_channel_id = ?, announce_template = ?,
		    poll_seconds = ?, jitter_seconds = ?, updated_at = ?
		WHERE id = ?`,
		d.Name, string(d.Type), d.Config, d.Reward,
		d.AnnounceChannelID, d.AnnounceTemplate, d.PollSeconds, d.JitterSeconds, now, d.ID)
	if err != nil {
		return nil, fmt.Errorf("update event %d: %w", d.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errNotFound
	}
	return s.get(d.ID)
}

func (s *eventStore) setEnabled(id int64, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE event_definitions SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabledInt, now, id)
	if err != nil {
		return fmt.Errorf("set event enabled %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

func (s *eventStore) delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM event_definitions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete event %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

func (s *eventStore) listClaims(eventID int64) ([]eventClaimRecord, error) {
	rows, err := s.db.Query(`
		SELECT event_id, version, account_id, status, claimed_at, attempts,
		       last_error, next_attempt_at, updated_at
		FROM event_award_claims WHERE event_id = ? ORDER BY updated_at DESC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event claims %d: %w", eventID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]eventClaimRecord, 0)
	for rows.Next() {
		var c eventClaimRecord
		if err := rows.Scan(&c.EventID, &c.Version, &c.AccountID, &c.Status,
			&c.ClaimedAt, &c.Attempts, &c.LastError, &c.NextAttemptAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan claim: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// getClaimStatus returns the current status and last error for a claim, plus
// whether it exists. Used by the live-tick path to decide whether to skip
// (granted/exhausted) or resume the un-granted remainder of a reward.
func (s *eventStore) getClaimStatus(eventID int64, version int, accountID int64) (status string, lastError string, exists bool, err error) {
	err = s.db.QueryRow(
		`SELECT status, last_error FROM event_award_claims WHERE event_id = ? AND version = ? AND account_id = ? LIMIT 1`,
		eventID, version, accountID).Scan(&status, &lastError)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("get claim status: %w", err)
	}
	return status, lastError, true, nil
}

// claimExists reports whether the reward for (event,version,account) has reached
// a state that blocks further automatic delivery. Only "granted" (terminal
// success) and "exhausted" (manual-only) qualify — a "pending" claim is still
// eligible for retry, so it must NOT count as existing.
func (s *eventStore) claimExists(eventID int64, version int, accountID int64) (bool, error) {
	var one int
	err := s.db.QueryRow(
		`SELECT 1 FROM event_award_claims
		 WHERE event_id = ? AND version = ? AND account_id = ?
		   AND status IN (?, ?) LIMIT 1`,
		eventID, version, accountID, eventClaimStatusGranted, eventClaimStatusExhausted).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim exists: %w", err)
	}
	return true, nil
}

// listRetryableClaims returns pending claims whose backoff window has elapsed
// (next_attempt_at <= now) and which still have attempts remaining.
func (s *eventStore) listRetryableClaims(now time.Time) ([]eventClaimRecord, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT event_id, version, account_id, status, claimed_at, attempts,
		       last_error, next_attempt_at, updated_at
		FROM event_award_claims
		WHERE status = ? AND attempts < ? AND next_attempt_at <= ?
		ORDER BY next_attempt_at ASC`,
		eventClaimStatusPending, eventGrantMaxAttempts, nowStr)
	if err != nil {
		return nil, fmt.Errorf("list retryable claims: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]eventClaimRecord, 0)
	for rows.Next() {
		var c eventClaimRecord
		if err := rows.Scan(&c.EventID, &c.Version, &c.AccountID, &c.Status,
			&c.ClaimedAt, &c.Attempts, &c.LastError, &c.NextAttemptAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan retryable claim: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *eventStore) recordGranted(eventID int64, version int, accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO event_award_claims
			(event_id, version, account_id, status, claimed_at, attempts, last_error, next_attempt_at, updated_at)
		VALUES (?, ?, ?, 'granted', ?, 1, '', '', ?)
		ON CONFLICT(event_id, version, account_id) DO UPDATE SET
			status          = 'granted',
			claimed_at      = excluded.claimed_at,
			attempts        = event_award_claims.attempts + 1,
			last_error      = '',
			next_attempt_at = '',
			updated_at      = excluded.updated_at`,
		eventID, version, accountID, now, now)
	if err != nil {
		return fmt.Errorf("record granted %d/%d/%d: %w", eventID, version, accountID, err)
	}
	return nil
}

// recordFailed records a failed grant attempt. The resulting status is computed
// from the post-increment attempt count: once eventGrantMaxAttempts attempts have
// been used the claim becomes "exhausted" (manual-only, next_attempt_at cleared);
// otherwise it becomes "pending" with next_attempt_at = now + backoff so the
// retry loop will try again after the window elapses.
func (s *eventStore) recordFailed(eventID int64, version int, accountID int64, errMsg string) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	nextStr := now.Add(eventGrantRetryBackoff).Format(time.RFC3339)
	// On insert this is attempt 1; on conflict it's the existing attempts+1.
	// The CASE expressions decide status and next_attempt_at from that new count.
	_, err := s.db.Exec(`
		INSERT INTO event_award_claims
			(event_id, version, account_id, status, claimed_at, attempts, last_error, next_attempt_at, updated_at)
		VALUES (
			?, ?, ?,
			CASE WHEN 1 >= ? THEN ? ELSE ? END,
			'', 1, ?,
			CASE WHEN 1 >= ? THEN '' ELSE ? END,
			?)
		ON CONFLICT(event_id, version, account_id) DO UPDATE SET
			status = CASE WHEN event_award_claims.attempts + 1 >= ? THEN ? ELSE ? END,
			attempts = event_award_claims.attempts + 1,
			last_error = excluded.last_error,
			next_attempt_at = CASE WHEN event_award_claims.attempts + 1 >= ? THEN '' ELSE ? END,
			updated_at = excluded.updated_at`,
		// VALUES (insert / attempt 1)
		eventID, version, accountID,
		eventGrantMaxAttempts, eventClaimStatusExhausted, eventClaimStatusPending,
		errMsg,
		eventGrantMaxAttempts, nextStr,
		nowStr,
		// ON CONFLICT (existing attempts + 1)
		eventGrantMaxAttempts, eventClaimStatusExhausted, eventClaimStatusPending,
		eventGrantMaxAttempts, nextStr)
	if err != nil {
		return fmt.Errorf("record failed %d/%d/%d: %w", eventID, version, accountID, err)
	}
	return nil
}

func (s *eventStore) clearClaims(eventID int64) error {
	_, err := s.db.Exec(`DELETE FROM event_award_claims WHERE event_id = ?`, eventID)
	if err != nil {
		return fmt.Errorf("clear claims %d: %w", eventID, err)
	}
	return nil
}
