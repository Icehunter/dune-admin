package main

import (
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
