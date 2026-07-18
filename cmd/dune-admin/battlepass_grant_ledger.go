package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// battlepass_grant_ledger.go implements the per-tier deferred-grant ledger for
// battlepass auto-grant (#197). It reuses the events backoff policy from the
// shared deferred-grant core: a row is pending until granted (terminal success)
// or exhausted (deferredGrantMaxAttempts failures → manual-grant only).
//
// This ledger is distinct from battlepass_claims: claims track catalog progress
// (baseline/earned/granted), while this ledger tracks automatic delivery
// attempts so a transient failure (player online, DB hiccup) is retried with
// backoff instead of being lost.

// Battlepass grant-ledger statuses (mirror the event claim lifecycle). The
// terminal "granted" status is written as a SQL literal in recordGrantLedgerSuccess.
const (
	battlepassGrantPending   = "pending"
	battlepassGrantExhausted = "exhausted"
)

const battlepassGrantLedgerSchema = `
CREATE TABLE IF NOT EXISTS battlepass_grant_ledger (
	server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	tier_key        TEXT    NOT NULL,
	account_id      INTEGER NOT NULL,
	status          TEXT    NOT NULL DEFAULT 'pending',
	attempts        INTEGER NOT NULL DEFAULT 0,
	last_error      TEXT    NOT NULL DEFAULT '',
	next_attempt_at TEXT    NOT NULL DEFAULT '',
	updated_at      TEXT    NOT NULL,
	PRIMARY KEY (server_id, tier_key, account_id)
);`

// battlepassGrantLedgerRow is one row from battlepass_grant_ledger.
type battlepassGrantLedgerRow struct {
	TierKey       string `json:"tier_key"`
	AccountID     int64  `json:"account_id"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error"`
	NextAttemptAt string `json:"next_attempt_at"`
	UpdatedAt     string `json:"updated_at"`
}

// recordPendingGrant inserts a pending grant-ledger row for (tierKey, accountID)
// if none exists. Existing rows are never modified — a tier already mid-retry,
// granted, or exhausted must not be reset by a re-evaluation pass.
func (s *battlepassStore) recordPendingGrant(tierKey string, accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO battlepass_grant_ledger
			(server_id, tier_key, account_id, status, attempts, last_error, next_attempt_at, updated_at)
		VALUES (?, ?, ?, ?, 0, '', '', ?)
		ON CONFLICT(server_id, tier_key, account_id) DO NOTHING`,
		s.serverID, tierKey, accountID, battlepassGrantPending, now)
	if err != nil {
		return fmt.Errorf("record pending battlepass grant %s/%d: %w", tierKey, accountID, err)
	}
	return nil
}

// recordGrantLedgerSuccess marks a grant-ledger row granted (terminal).
func (s *battlepassStore) recordGrantLedgerSuccess(tierKey string, accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO battlepass_grant_ledger
			(server_id, tier_key, account_id, status, attempts, last_error, next_attempt_at, updated_at)
		VALUES (?, ?, ?, 'granted', 1, '', '', ?)
		ON CONFLICT(server_id, tier_key, account_id) DO UPDATE SET
			status          = 'granted',
			last_error      = '',
			next_attempt_at = '',
			updated_at      = excluded.updated_at`,
		s.serverID, tierKey, accountID, now)
	if err != nil {
		return fmt.Errorf("record battlepass grant success %s/%d: %w", tierKey, accountID, err)
	}
	return nil
}

// recordGrantLedgerFailure records a failed grant attempt. The resulting status
// is computed from the post-increment attempt count: once deferredGrantMaxAttempts
// attempts have been used the row becomes exhausted (manual-only, next_attempt_at
// cleared); otherwise it stays pending with next_attempt_at = now + backoff.
func (s *battlepassStore) recordGrantLedgerFailure(tierKey string, accountID int64, errMsg string) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	nextStr := now.Add(deferredGrantRetryBackoff).Format(time.RFC3339)
	// On insert this is attempt 1; on conflict it's existing attempts+1.
	_, err := s.db.Exec(`
		INSERT INTO battlepass_grant_ledger
			(server_id, tier_key, account_id, status, attempts, last_error, next_attempt_at, updated_at)
		VALUES (
			?, ?, ?,
			CASE WHEN 1 >= ? THEN ? ELSE ? END,
			1, ?,
			CASE WHEN 1 >= ? THEN '' ELSE ? END,
			?)
		ON CONFLICT(server_id, tier_key, account_id) DO UPDATE SET
			status = CASE WHEN battlepass_grant_ledger.attempts + 1 >= ? THEN ? ELSE ? END,
			attempts = battlepass_grant_ledger.attempts + 1,
			last_error = excluded.last_error,
			next_attempt_at = CASE WHEN battlepass_grant_ledger.attempts + 1 >= ? THEN '' ELSE ? END,
			updated_at = excluded.updated_at`,
		// VALUES (insert / attempt 1)
		s.serverID, tierKey, accountID,
		deferredGrantMaxAttempts, battlepassGrantExhausted, battlepassGrantPending,
		errMsg,
		deferredGrantMaxAttempts, nextStr,
		nowStr,
		// ON CONFLICT (existing attempts + 1)
		deferredGrantMaxAttempts, battlepassGrantExhausted, battlepassGrantPending,
		deferredGrantMaxAttempts, nextStr)
	if err != nil {
		return fmt.Errorf("record battlepass grant failure %s/%d: %w", tierKey, accountID, err)
	}
	return nil
}

// recordGrantLedgerRetryLater defers a grant attempt without counting it
// toward deferredGrantMaxAttempts. Used specifically when delivery failed
// because the player is online (#259/#280): that is an expected, frequent
// condition rather than a real failure, and the standard 24h/3-attempt
// failure policy would exhaust the row — forcing a manual grant — long
// before a player who stays online for hours ever logs out. attempts and
// status are deliberately left untouched on conflict.
func (s *battlepassStore) recordGrantLedgerRetryLater(tierKey string, accountID int64, errMsg string, backoff time.Duration) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	nextStr := now.Add(backoff).Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO battlepass_grant_ledger
			(server_id, tier_key, account_id, status, attempts, last_error, next_attempt_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(server_id, tier_key, account_id) DO UPDATE SET
			last_error      = excluded.last_error,
			next_attempt_at = excluded.next_attempt_at,
			updated_at      = excluded.updated_at`,
		s.serverID, tierKey, accountID, battlepassGrantPending, errMsg, nextStr, nowStr)
	if err != nil {
		return fmt.Errorf("record battlepass grant retry-later %s/%d: %w", tierKey, accountID, err)
	}
	return nil
}

// healExhaustedOnlineGrantLedger resets grant-ledger rows that were exhausted
// by the PRE-#259/#280 policy, where "player is online" counted as a real
// failure and burned all attempts — leaving the players who reported the bug
// permanently stuck on manual grant even after the fix shipped. It matches
// exhausted rows whose last_error carries the online message
// (playerOnlineErrMarker) and returns them to a fresh pending state so the
// retry engine picks them up. Deliberately UNSCOPED across server_id (runs
// once at startup on the shared handle, healing every server) and naturally
// idempotent: post-fix, online failures are recorded via
// recordGrantLedgerRetryLater and can never exhaust a row with this message
// again, so a second pass matches nothing.
func (s *battlepassStore) healExhaustedOnlineGrantLedger() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
		UPDATE battlepass_grant_ledger
		SET status = ?, attempts = 0, next_attempt_at = '', updated_at = ?
		WHERE status = ? AND last_error LIKE '%' || ? || '%'`,
		battlepassGrantPending, now, battlepassGrantExhausted, playerOnlineErrMarker)
	if err != nil {
		return 0, fmt.Errorf("heal exhausted online battlepass grants: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("heal exhausted online battlepass grants: %w", err)
	}
	return n, nil
}

// deleteUnsettledGrantLedger removes pending and exhausted grant-ledger rows —
// the #293 cleanup path, so a claims reset also stops queued auto-deliveries.
// Granted rows are delivery history and are kept. accountID 0 = every account
// in this server scope.
func (s *battlepassStore) deleteUnsettledGrantLedger(accountID int64) (int64, error) {
	q := `DELETE FROM battlepass_grant_ledger
	      WHERE server_id = ? AND status IN (?, ?)`
	args := []any{s.serverID, battlepassGrantPending, battlepassGrantExhausted}
	if accountID != 0 {
		q += ` AND account_id = ?`
		args = append(args, accountID)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("delete unsettled battlepass grant ledger: %w", err)
	}
	return res.RowsAffected()
}

// grantLedgerStatus returns the ledger status for (tierKey, accountID), or ""
// when no row exists.
func (s *battlepassStore) grantLedgerStatus(tierKey string, accountID int64) (string, error) {
	var status string
	err := s.db.QueryRow(
		`SELECT status FROM battlepass_grant_ledger
		 WHERE server_id = ? AND tier_key = ? AND account_id = ?`,
		s.serverID, tierKey, accountID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("battlepass grant ledger status %s/%d: %w", tierKey, accountID, err)
	}
	return status, nil
}

// listRetryableGrantLedger returns pending grant-ledger rows whose backoff
// window has elapsed (next_attempt_at <= now, empty = due) and which still have
// attempts remaining. A fresh pending row (empty next_attempt_at) sorts first
// and is always due.
func (s *battlepassStore) listRetryableGrantLedger(now time.Time) ([]battlepassGrantLedgerRow, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT tier_key, account_id, status, attempts, last_error, next_attempt_at, updated_at
		FROM battlepass_grant_ledger
		WHERE server_id = ? AND status = ? AND attempts < ? AND next_attempt_at <= ?
		ORDER BY next_attempt_at ASC`,
		s.serverID, battlepassGrantPending, deferredGrantMaxAttempts, nowStr)
	if err != nil {
		return nil, fmt.Errorf("list retryable battlepass grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]battlepassGrantLedgerRow, 0)
	for rows.Next() {
		var r battlepassGrantLedgerRow
		if err := rows.Scan(&r.TierKey, &r.AccountID, &r.Status, &r.Attempts,
			&r.LastError, &r.NextAttemptAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan battlepass grant ledger row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
