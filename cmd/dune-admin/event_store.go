package main

import (
	"database/sql"
	"errors"
	"fmt"
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
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
}

// eventClaimRecord is one row from event_award_claims.
type eventClaimRecord struct {
	EventID   int64  `json:"event_id"`
	Version   int    `json:"version"`
	AccountID int64  `json:"account_id"`
	Status    string `json:"status"`
	ClaimedAt string `json:"claimed_at"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	UpdatedAt string `json:"updated_at"`
}

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
	created_at          TEXT    NOT NULL,
	updated_at          TEXT    NOT NULL
);
CREATE TABLE IF NOT EXISTS event_award_claims (
	event_id    INTEGER NOT NULL,
	version     INTEGER NOT NULL,
	account_id  INTEGER NOT NULL,
	status      TEXT    NOT NULL,
	claimed_at  TEXT    NOT NULL DEFAULT '',
	attempts    INTEGER NOT NULL DEFAULT 1,
	last_error  TEXT    NOT NULL DEFAULT '',
	updated_at  TEXT    NOT NULL,
	PRIMARY KEY (event_id, version, account_id)
);`

var errNotFound = fmt.Errorf("not found")

func initEventsSchema(db *sql.DB) error {
	if _, err := db.Exec(eventsStoreSchema); err != nil {
		return fmt.Errorf("init events schema: %w", err)
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
		       announce_channel_id, announce_template, created_at, updated_at
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
			&d.CreatedAt, &d.UpdatedAt); err != nil {
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
		       announce_channel_id, announce_template, created_at, updated_at
		FROM event_definitions WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Type, &enabledInt, &d.Version,
			&d.Config, &d.Reward, &d.AnnounceChannelID, &d.AnnounceTemplate,
			&d.CreatedAt, &d.UpdatedAt)
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
	res, err := s.db.Exec(`
		INSERT INTO event_definitions
			(name, type, enabled, version, config_json, reward_json,
			 announce_channel_id, announce_template, created_at, updated_at)
		VALUES (?, ?, 0, 1, ?, ?, ?, ?, ?, ?)`,
		d.Name, string(d.Type), d.Config, d.Reward,
		d.AnnounceChannelID, d.AnnounceTemplate, now, now)
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.get(id)
}

func (s *eventStore) update(d eventDefinition) (*eventDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
		UPDATE event_definitions
		SET name = ?, type = ?, config_json = ?, reward_json = ?,
		    announce_channel_id = ?, announce_template = ?, updated_at = ?
		WHERE id = ?`,
		d.Name, string(d.Type), d.Config, d.Reward,
		d.AnnounceChannelID, d.AnnounceTemplate, now, d.ID)
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
		SELECT event_id, version, account_id, status, claimed_at, attempts, last_error, updated_at
		FROM event_award_claims WHERE event_id = ? ORDER BY updated_at DESC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event claims %d: %w", eventID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]eventClaimRecord, 0)
	for rows.Next() {
		var c eventClaimRecord
		if err := rows.Scan(&c.EventID, &c.Version, &c.AccountID, &c.Status,
			&c.ClaimedAt, &c.Attempts, &c.LastError, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan claim: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *eventStore) claimExists(eventID int64, version int, accountID int64) (bool, error) {
	var one int
	err := s.db.QueryRow(
		`SELECT 1 FROM event_award_claims WHERE event_id = ? AND version = ? AND account_id = ? LIMIT 1`,
		eventID, version, accountID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim exists: %w", err)
	}
	return true, nil
}

func (s *eventStore) recordGranted(eventID int64, version int, accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO event_award_claims
			(event_id, version, account_id, status, claimed_at, attempts, last_error, updated_at)
		VALUES (?, ?, ?, 'granted', ?, 1, '', ?)
		ON CONFLICT(event_id, version, account_id) DO UPDATE SET
			status     = 'granted',
			claimed_at = excluded.claimed_at,
			attempts   = event_award_claims.attempts + 1,
			last_error = '',
			updated_at = excluded.updated_at`,
		eventID, version, accountID, now, now)
	if err != nil {
		return fmt.Errorf("record granted %d/%d/%d: %w", eventID, version, accountID, err)
	}
	return nil
}

func (s *eventStore) recordFailed(eventID int64, version int, accountID int64, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO event_award_claims
			(event_id, version, account_id, status, claimed_at, attempts, last_error, updated_at)
		VALUES (?, ?, ?, 'failed', '', 1, ?, ?)
		ON CONFLICT(event_id, version, account_id) DO UPDATE SET
			status     = 'failed',
			attempts   = event_award_claims.attempts + 1,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		eventID, version, accountID, errMsg, now)
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
