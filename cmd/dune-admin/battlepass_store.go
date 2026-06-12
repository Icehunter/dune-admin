package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// battlepassSignal names the data source used to evaluate a battlepass tier.
type battlepassSignal string

const (
	battlepassSignalLevel       battlepassSignal = "level"
	battlepassSignalJourneyNode battlepassSignal = "journey_node"
	battlepassSignalPlayerTag   battlepassSignal = "player_tag"
)

// Battlepass claim statuses. A baseline claim marks progress that existed
// before the pass started tracking the player — it is never grantable.
// Earned claims are grantable; granted claims have had their intel applied.
const (
	battlepassClaimBaseline = "baseline"
	battlepassClaimEarned   = "earned"
	battlepassClaimGranted  = "granted"
)

// battlepassTier is one reward tier. TierKey is the stable identity used by
// claims so the catalog can be reseeded without orphaning claim history.
type battlepassTier struct {
	ID        int64            `json:"id"`
	TierKey   string           `json:"tier_key"`
	Category  string           `json:"category"`
	Label     string           `json:"label"`
	Signal    battlepassSignal `json:"signal"`
	SignalKey string           `json:"signal_key"`
	Threshold int64            `json:"threshold"`
	Intel     int64            `json:"intel"`
	// RewardItems is an optional JSON-encoded []rewardItem granted alongside
	// the intel (same shape as event rewards). Empty string = intel only.
	RewardItems string `json:"reward_items"`
	Enabled     bool   `json:"enabled"`
}

// battlepassClaim is one row from battlepass_claims.
type battlepassClaim struct {
	TierKey   string `json:"tier_key"`
	AccountID int64  `json:"account_id"`
	Status    string `json:"status"`
	Intel     int64  `json:"intel"`
	EarnedAt  string `json:"earned_at"`
	GrantedAt string `json:"granted_at"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
}

// battlepassTierCounts summarises claim states for one tier.
type battlepassTierCounts struct {
	Baseline int `json:"baseline"`
	Earned   int `json:"earned"`
	Granted  int `json:"granted"`
}

const battlepassStoreSchema = `
CREATE TABLE IF NOT EXISTS battlepass_tiers (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	tier_key   TEXT    NOT NULL UNIQUE,
	category   TEXT    NOT NULL,
	label      TEXT    NOT NULL,
	signal     TEXT    NOT NULL,
	signal_key TEXT    NOT NULL DEFAULT '',
	threshold  INTEGER NOT NULL DEFAULT 0,
	intel      INTEGER NOT NULL DEFAULT 0,
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT    NOT NULL,
	updated_at TEXT    NOT NULL
);
CREATE TABLE IF NOT EXISTS battlepass_claims (
	tier_key   TEXT    NOT NULL,
	account_id INTEGER NOT NULL,
	status     TEXT    NOT NULL,
	intel      INTEGER NOT NULL DEFAULT 0,
	earned_at  TEXT    NOT NULL DEFAULT '',
	granted_at TEXT    NOT NULL DEFAULT '',
	attempts   INTEGER NOT NULL DEFAULT 0,
	last_error TEXT    NOT NULL DEFAULT '',
	updated_at TEXT    NOT NULL,
	PRIMARY KEY (tier_key, account_id)
);
CREATE TABLE IF NOT EXISTS battlepass_accounts (
	account_id   INTEGER PRIMARY KEY,
	baselined_at TEXT NOT NULL
);`

type battlepassStore struct {
	db *sql.DB
}

func initBattlepassSchema(db *sql.DB) error {
	if _, err := db.Exec(battlepassStoreSchema); err != nil {
		return fmt.Errorf("init battlepass schema: %w", err)
	}
	// reward_items was added after first release; ignore the duplicate-column
	// error so the migration is idempotent across restarts.
	if _, err := db.Exec(`ALTER TABLE battlepass_tiers ADD COLUMN reward_items TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("migrate battlepass reward_items: %w", err)
	}
	return nil
}

// newBattlepassStore wraps an existing (already-migrated) shared handle.
func newBattlepassStore(db *sql.DB) *battlepassStore {
	return &battlepassStore{db: db}
}

func openBattlepassStore(path string) (*battlepassStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open battlepass store: %w", err)
	}
	// SQLite is not safe for concurrent writers; a single open connection also
	// ensures in-memory databases (:memory:) share one instance across all
	// callers rather than each connection seeing its own empty database.
	db.SetMaxOpenConns(1)
	if err := initBattlepassSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &battlepassStore{db: db}, nil
}

var errBattlepassDuplicateTierKey = errors.New("tier_key already exists")

// ── tiers ─────────────────────────────────────────────────────────────────────

const battlepassTierColumns = `id, tier_key, category, label, signal, signal_key, threshold, intel, reward_items, enabled`

func scanBattlepassTier(row interface{ Scan(...any) error }) (battlepassTier, error) {
	var t battlepassTier
	var enabledInt int
	err := row.Scan(&t.ID, &t.TierKey, &t.Category, &t.Label, &t.Signal,
		&t.SignalKey, &t.Threshold, &t.Intel, &t.RewardItems, &enabledInt)
	t.Enabled = enabledInt != 0
	return t, err
}

func (s *battlepassStore) listTiers() ([]battlepassTier, error) {
	rows, err := s.db.Query(`SELECT ` + battlepassTierColumns + ` FROM battlepass_tiers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list battlepass tiers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]battlepassTier, 0)
	for rows.Next() {
		t, err := scanBattlepassTier(rows)
		if err != nil {
			return nil, fmt.Errorf("scan battlepass tier: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *battlepassStore) getTier(id int64) (*battlepassTier, error) {
	row := s.db.QueryRow(`SELECT `+battlepassTierColumns+` FROM battlepass_tiers WHERE id = ?`, id)
	t, err := scanBattlepassTier(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get battlepass tier %d: %w", id, err)
	}
	return &t, nil
}

func (s *battlepassStore) updateTier(id int64, label string, intel int64, enabled bool, rewardItems, category string, signal battlepassSignal, signalKey string, threshold int64) (*battlepassTier, error) {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE battlepass_tiers SET label = ?, intel = ?, enabled = ?, reward_items = ?, category = ?, signal = ?, signal_key = ?, threshold = ?, updated_at = ? WHERE id = ?`,
		label, intel, enabledInt, rewardItems, category, string(signal), signalKey, threshold, now, id)
	if err != nil {
		return nil, fmt.Errorf("update battlepass tier %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errNotFound
	}
	return s.getTier(id)
}

// createTier inserts a new tier and returns the created row.
// Returns errBattlepassDuplicateTierKey when tier_key already exists.
func (s *battlepassStore) createTier(t battlepassTier) (*battlepassTier, error) {
	enabledInt := 0
	if t.Enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`
		INSERT INTO battlepass_tiers
			(tier_key, category, label, signal, signal_key, threshold, intel, reward_items, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TierKey, t.Category, t.Label, string(t.Signal), t.SignalKey,
		t.Threshold, t.Intel, t.RewardItems, enabledInt, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errBattlepassDuplicateTierKey
		}
		return nil, fmt.Errorf("create battlepass tier %q: %w", t.TierKey, err)
	}
	id, _ := res.LastInsertId()
	return s.getTier(id)
}

// battlepassIDPlaceholders builds the (?, ?, ...) fragment and args for an
// IN clause over tier IDs.
func battlepassIDPlaceholders(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ", "), args
}

// setTiersEnabled flips the enabled flag for all given tier IDs.
func (s *battlepassStore) setTiersEnabled(ids []int64, enabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	marks, args := battlepassIDPlaceholders(ids)
	now := time.Now().UTC().Format(time.RFC3339)
	args = append([]any{enabledInt, now}, args...)
	if _, err := s.db.Exec(
		`UPDATE battlepass_tiers SET enabled = ?, updated_at = ? WHERE id IN (`+marks+`)`,
		args...); err != nil {
		return fmt.Errorf("bulk set battlepass tiers enabled: %w", err)
	}
	return nil
}

// deleteTiers removes the given tiers from the catalog. Claims are left in
// place (keyed by tier_key) in case the tier is later restored by a reseed.
func (s *battlepassStore) deleteTiers(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	marks, args := battlepassIDPlaceholders(ids)
	if _, err := s.db.Exec(`DELETE FROM battlepass_tiers WHERE id IN (`+marks+`)`, args...); err != nil {
		return fmt.Errorf("bulk delete battlepass tiers: %w", err)
	}
	return nil
}

func (s *battlepassStore) insertTiers(tiers []battlepassTier) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range tiers {
		enabledInt := 0
		if t.Enabled {
			enabledInt = 1
		}
		if _, err := s.db.Exec(`
			INSERT INTO battlepass_tiers
				(tier_key, category, label, signal, signal_key, threshold, intel, reward_items, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.TierKey, t.Category, t.Label, string(t.Signal), t.SignalKey,
			t.Threshold, t.Intel, t.RewardItems, enabledInt, now, now); err != nil {
			return fmt.Errorf("insert battlepass tier %q: %w", t.TierKey, err)
		}
	}
	return nil
}

// seedTiersIfEmpty inserts the catalog only when no tiers exist yet.
// Returns the number of tiers inserted (0 when already seeded).
func (s *battlepassStore) seedTiersIfEmpty(tiers []battlepassTier) (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM battlepass_tiers`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count battlepass tiers: %w", err)
	}
	if count > 0 {
		return 0, nil
	}
	if err := s.insertTiers(tiers); err != nil {
		return 0, err
	}
	return len(tiers), nil
}

// reseedTiers replaces the tier catalog. Claims are keyed by tier_key and
// are intentionally preserved.
func (s *battlepassStore) reseedTiers(tiers []battlepassTier) error {
	if _, err := s.db.Exec(`DELETE FROM battlepass_tiers`); err != nil {
		return fmt.Errorf("clear battlepass tiers: %w", err)
	}
	return s.insertTiers(tiers)
}

// ── claims ────────────────────────────────────────────────────────────────────

// recordClaim inserts a claim if none exists for (tierKey, accountID).
// Existing claims are never modified — re-evaluation must not downgrade
// earned or granted rows.
func (s *battlepassStore) recordClaim(tierKey string, accountID, intel int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO battlepass_claims
			(tier_key, account_id, status, intel, earned_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(tier_key, account_id) DO NOTHING`,
		tierKey, accountID, status, intel, now, now)
	if err != nil {
		return fmt.Errorf("record battlepass claim %s/%d: %w", tierKey, accountID, err)
	}
	return nil
}

// claimedKeys returns tier_key → status for one account.
func (s *battlepassStore) claimedKeys(accountID int64) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT tier_key, status FROM battlepass_claims WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, fmt.Errorf("claimed keys for %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var key, status string
		if err := rows.Scan(&key, &status); err != nil {
			return nil, fmt.Errorf("scan claimed key: %w", err)
		}
		out[key] = status
	}
	return out, rows.Err()
}

func scanBattlepassClaims(rows *sql.Rows) ([]battlepassClaim, error) {
	out := make([]battlepassClaim, 0)
	for rows.Next() {
		var c battlepassClaim
		if err := rows.Scan(&c.TierKey, &c.AccountID, &c.Status, &c.Intel,
			&c.EarnedAt, &c.GrantedAt, &c.Attempts, &c.LastError); err != nil {
			return nil, fmt.Errorf("scan battlepass claim: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const battlepassClaimColumns = `tier_key, account_id, status, intel, earned_at, granted_at, attempts, last_error`

func (s *battlepassStore) listClaims(accountID int64) ([]battlepassClaim, error) {
	rows, err := s.db.Query(
		`SELECT `+battlepassClaimColumns+` FROM battlepass_claims WHERE account_id = ? ORDER BY tier_key`,
		accountID)
	if err != nil {
		return nil, fmt.Errorf("list battlepass claims for %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanBattlepassClaims(rows)
}

// earnedClaims returns the grantable (status=earned) claims for one account.
func (s *battlepassStore) earnedClaims(accountID int64) ([]battlepassClaim, error) {
	rows, err := s.db.Query(
		`SELECT `+battlepassClaimColumns+` FROM battlepass_claims
		 WHERE account_id = ? AND status = ? ORDER BY tier_key`,
		accountID, battlepassClaimEarned)
	if err != nil {
		return nil, fmt.Errorf("earned battlepass claims for %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanBattlepassClaims(rows)
}

// battlepassEarnedTierRow is one pending claim joined with its tier's display
// metadata, used by the pending endpoint to return tier-level rows.
type battlepassEarnedTierRow struct {
	TierKey     string `json:"tier_key"`
	AccountID   int64  `json:"account_id"`
	Intel       int64  `json:"intel"`
	TierLabel   string `json:"tier_label"`
	RewardItems string `json:"reward_items"`
}

// earnedClaimsWithTiers returns all earned claims joined with their tier's
// label and reward_items. Claims with no matching tier fall back to tier_key
// as the label and empty reward_items.
func (s *battlepassStore) earnedClaimsWithTiers() ([]battlepassEarnedTierRow, error) {
	rows, err := s.db.Query(`
		SELECT c.tier_key, c.account_id, c.intel,
		       COALESCE(NULLIF(t.label, ''), c.tier_key),
		       COALESCE(t.reward_items, '')
		FROM battlepass_claims c
		LEFT JOIN battlepass_tiers t ON t.tier_key = c.tier_key
		WHERE c.status = ?
		ORDER BY t.label, c.account_id`, battlepassClaimEarned)
	if err != nil {
		return nil, fmt.Errorf("earned claims with tiers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]battlepassEarnedTierRow, 0)
	for rows.Next() {
		var r battlepassEarnedTierRow
		if err := rows.Scan(&r.TierKey, &r.AccountID, &r.Intel, &r.TierLabel, &r.RewardItems); err != nil {
			return nil, fmt.Errorf("scan earned tier row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// earnedClaim returns the single earned claim for an account+tier pair.
// Returns errBattlepassNothingEarned if no earned claim exists.
func (s *battlepassStore) earnedClaim(accountID int64, tierKey string) (battlepassClaim, error) {
	var c battlepassClaim
	err := s.db.QueryRow(
		`SELECT `+battlepassClaimColumns+` FROM battlepass_claims
		 WHERE account_id = ? AND tier_key = ? AND status = ?`,
		accountID, tierKey, battlepassClaimEarned).
		Scan(&c.TierKey, &c.AccountID, &c.Status, &c.Intel,
			&c.EarnedAt, &c.GrantedAt, &c.Attempts, &c.LastError)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return battlepassClaim{}, errBattlepassNothingEarned
		}
		return battlepassClaim{}, fmt.Errorf("earned claim %d/%s: %w", accountID, tierKey, err)
	}
	return c, nil
}

// markGrantedForTier flips a single earned claim to granted.
func (s *battlepassStore) markGrantedForTier(accountID int64, tierKey string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE battlepass_claims
		SET status = ?, granted_at = ?, last_error = '', updated_at = ?
		WHERE account_id = ? AND tier_key = ? AND status = ?`,
		battlepassClaimGranted, now, now, accountID, tierKey, battlepassClaimEarned)
	if err != nil {
		return fmt.Errorf("mark battlepass granted for %d/%s: %w", accountID, tierKey, err)
	}
	return nil
}

// recordGrantFailureForTier notes a failed grant on a single claim; it
// remains earned so the grant can be retried.
func (s *battlepassStore) recordGrantFailureForTier(accountID int64, tierKey, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE battlepass_claims
		SET attempts = attempts + 1, last_error = ?, updated_at = ?
		WHERE account_id = ? AND tier_key = ? AND status = ?`,
		errMsg, now, accountID, tierKey, battlepassClaimEarned)
	if err != nil {
		return fmt.Errorf("record battlepass grant failure for %d/%s: %w", accountID, tierKey, err)
	}
	return nil
}

// earnedTotals returns account_id → pending (earned, ungranted) intel.
func (s *battlepassStore) earnedTotals() (map[int64]int64, error) {
	rows, err := s.db.Query(`
		SELECT account_id, SUM(intel) FROM battlepass_claims
		WHERE status = ? GROUP BY account_id`, battlepassClaimEarned)
	if err != nil {
		return nil, fmt.Errorf("battlepass earned totals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]int64)
	for rows.Next() {
		var account, total int64
		if err := rows.Scan(&account, &total); err != nil {
			return nil, fmt.Errorf("scan earned total: %w", err)
		}
		out[account] = total
	}
	return out, rows.Err()
}

// markGrantedForAccount flips every earned claim for the account to granted.
func (s *battlepassStore) markGrantedForAccount(accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE battlepass_claims
		SET status = ?, granted_at = ?, last_error = '', updated_at = ?
		WHERE account_id = ? AND status = ?`,
		battlepassClaimGranted, now, now, accountID, battlepassClaimEarned)
	if err != nil {
		return fmt.Errorf("mark battlepass granted for %d: %w", accountID, err)
	}
	return nil
}

// recordGrantFailure notes a failed grant attempt on the account's earned
// claims; they remain earned so the grant can be retried.
func (s *battlepassStore) recordGrantFailure(accountID int64, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE battlepass_claims
		SET attempts = attempts + 1, last_error = ?, updated_at = ?
		WHERE account_id = ? AND status = ?`,
		errMsg, now, accountID, battlepassClaimEarned)
	if err != nil {
		return fmt.Errorf("record battlepass grant failure for %d: %w", accountID, err)
	}
	return nil
}

// isBaselined reports whether the account's pre-existing progress has been
// recorded. Until then, satisfied tiers are claimed as baseline (no reward).
func (s *battlepassStore) isBaselined(accountID int64) (bool, error) {
	var one int
	err := s.db.QueryRow(
		`SELECT 1 FROM battlepass_accounts WHERE account_id = ?`, accountID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("battlepass baselined check for %d: %w", accountID, err)
	}
	return true, nil
}

func (s *battlepassStore) markBaselined(accountID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO battlepass_accounts (account_id, baselined_at) VALUES (?, ?)
		ON CONFLICT(account_id) DO NOTHING`, accountID, now)
	if err != nil {
		return fmt.Errorf("mark battlepass baselined %d: %w", accountID, err)
	}
	return nil
}

// countsByTier returns tier_key → claim-state counts for the catalog view.
func (s *battlepassStore) countsByTier() (map[string]battlepassTierCounts, error) {
	rows, err := s.db.Query(`
		SELECT tier_key, status, COUNT(*) FROM battlepass_claims
		GROUP BY tier_key, status`)
	if err != nil {
		return nil, fmt.Errorf("battlepass counts by tier: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]battlepassTierCounts)
	for rows.Next() {
		var key, status string
		var n int
		if err := rows.Scan(&key, &status, &n); err != nil {
			return nil, fmt.Errorf("scan tier counts: %w", err)
		}
		c := out[key]
		switch status {
		case battlepassClaimBaseline:
			c.Baseline = n
		case battlepassClaimEarned:
			c.Earned = n
		case battlepassClaimGranted:
			c.Granted = n
		}
		out[key] = c
	}
	return out, rows.Err()
}
