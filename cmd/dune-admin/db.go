package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── journey-node fetch cache ────────────────────────────────────────────────
// Journey fetches return ~300-800 rows per character through an SSH tunnel,
// which makes them visibly slow. A short TTL cache keeps the common
// "open modal → close → reopen" loop snappy.
//
// The key is "<server scope>:journey:<accountID>" — scoped by SERVER first, then
// the query — so two game servers (separate DBs that can share account ids)
// never serve each other's journey data. Mutations call invalidateAllJourneyCache
// to drop stale entries (cheap for this single-user admin tool).

const journeyCacheTTL = 30 * time.Second

type journeyCacheEntry struct {
	nodes  []journeyNode
	cached time.Time
}

var (
	journeyCacheMu sync.RWMutex
	journeyCache   = map[string]journeyCacheEntry{}
)

// journeyCacheKey scopes the cache by server then account: "<scope>:journey:<id>".
func journeyCacheKey(scope string, accountID int64) string {
	return cacheKey(scope, "journey", strconv.FormatInt(accountID, 10))
}

// invalidateAllJourneyCache drops every journey entry across all servers. Called
// after any journey/progression mutation; over-invalidating is cheap here (the
// cache only accelerates the open→close→reopen loop on a single-user tool).
func invalidateAllJourneyCache() {
	journeyCacheMu.Lock()
	journeyCache = map[string]journeyCacheEntry{}
	journeyCacheMu.Unlock()
}

// ── data fetch commands ───────────────────────────────────────────────────────

func cmdFetchPlayers(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgPlayers{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT a.id,
		       COALESCE(a.owner_account_id, 0),
		       COALESCE(ps.character_name, convert_from(e.encrypted_funcom_id, 'UTF8'), ''),
		       COALESCE(ps.player_controller_id, 0),
		       COALESCE(ac."user", ''),
		       a.class,
		       COALESCE(a.map, ''),
		       COALESCE(af.faction_id, 0),
		       COALESCE(ps.online_status::text, 'Offline')
		FROM dune.actors a`+playerStateCanonicalJoin+`
		LEFT JOIN dune.encrypted_accounts e ON e.id = a.owner_account_id
		LEFT JOIN dune.accounts ac ON ac.id = a.owner_account_id`+factionByAccountJoin+`
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1
		ORDER BY a.id`, gmIdentityAccountID)
	if err != nil {
		return msgPlayers{err: err}
	}
	defer rows.Close()

	var players []playerInfo
	for rows.Next() {
		var p playerInfo
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.ControllerID, &p.FLSID, &p.Class, &p.Map, &p.FactionID, &p.OnlineStatus); err != nil {
			continue
		}
		p.Class = shortClass(p.Class)
		players = append(players, p)
	}
	if rows.Err() != nil {
		return msgPlayers{err: rows.Err()}
	}
	return msgPlayers{rows: players}
}

// enrichPlayersWithDiscord fills DiscordUserID/DiscordAvatar on each player from
// the SQLite discord_user_links rows scoped to serverID. The players query is
// per-server (each pool is one game server), so the caller passes the store
// scope of the pool being queried. Best-effort: a store error leaves the Discord
// fields empty rather than failing the whole player list.
func enrichPlayersWithDiscord(players []playerInfo, serverID int) []playerInfo {
	if len(players) == 0 || globalDiscordGuildsStore == nil {
		return players
	}
	links, err := globalDiscordGuildsStore.userLinksForServer(serverID)
	if err != nil {
		componentLog("discord").Warn().Int("server_id", serverID).Err(err).Msg("player-list discord enrichment failed")
		return players
	}
	for i := range players {
		if info, ok := links[players[i].AccountID]; ok {
			players[i].DiscordUserID = info.discordUserID
			players[i].DiscordAvatar = info.avatar
		}
	}
	return players
}

// labeledCount is one (label, count) row of a server-wide distribution on the
// Players dashboard (#130) — e.g. players per map.
type labeledCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// factionStat is one faction's player count + economy totals on the Players
// dashboard (#130). "Unaligned" buckets characters with no dune.player_faction
// row (i.e. players who never picked a faction).
type factionStat struct {
	Faction  string  `json:"faction"`
	Players  int64   `json:"players"`
	Solaris  int64   `json:"solaris"`
	Scrip    int64   `json:"scrip"`
	AvgLevel float64 `json:"avg_level"`
}

// serverStats is the Postgres-derived half of the Players dashboard summary
// (#130): population counts, a per-map distribution, and economy totals.
type serverStats struct {
	TotalPlayers  int64          `json:"total_players"`
	OnlinePlayers int64          `json:"online_players"`
	ByMap         []labeledCount `json:"by_map"`
	ByFaction     []factionStat  `json:"by_faction"`
	TotalSolaris  int64          `json:"total_solaris"`
	TotalScrip    int64          `json:"total_scrip"`
}

// serverSummary is the full Players-dashboard payload: serverStats plus the
// session-derived playtime + activity trend (from sessions.db).
type serverSummary struct {
	TotalPlayers      int64           `json:"total_players"`
	OnlinePlayers     int64           `json:"online_players"`
	ByMap             []labeledCount  `json:"by_map"`
	ByFaction         []factionStat   `json:"by_faction"`
	TotalSolaris      int64           `json:"total_solaris"`
	TotalScrip        int64           `json:"total_scrip"`
	AvgCharLevel      float64         `json:"avg_char_level"`
	TotalPlaytimeSecs int64           `json:"total_playtime_secs"`
	ActivityTrend     []activityPoint `json:"activity_trend"`
	TrendDays         int             `json:"trend_days"`
}

// playerStateCanonicalJoinOn builds the canonical-row lateral join for the
// given actors alias, exposing the single chosen dune.player_state row under
// psAlias (#290). The game's own post-1.5 schema migration dropped the unique
// constraint on encrypted_player_state.account_id (see seedGMIdentity's doc
// comment for the full story), so an account can end up with more than one
// player_state row; a plain "LEFT JOIN dune.player_state ps ON ps.account_id
// = <a>.owner_account_id" then fans a single actor row out into one output
// row per duplicate — the "duplicate players" symptom — and can hand callers
// a stale player_pawn_id from an orphaned row. This LATERAL always picks
// exactly one row under THE canonical definition used everywhere:
// most-recently-active (last_login_time DESC NULLS LAST, id DESC), i.e. the
// row the game itself last touched. resolvePlayerCharacterID uses the same
// definition so the players list and every character-keyed feature agree on
// which duplicate is "the" character. Aliases are compile-time literals from
// the query constants below, never input.
func playerStateCanonicalJoinOn(actorAlias, psAlias string) string {
	return fmt.Sprintf(`
		LEFT JOIN LATERAL (
			SELECT * FROM dune.player_state ps2
			WHERE ps2.account_id = %s.owner_account_id
			ORDER BY ps2.last_login_time DESC NULLS LAST, ps2.id DESC
			LIMIT 1
		) %s ON true`, actorAlias, psAlias)
}

// playerStateCanonicalJoin is the common "a"/"ps" form of
// playerStateCanonicalJoinOn — the outer query must alias the character
// actor as "a"; this exposes the usual "ps" columns.
var playerStateCanonicalJoin = playerStateCanonicalJoinOn("a", "ps")

const (
	// factionByAccountJoin resolves a player's faction by ACCOUNT. Faction is
	// stored on the PlayerController actor, NOT the PlayerCharacter, so joining
	// dune.player_faction directly onto the character actor (pf.actor_id = a.id)
	// misses and mis-buckets aligned players as "Unaligned". Both actors share
	// owner_account_id, so we resolve through a per-account derived table. The
	// outer query must alias the character actor as "a"; this exposes af.faction_id.
	factionByAccountJoin = `
		LEFT JOIN (
			SELECT DISTINCT fa.owner_account_id AS account_id, pf.faction_id
			FROM dune.player_faction pf
			JOIN dune.actors fa ON fa.id = pf.actor_id
		) af ON af.account_id = a.owner_account_id`

	serverByMapSQL = `
		SELECT COALESCE(NULLIF(a.map, ''), 'Unknown') AS label, COUNT(*) AS count
		FROM dune.actors a
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1
		GROUP BY label
		ORDER BY count DESC, label`

	// Economy totals across all balances. Solaris is identified by the game's
	// own dune.get_solaris_id(); everything else is treated as scrip.
	serverEconomySQL = `
		SELECT
			COALESCE(SUM(CASE WHEN currency_id =  dune.get_solaris_id() THEN balance ELSE 0 END), 0) AS solaris,
			COALESCE(SUM(CASE WHEN currency_id <> dune.get_solaris_id() THEN balance ELSE 0 END), 0) AS scrip
		FROM dune.player_virtual_currency_balances`

	// Players + economy grouped by faction. LEFT JOINs so characters with no
	// dune.player_faction row fall into "Unaligned"; COUNT(DISTINCT a.id) stays
	// Cumulative character XP per player (DuneCharacter FLevelComponent), fed
	// through xpToLevel to compute the server's average character level.
	serverCharXPSQL = `
		SELECT COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0) AS xp
		FROM dune.actors a
		JOIN dune.actor_fgl_entities afe ON afe.actor_id = a.id AND afe.slot_name = 'DuneCharacter'
		JOIN dune.fgl_entities fe ON fe.entity_id = afe.entity_id
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`

	// Per-player (faction, char XP) for the per-faction average level (#130 ext).
	// Same DuneCharacter XP source as serverCharXPSQL, joined to faction (LEFT
	// JOINs → "Unaligned" bucket). Averaged in Go via xpToLevel.
	serverFactionXPSQL = `
		SELECT
			COALESCE(f.name, 'Unaligned') AS faction,
			COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0) AS xp
		FROM dune.actors a
		JOIN dune.actor_fgl_entities afe ON afe.actor_id = a.id AND afe.slot_name = 'DuneCharacter'
		JOIN dune.fgl_entities fe ON fe.entity_id = afe.entity_id` + factionByAccountJoin + `
		LEFT JOIN dune.factions f ON f.id = af.faction_id
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`
)

// Vars (not consts) because they embed playerStateCanonicalJoin, which is
// built by playerStateCanonicalJoinOn at init.
var (
	// Population counts. The seeded GM identity ($1) is excluded — it is not a
	// real player. online_status compares against the enum literal (see
	// cmdFetchOnlineAccountIDs), summed via CASE to match existing query style.
	// Uses playerStateCanonicalJoin (#290) so a duplicate player_state row
	// can't inflate the total/online counts the same way it fanned out
	// cmdFetchPlayers's list.
	serverCountsSQL = `
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN ps.online_status = 'Online' THEN 1 ELSE 0 END), 0) AS online
		FROM dune.actors a` + playerStateCanonicalJoin + `
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`

	// Players + economy grouped by faction. LEFT JOINs so characters with no
	// dune.player_faction row fall into "Unaligned"; COUNT(DISTINCT a.id) stays
	// correct despite the currency-row fan-out from the balances join, and the
	// canonical player_state join (#290) keeps duplicate state rows from
	// double-counting currency balances. Verified read-only against the test
	// VM before shipping.
	serverByFactionSQL = `
		SELECT
			COALESCE(f.name, 'Unaligned') AS faction,
			COUNT(DISTINCT a.id) AS players,
			COALESCE(SUM(CASE WHEN vcb.currency_id =  dune.get_solaris_id() THEN vcb.balance ELSE 0 END), 0) AS solaris,
			COALESCE(SUM(CASE WHEN vcb.currency_id <> dune.get_solaris_id() THEN vcb.balance ELSE 0 END), 0) AS scrip
		FROM dune.actors a` + factionByAccountJoin + `
		LEFT JOIN dune.factions f ON f.id = af.faction_id` + playerStateCanonicalJoin + `
		LEFT JOIN dune.player_virtual_currency_balances vcb ON vcb.player_controller_id = ps.player_controller_id
		WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1
		GROUP BY faction
		ORDER BY players DESC, faction`
)

// cmdFetchServerStats computes the Postgres-derived dashboard aggregates (#130):
// player counts, the per-map distribution, and economy totals.
func cmdFetchServerStats(ctx context.Context, pool *pgxpool.Pool) (serverStats, error) {
	var s serverStats
	if err := pool.QueryRow(ctx, serverCountsSQL, gmIdentityAccountID).Scan(&s.TotalPlayers, &s.OnlinePlayers); err != nil {
		return serverStats{}, fmt.Errorf("server counts: %w", err)
	}

	byMap, err := scanLabeledCounts(ctx, pool, serverByMapSQL, gmIdentityAccountID)
	if err != nil {
		return serverStats{}, fmt.Errorf("server by-map: %w", err)
	}
	s.ByMap = byMap

	if err := pool.QueryRow(ctx, serverEconomySQL).Scan(&s.TotalSolaris, &s.TotalScrip); err != nil {
		return serverStats{}, fmt.Errorf("server economy: %w", err)
	}

	byFaction, err := scanFactionStats(ctx, pool, serverByFactionSQL, gmIdentityAccountID)
	if err != nil {
		return serverStats{}, fmt.Errorf("server by-faction: %w", err)
	}
	levels, err := factionAvgLevels(ctx, pool)
	if err != nil {
		return serverStats{}, fmt.Errorf("server faction levels: %w", err)
	}
	for i := range byFaction {
		byFaction[i].AvgLevel = levels[byFaction[i].Faction]
	}
	s.ByFaction = byFaction
	return s, nil
}

// factionXP pairs a character's faction with its cumulative character XP, for
// the per-faction average level (#130 ext).
type factionXP struct {
	Faction string
	XP      int64
}

// avgLevelsByFaction averages per-character levels (via xpToLevel) within each
// faction. Pure + testable; empty input → empty map.
func avgLevelsByFaction(pairs []factionXP) map[string]float64 {
	sum := map[string]int{}
	cnt := map[string]int{}
	for _, p := range pairs {
		sum[p.Faction] += xpToLevel(p.XP)
		cnt[p.Faction]++
	}
	out := make(map[string]float64, len(cnt))
	for fac, n := range cnt {
		out[fac] = float64(sum[fac]) / float64(n)
	}
	return out
}

// factionAvgLevels queries per-player (faction, char XP) and returns the mean
// character level per faction.
func factionAvgLevels(ctx context.Context, pool *pgxpool.Pool) (map[string]float64, error) {
	rows, err := pool.Query(ctx, serverFactionXPSQL, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("faction xp: %w", err)
	}
	defer rows.Close()

	var pairs []factionXP
	for rows.Next() {
		var p factionXP
		if err := rows.Scan(&p.Faction, &p.XP); err != nil {
			return nil, fmt.Errorf("scan faction xp: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return avgLevelsByFaction(pairs), nil
}

// serverAccountFactionSQL maps each player account to its current faction name
// for the faction-growth trend (#130 ext). "Unaligned" when no faction row.
const serverAccountFactionSQL = `
	SELECT a.owner_account_id, COALESCE(f.name, 'Unaligned')
	FROM dune.actors a` + factionByAccountJoin + `
	LEFT JOIN dune.factions f ON f.id = af.faction_id
	WHERE a.class ILIKE '%PlayerCharacter%' AND a.owner_account_id <> $1`

// cmdFetchAccountFactions returns account_id -> current faction name.
func cmdFetchAccountFactions(ctx context.Context, pool *pgxpool.Pool) (map[int64]string, error) {
	rows, err := pool.Query(ctx, serverAccountFactionSQL, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("account factions: %w", err)
	}
	defer rows.Close()

	out := map[int64]string{}
	for rows.Next() {
		var acct int64
		var fac string
		if err := rows.Scan(&acct, &fac); err != nil {
			return nil, fmt.Errorf("scan account faction: %w", err)
		}
		out[acct] = fac
	}
	return out, rows.Err()
}

// ── Guilds (#117 Phase A — read-only) ────────────────────────────────────────
//
// Schema (all dune.): guilds(guild_id, guild_name, guild_description,
// guild_faction → factions.id); guild_members(player_id → actors.id, guild_id,
// role_id); guild_invites(invite_id, guild_id, player_id → actors.id,
// sender_player_id → actors.id, invite_sent_timespan). role_id is an in-game
// rank enum not modelled in the DB, so it is surfaced numerically. Names resolve
// actors.id → actors.owner_account_id → player_state.character_name.

var errGuildNotFound = errors.New("guild not found")

type guildSummary struct {
	GuildID     int64  `json:"guild_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	FactionID   int16  `json:"faction_id"`
	FactionName string `json:"faction_name"`
	MemberCount int64  `json:"member_count"`
}

type guildMember struct {
	PlayerID      int64  `json:"player_id"`
	RoleID        int16  `json:"role_id"`
	CharacterName string `json:"character_name"`
}

type guildInvite struct {
	InviteID      int64  `json:"invite_id"`
	PlayerID      int64  `json:"player_id"`
	CharacterName string `json:"character_name"`
	SenderID      int64  `json:"sender_player_id"`
	SenderName    string `json:"sender_name"`
}

type guildDetail struct {
	guildSummary
	Members []guildMember `json:"members"`
	Invites []guildInvite `json:"invites"`
}

// guildMemberDisplayName returns the character name, or a stable "Actor <id>"
// fallback when the name can't be resolved (the actor row exists — FK-guaranteed
// — but has no player_state, e.g. a never-fully-initialised or system actor).
func guildMemberDisplayName(charName string, actorID int64) string {
	if strings.TrimSpace(charName) != "" {
		return charName
	}
	return fmt.Sprintf("Actor %d", actorID)
}

// guildSummarySelect is the shared SELECT for list + detail; callers append the
// ORDER BY (list) or WHERE g.guild_id = $1 (detail).
const guildSummarySelect = `
	SELECT g.guild_id,
	       COALESCE(g.guild_name, ''),
	       COALESCE(g.guild_description, ''),
	       g.guild_faction,
	       COALESCE(f.name, ''),
	       (SELECT count(*) FROM dune.guild_members m WHERE m.guild_id = g.guild_id)
	FROM dune.guilds g
	LEFT JOIN dune.factions f ON f.id = g.guild_faction`

func cmdFetchGuilds(ctx context.Context, pool *pgxpool.Pool) ([]guildSummary, error) {
	rows, err := pool.Query(ctx, guildSummarySelect+`
		ORDER BY g.guild_name NULLS LAST, g.guild_id`)
	if err != nil {
		return nil, fmt.Errorf("list guilds: %w", err)
	}
	defer rows.Close()
	out := make([]guildSummary, 0, 16)
	for rows.Next() {
		var g guildSummary
		if err := rows.Scan(&g.GuildID, &g.Name, &g.Description, &g.FactionID, &g.FactionName, &g.MemberCount); err != nil {
			return nil, fmt.Errorf("scan guild: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Guild rosters resolve character names through the canonical player_state
// join (#290) — a bare account_id join duplicates member/invite rows when an
// account has duplicate state rows. The invites query joins twice (invitee
// via a/ps, sender via sa/sps); both must be canonical.
var guildMembersSQL = `
	SELECT m.player_id, COALESCE(m.role_id, 0), COALESCE(ps.character_name, '')
	FROM dune.guild_members m
	JOIN dune.actors a ON a.id = m.player_id` + playerStateCanonicalJoin + `
	WHERE m.guild_id = $1
	ORDER BY m.role_id, m.player_id`

var guildInvitesSQL = `
	SELECT i.invite_id, i.player_id, COALESCE(ps.character_name, ''),
	       i.sender_player_id, COALESCE(sps.character_name, '')
	FROM dune.guild_invites i
	JOIN dune.actors a ON a.id = i.player_id` + playerStateCanonicalJoin + `
	LEFT JOIN dune.actors sa ON sa.id = i.sender_player_id` + playerStateCanonicalJoinOnSender + `
	WHERE i.guild_id = $1
	ORDER BY i.invite_id`

// playerStateCanonicalJoinOnSender is the sa/sps form for guildInvitesSQL's
// second (sender) resolution.
var playerStateCanonicalJoinOnSender = playerStateCanonicalJoinOn("sa", "sps")

func scanGuildMembers(ctx context.Context, pool *pgxpool.Pool, guildID int64) ([]guildMember, error) {
	rows, err := pool.Query(ctx, guildMembersSQL, guildID)
	if err != nil {
		return nil, fmt.Errorf("guild members %d: %w", guildID, err)
	}
	defer rows.Close()
	out := make([]guildMember, 0, 16)
	for rows.Next() {
		var m guildMember
		if err := rows.Scan(&m.PlayerID, &m.RoleID, &m.CharacterName); err != nil {
			return nil, fmt.Errorf("scan guild member: %w", err)
		}
		m.CharacterName = guildMemberDisplayName(m.CharacterName, m.PlayerID)
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanGuildInvites(ctx context.Context, pool *pgxpool.Pool, guildID int64) ([]guildInvite, error) {
	rows, err := pool.Query(ctx, guildInvitesSQL, guildID)
	if err != nil {
		return nil, fmt.Errorf("guild invites %d: %w", guildID, err)
	}
	defer rows.Close()
	out := make([]guildInvite, 0, 8)
	for rows.Next() {
		var iv guildInvite
		if err := rows.Scan(&iv.InviteID, &iv.PlayerID, &iv.CharacterName, &iv.SenderID, &iv.SenderName); err != nil {
			return nil, fmt.Errorf("scan guild invite: %w", err)
		}
		iv.CharacterName = guildMemberDisplayName(iv.CharacterName, iv.PlayerID)
		iv.SenderName = guildMemberDisplayName(iv.SenderName, iv.SenderID)
		out = append(out, iv)
	}
	return out, rows.Err()
}

func cmdFetchGuildDetail(ctx context.Context, pool *pgxpool.Pool, guildID int64) (guildDetail, error) {
	var d guildDetail
	err := pool.QueryRow(ctx, guildSummarySelect+`
		WHERE g.guild_id = $1`, guildID).Scan(
		&d.GuildID, &d.Name, &d.Description, &d.FactionID, &d.FactionName, &d.MemberCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return guildDetail{}, errGuildNotFound
		}
		return guildDetail{}, fmt.Errorf("guild %d: %w", guildID, err)
	}
	if d.Members, err = scanGuildMembers(ctx, pool, guildID); err != nil {
		return guildDetail{}, err
	}
	if d.Invites, err = scanGuildInvites(ctx, pool, guildID); err != nil {
		return guildDetail{}, err
	}
	return d, nil
}

// ── Guild mutations (#117 Phase B) ───────────────────────────────────────────
// Writes go through the game's own stored procs (dune.edit_guild_description,
// promote/demote_guild_member): each self-acquires pg_advisory_xact_lock(601145)
// and pg_notify('guild_notify_channel', ...) so the live game applies the change —
// the same safe pattern as faction mutations. Guild NAME has no game proc, so it
// is a raw UPDATE (lock-guarded, uniqueness-checked) that the game only reflects
// after it reloads guild data (e.g. a server restart).

const (
	guildRoleMember = 50
	guildRoleAdmin  = 100
)

var errGuildNameTaken = errors.New("guild name already taken")

// guildRoleSetProc picks the game proc for a role change: promoting to admin (100)
// transfers the single admin slot via promote_guild_member; any lower role goes
// through demote_guild_member, which refuses to demote the sitting admin.
func guildRoleSetProc(newRole int16) string {
	if newRole == guildRoleAdmin {
		return "promote_guild_member"
	}
	return "demote_guild_member"
}

func cmdEditGuildDescription(ctx context.Context, pool *pgxpool.Pool, guildID int64, desc string) error {
	if _, err := pool.Exec(ctx, `SELECT dune.edit_guild_description($1, $2)`, guildID, desc); err != nil {
		return fmt.Errorf("edit guild %d description: %w", guildID, err)
	}
	return nil
}

func cmdSetGuildMemberRole(ctx context.Context, pool *pgxpool.Pool, guildID, playerID int64, newRole int16) error {
	// Static query strings (no concatenation) selected by the allowlisted helper.
	var q string
	switch guildRoleSetProc(newRole) {
	case "promote_guild_member":
		q = `SELECT dune.promote_guild_member($1, $2, $3)`
	default:
		q = `SELECT dune.demote_guild_member($1, $2, $3)`
	}
	if _, err := pool.Exec(ctx, q, guildID, playerID, newRole); err != nil {
		return fmt.Errorf("set guild %d member %d role %d: %w", guildID, playerID, newRole, err)
	}
	return nil
}

// cmdEditGuildName renames a guild. No game proc exists for this, so it is a raw
// UPDATE wrapped in a transaction that takes the same advisory lock the game's
// guild procs use, and rejects a name already in use (case-insensitive, mirroring
// create_guild). No pg_notify verb exists for a rename, so the game only reflects
// the new name after it reloads guild data (e.g. a server restart).
func cmdEditGuildName(ctx context.Context, pool *pgxpool.Pool, guildID int64, name string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rename guild %d: %w", guildID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT dune.guilds_get_exclusive_operation_lock()`); err != nil {
		return fmt.Errorf("lock for rename guild %d: %w", guildID, err)
	}

	var taken bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM dune.guilds WHERE guild_name ILIKE $1 AND guild_id <> $2)`,
		name, guildID).Scan(&taken); err != nil {
		return fmt.Errorf("check guild name: %w", err)
	}
	if taken {
		return errGuildNameTaken
	}

	ct, err := tx.Exec(ctx, `UPDATE dune.guilds SET guild_name = $1 WHERE guild_id = $2`, name, guildID)
	if err != nil {
		return fmt.Errorf("rename guild %d: %w", guildID, err)
	}
	if ct.RowsAffected() == 0 {
		return errGuildNotFound
	}
	return tx.Commit(ctx)
}

// ── Landsraad (#117 Phase A — read-only) ─────────────────────────────────────
//
// The Landsraad is the weekly political endgame: a term cycle
// (landsraad_decree_term) with a 25-task board (landsraad_tasks, each keyed to a
// noble house) and electable server-wide decrees (landsraad_decrees). This is a
// read-only overview — the latest term + the decree catalogue + that term's task
// board. Faction/decree ids resolve to names; nullable election fields → "".

type landsraadTerm struct {
	TermID          int64     `json:"term_id"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	TestTerm        bool      `json:"test_term"`
	ReigningFaction string    `json:"reigning_faction"`
	ActiveDecree    string    `json:"active_decree"`
	ElectedDecree   string    `json:"elected_decree"`
	WinningFaction  string    `json:"winning_faction"`
}

type landsraadDecree struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Weight   float64 `json:"weight"`
	Disabled bool    `json:"disabled"`
}

type landsraadTask struct {
	ID             int64  `json:"id"`
	BoardIndex     int16  `json:"board_index"`
	House          string `json:"house"`
	Completed      bool   `json:"completed"`
	WinningFaction string `json:"winning_faction"`
	Sysselraad     bool   `json:"sysselraad"`
	GoalAmount     int    `json:"goal_amount"`
}

type landsraadOverview struct {
	Term    *landsraadTerm    `json:"term"`
	Decrees []landsraadDecree `json:"decrees"`
	Tasks   []landsraadTask   `json:"tasks"`
}

// landsraadHouseName strips the "DA_House" prefix from a task's house_name
// (e.g. "DA_HouseHagal" -> "Hagal"). Unprefixed values pass through unchanged.
func landsraadHouseName(raw string) string {
	return strings.TrimPrefix(raw, "DA_House")
}

func cmdFetchLandsraad(ctx context.Context, pool *pgxpool.Pool) (landsraadOverview, error) {
	var ov landsraadOverview
	term, err := fetchLandsraadTerm(ctx, pool)
	if err != nil {
		return ov, err
	}
	ov.Term = term
	if ov.Decrees, err = fetchLandsraadDecrees(ctx, pool); err != nil {
		return ov, err
	}
	if term != nil {
		if ov.Tasks, err = fetchLandsraadTasks(ctx, pool, term.TermID); err != nil {
			return ov, err
		}
	}
	return ov, nil
}

const landsraadTermSQL = `
	SELECT t.term_id, t.start_time, t.end_time, t.test_term,
	       COALESCE(rf.name, ''), COALESCE(ad.decree_name, ''),
	       COALESCE(ed.decree_name, ''), COALESCE(wf.name, '')
	FROM dune.landsraad_decree_term t
	LEFT JOIN dune.factions rf ON rf.id = t.reigning_faction_id
	LEFT JOIN dune.landsraad_decrees ad ON ad.id = t.active_decree_id
	LEFT JOIN dune.landsraad_decrees ed ON ed.id = t.elected_decree_id
	LEFT JOIN dune.factions wf ON wf.id = t.winning_faction_id
	ORDER BY t.term_id DESC
	LIMIT 1`

// fetchLandsraadTerm returns the latest term, or (nil, nil) when none exist.
func fetchLandsraadTerm(ctx context.Context, pool *pgxpool.Pool) (*landsraadTerm, error) {
	var t landsraadTerm
	err := pool.QueryRow(ctx, landsraadTermSQL).Scan(
		&t.TermID, &t.StartTime, &t.EndTime, &t.TestTerm,
		&t.ReigningFaction, &t.ActiveDecree, &t.ElectedDecree, &t.WinningFaction)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("landsraad term: %w", err)
	}
	return &t, nil
}

func fetchLandsraadDecrees(ctx context.Context, pool *pgxpool.Pool) ([]landsraadDecree, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, decree_name, weight::float8, disabled
		FROM dune.landsraad_decrees ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("landsraad decrees: %w", err)
	}
	defer rows.Close()
	out := make([]landsraadDecree, 0, 16)
	for rows.Next() {
		var d landsraadDecree
		if err := rows.Scan(&d.ID, &d.Name, &d.Weight, &d.Disabled); err != nil {
			return nil, fmt.Errorf("scan decree: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const landsraadTasksSQL = `
	SELECT t.id, t.board_index, t.house_name, t.completed,
	       COALESCE(wf.name, ''), t.sysselraad, t.goal_amount
	FROM dune.landsraad_tasks t
	LEFT JOIN dune.factions wf ON wf.id = t.winning_faction_id
	WHERE t.term_id = $1
	ORDER BY t.board_index, t.id`

func fetchLandsraadTasks(ctx context.Context, pool *pgxpool.Pool, termID int64) ([]landsraadTask, error) {
	rows, err := pool.Query(ctx, landsraadTasksSQL, termID)
	if err != nil {
		return nil, fmt.Errorf("landsraad tasks %d: %w", termID, err)
	}
	defer rows.Close()
	out := make([]landsraadTask, 0, 32)
	for rows.Next() {
		var tk landsraadTask
		var house string
		if err := rows.Scan(&tk.ID, &tk.BoardIndex, &house, &tk.Completed, &tk.WinningFaction, &tk.Sysselraad, &tk.GoalAmount); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tk.House = landsraadHouseName(house)
		out = append(out, tk)
	}
	return out, rows.Err()
}

// factionTrendPoint is one day's per-faction value (faction name -> value).
type factionTrendPoint struct {
	Day    string             `json:"day"`
	Values map[string]float64 `json:"values"`
}

// factionTrends is the faction-growth time series (#130 ext): a sorted faction
// list (the chart's lines) + per-day values. The data is approximate — it comes
// from stat_snapshots, which only capture players who were online during a poll,
// and uses each account's CURRENT faction.
type factionTrends struct {
	Metric   string              `json:"metric"`
	Factions []string            `json:"factions"`
	Points   []factionTrendPoint `json:"points"`
}

// factionSnapAcc is an intermediate accumulator used by bucketFactionTrends.
type factionSnapAcc struct {
	sum float64
	n   int
}

// accumulateFactionSnaps groups snapshots by day and faction into accumulators.
// metric "level" → sum of xpToLevel; otherwise → sum of Solaris.
func accumulateFactionSnaps(snaps []daySnap, acctFaction map[int64]string, metric string) (
	byDay map[string]map[string]*factionSnapAcc,
	order []string,
	factionSet map[string]bool,
) {
	byDay = map[string]map[string]*factionSnapAcc{}
	factionSet = map[string]bool{}
	for _, s := range snaps {
		fac := acctFaction[s.AccountID]
		if fac == "" {
			fac = "Unaligned"
		}
		factionSet[fac] = true
		if byDay[s.Day] == nil {
			byDay[s.Day] = map[string]*factionSnapAcc{}
			order = append(order, s.Day)
		}
		a := byDay[s.Day][fac]
		if a == nil {
			a = &factionSnapAcc{}
			byDay[s.Day][fac] = a
		}
		if metric == "level" {
			a.sum += float64(xpToLevel(s.CharXP))
		} else {
			a.sum += float64(s.Solaris)
		}
		a.n++
	}
	return
}

// bucketFactionTrends aggregates per-account daily snapshots into a per-day,
// per-faction series. Pure + testable (xpToLevel is its only dependency).
// metric "level" → average character level per faction; otherwise → summed Solaris.
func bucketFactionTrends(snaps []daySnap, acctFaction map[int64]string, metric string) factionTrends {
	byDay, order, factionSet := accumulateFactionSnaps(snaps, acctFaction, metric)
	sort.Strings(order)
	factions := make([]string, 0, len(factionSet))
	for f := range factionSet {
		factions = append(factions, f)
	}
	sort.Strings(factions)
	points := make([]factionTrendPoint, 0, len(order))
	for _, day := range order {
		vals := make(map[string]float64, len(byDay[day]))
		for fac, a := range byDay[day] {
			if metric == "level" {
				vals[fac] = a.sum / float64(a.n)
			} else {
				vals[fac] = a.sum
			}
		}
		points = append(points, factionTrendPoint{Day: day, Values: vals})
	}
	return factionTrends{Metric: metric, Factions: factions, Points: points}
}

// scanLabeledCounts runs a (label text, count bigint) query and returns the
// rows as a never-nil slice (empty → [], so the JSON is [] not null).
func scanLabeledCounts(ctx context.Context, pool *pgxpool.Pool, query string, args ...any) ([]labeledCount, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []labeledCount{}
	for rows.Next() {
		var c labeledCount
		if err := rows.Scan(&c.Label, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// scanFactionStats runs the per-faction (faction, players, solaris, scrip) query
// and returns a never-nil slice. NUMERIC balance sums scan cleanly into int64.
func scanFactionStats(ctx context.Context, pool *pgxpool.Pool, query string, args ...any) ([]factionStat, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []factionStat{}
	for rows.Next() {
		var fs factionStat
		if err := rows.Scan(&fs.Faction, &fs.Players, &fs.Solaris, &fs.Scrip); err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

// cmdFetchCharXPList returns every player character's cumulative character XP,
// for the server-wide average character level (#130).
func cmdFetchCharXPList(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	rows, err := pool.Query(ctx, serverCharXPSQL, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("server char xp: %w", err)
	}
	defer rows.Close()

	var xps []int64
	for rows.Next() {
		var xp int64
		if err := rows.Scan(&xp); err != nil {
			return nil, fmt.Errorf("scan char xp: %w", err)
		}
		xps = append(xps, xp)
	}
	return xps, rows.Err()
}

// averageLevel is the mean character level across the given cumulative-XP values
// (via xpToLevel). Averaging per-character levels — not raw XP — is the intent,
// since the XP→level curve is non-linear. Empty input → 0. Rounding is left to
// the client.
func averageLevel(xps []int64) float64 {
	if len(xps) == 0 {
		return 0
	}
	sum := 0
	for _, xp := range xps {
		sum += xpToLevel(xp)
	}
	return float64(sum) / float64(len(xps))
}

func cmdFetchInventory(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgInventory{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT i.id, i.template_id, i.stack_size, i.quality_level,
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'), 'N/A'),
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'), 'N/A')
			FROM dune.items i
			JOIN dune.inventories inv ON i.inventory_id = inv.id
			WHERE inv.actor_id = $1::bigint
			ORDER BY i.template_id`, playerID)
		if err != nil {
			return msgInventory{err: err}
		}
		defer rows.Close()

		var items []itemInfo
		for rows.Next() {
			var it itemInfo
			if err := rows.Scan(&it.ID, &it.TemplateID, &it.StackSize, &it.Quality, &it.Durability, &it.MaxDurability); err != nil {
				continue
			}
			it.Name = itemData.Names[strings.ToLower(it.TemplateID)]
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return msgInventory{err: err}
		}
		return msgInventory{rows: items}
	}
}

func cmdFetchCurrency(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgCurrency{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT player_controller_id, currency_id, balance
		FROM dune.player_virtual_currency_balances
		ORDER BY player_controller_id, currency_id`)
	if err != nil {
		return msgCurrency{err: err}
	}
	defer rows.Close()

	var out []currencyRow
	for rows.Next() {
		var r currencyRow
		if err := rows.Scan(&r.PlayerID, &r.CurrencyID, &r.Balance); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgCurrency{err: err}
	}
	return msgCurrency{rows: out}
}

func cmdFetchFactions(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgFactions{err: fmt.Errorf("not connected")}
	}
	ctx := context.Background()
	scripID, err := resolveScripCurrencyID(ctx, pool)
	if err != nil {
		return msgFactions{err: err}
	}
	rows, err := pool.Query(ctx, `
		SELECT pfr.actor_id, pfr.faction_id, f.name, pfr.reputation_amount,
		       COALESCE(vcb.balance, 0)
		FROM dune.player_faction_reputation pfr
		JOIN dune.factions f ON f.id = pfr.faction_id
		LEFT JOIN dune.player_virtual_currency_balances vcb
			ON vcb.player_controller_id = pfr.actor_id
			AND vcb.currency_id = $1::smallint
		ORDER BY pfr.actor_id, pfr.faction_id`, scripID)
	if err != nil {
		return msgFactions{err: err}
	}
	defer rows.Close()

	var out []factionRep
	for rows.Next() {
		var r factionRep
		if err := rows.Scan(&r.ActorID, &r.FactionID, &r.FactionName, &r.Reputation, &r.Scrips); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgFactions{err: err}
	}
	return msgFactions{rows: out, scripCurrencyID: scripID}
}

func cmdFetchSpecs(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgSpecs{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT player_id, track_type::text, xp_amount, level
		FROM dune.specialization_tracks
		ORDER BY player_id, track_type`)
	if err != nil {
		return msgSpecs{err: err}
	}
	defer rows.Close()

	var out []specTrack
	for rows.Next() {
		var r specTrack
		if err := rows.Scan(&r.PlayerID, &r.TrackType, &r.XP, &r.Level); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgSpecs{err: err}
	}
	return msgSpecs{rows: out}
}

func sqlHeaderNames(rows pgx.Rows) []string {
	descs := rows.FieldDescriptions()
	headers := make([]string, len(descs))
	for i, desc := range descs {
		headers[i] = string(desc.Name)
	}
	return headers
}

func collectSQLRows(rows pgx.Rows, limit int) ([][]any, bool) {
	collected := make([][]any, 0, limit)
	for rows.Next() && len(collected) < limit {
		values, err := rows.Values()
		if err != nil {
			continue
		}
		collected = append(collected, values)
	}
	return collected, len(collected) == limit
}

func formatSQLRow(values []any) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%v", value)
	}
	return strings.Join(parts, " │ ")
}

func buildSQLResult(headers []string, rows [][]any, truncated bool) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(headers, " │ "))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", 80))
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString(formatSQLRow(row))
		sb.WriteString("\n")
	}
	if truncated {
		sb.WriteString("… (limited to 200 rows)\n")
	}
	return sb.String()
}

func formatSQLStringRows(rows [][]any) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for j, v := range row {
			cells[j] = fmt.Sprintf("%v", v)
		}
		out[i] = cells
	}
	return out
}

func cmdRunSQL(pool *pgxpool.Pool, sql string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgSQL{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), sql)
		if err != nil {
			return msgSQL{err: err}
		}
		defer rows.Close()

		headers := sqlHeaderNames(rows)
		resultRows, truncated := collectSQLRows(rows, 200)
		return msgSQL{
			result:    buildSQLResult(headers, resultRows, truncated),
			headers:   headers,
			rows:      formatSQLStringRows(resultRows),
			truncated: truncated,
		}
	}
}

func cmdGiveItem(pool *pgxpool.Pool, playerID int64, template string, qty, quality int64) Cmd {
	return func() Msg {
		return runGiveItem(pool, playerID, template, qty, quality)
	}
}

func runGiveItem(pool *pgxpool.Pool, playerID int64, template string, qty, quality int64) Msg {
	if pool == nil {
		return msgMutate{err: fmt.Errorf("not connected")}
	}
	trimmedTemplate, err := validateGiveItemInput(playerID, template, qty)
	if err != nil {
		return msgMutate{err: err}
	}
	template = trimmedTemplate

	ctx := context.Background()
	inv, err := findGiveItemInventory(ctx, pool, playerID)
	if err != nil {
		return msgMutate{err: err}
	}
	state, err := loadGiveItemInventoryState(ctx, pool, inv.id, template, quality, inv.hasVolumeCap)
	if err != nil {
		return msgMutate{err: err}
	}
	stackMax, known, err := resolveStackMax(ctx, pool, template, quality)
	if err != nil {
		return msgMutate{err: err}
	}
	stackMax = effectiveStackMax(stackMax, known, qty)
	if err := ensureGiveItemVolumeCapacity(ctx, pool, inv, state, template, qty); err != nil {
		return msgMutate{err: err}
	}

	updates, newStacks := planGiveItemStacks(qty, stackMax, state.stacks)
	if err := ensureGiveItemSlotCapacity(inv, state, len(newStacks)); err != nil {
		return msgMutate{err: err}
	}
	if err := applyGiveItemChanges(ctx, pool, inv.id, template, quality, state.maxPos, updates, newStacks); err != nil {
		return msgMutate{err: err}
	}
	return msgMutate{ok: formatGiveItemResult(playerID, template, qty, len(updates), len(newStacks))}
}

type giveItemInventory struct {
	id           int64
	maxSlots     int
	maxVolume    float64
	hasSlotCap   bool
	hasVolumeCap bool
}

type giveItemStackSlot struct {
	id   int64
	size int64
}

type giveItemInventoryState struct {
	stacks     []giveItemStackSlot
	usedSlots  int
	usedVolume float64
	maxPos     int64
}

type giveItemStackUpdate struct {
	id  int64
	add int64
}

func validateGiveItemInput(playerID int64, template string, qty int64) (string, error) {
	if playerID == 0 {
		return "", fmt.Errorf("player ID required")
	}
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("item template required")
	}
	if qty <= 0 {
		return "", fmt.Errorf("quantity must be > 0")
	}
	return template, nil
}

func findGiveItemInventory(ctx context.Context, pool *pgxpool.Pool, playerID int64) (giveItemInventory, error) {
	var inv giveItemInventory
	err := pool.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint AND inventory_type = 0
		LIMIT 1`, playerID).Scan(&inv.id, &inv.maxSlots, &inv.maxVolume)
	if err == nil {
		inv.hasSlotCap = inv.maxSlots > 0
		inv.hasVolumeCap = inv.maxVolume > 0
		return inv, nil
	}
	err = pool.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint
		LIMIT 1`, playerID).Scan(&inv.id, &inv.maxSlots, &inv.maxVolume)
	if err != nil {
		return giveItemInventory{}, fmt.Errorf("find inventory: %w", err)
	}
	inv.hasSlotCap = inv.maxSlots > 0
	inv.hasVolumeCap = inv.maxVolume > 0
	return inv, nil
}

func loadGiveItemInventoryState(ctx context.Context, pool *pgxpool.Pool, invID int64, template string, quality int64, includeVolume bool) (giveItemInventoryState, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, template_id, stack_size, quality_level, volume_override, position_index
		FROM dune.items
		WHERE inventory_id = $1::bigint`, invID)
	if err != nil {
		return giveItemInventoryState{}, err
	}
	defer rows.Close()

	state := giveItemInventoryState{maxPos: -1}
	for rows.Next() {
		var id int64
		var tmpl string
		var stackSize int64
		var qLevel int64
		var vol pgtype.Float8
		var pos int64
		if err := rows.Scan(&id, &tmpl, &stackSize, &qLevel, &vol, &pos); err != nil {
			continue
		}
		state.usedSlots++
		if pos > state.maxPos {
			state.maxPos = pos
		}
		if qLevel == quality && tmpl == template {
			state.stacks = append(state.stacks, giveItemStackSlot{id: id, size: stackSize})
		}
		if includeVolume {
			state.usedVolume += inventoryItemVolume(tmpl, vol) * float64(stackSize)
		}
	}
	if err := rows.Err(); err != nil {
		return giveItemInventoryState{}, err
	}
	return state, nil
}

func inventoryItemVolume(template string, vol pgtype.Float8) float64 {
	if vol.Valid && vol.Float64 > 0 {
		return vol.Float64
	}
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok {
			return rule.Volume // 0 is valid — item takes no volume
		}
		if itemData.DefaultVolume > 0 {
			return itemData.DefaultVolume
		}
		// Unknown volume: treat as 0 (no space consumed).
		return 0
	}
	if itemData.DefaultVolume > 0 {
		return itemData.DefaultVolume
	}
	return 0
}

func ensureGiveItemVolumeCapacity(
	ctx context.Context,
	pool *pgxpool.Pool,
	inv giveItemInventory,
	state giveItemInventoryState,
	template string,
	qty int64,
) error {
	if !inv.hasVolumeCap {
		return nil
	}
	perItemVol, err := resolveItemVolume(ctx, pool, template)
	if err != nil {
		return err
	}
	if perItemVol <= 0 {
		return nil
	}
	availableVol := max(inv.maxVolume-state.usedVolume, 0)
	maxByVolume := int64(math.Floor(availableVol / perItemVol))
	if maxByVolume < qty {
		return fmt.Errorf(
			"over weight limit: room for %d more %s (%.2f/%.2f volume used)",
			maxByVolume, template, state.usedVolume, inv.maxVolume)
	}
	return nil
}

func fillExistingStacks(sorted []giveItemStackSlot, remaining, stackMax int64) ([]giveItemStackUpdate, int64) {
	updates := make([]giveItemStackUpdate, 0, len(sorted))
	for _, st := range sorted {
		if remaining == 0 {
			break
		}
		space := stackMax - st.size
		if space <= 0 {
			continue
		}
		add := min(space, remaining)
		updates = append(updates, giveItemStackUpdate{id: st.id, add: add})
		remaining -= add
	}
	return updates, remaining
}

func planGiveItemStacks(qty, stackMax int64, stacks []giveItemStackSlot) ([]giveItemStackUpdate, []int64) {
	sorted := make([]giveItemStackSlot, len(stacks))
	copy(sorted, stacks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].size > sorted[j].size
	})
	remaining := qty
	var updates []giveItemStackUpdate
	if stackMax > 1 {
		updates, remaining = fillExistingStacks(sorted, remaining, stackMax)
	} else {
		updates = make([]giveItemStackUpdate, 0)
	}
	newStackCap := 0
	if stackMax > 0 {
		newStackCap = int((remaining + stackMax - 1) / stackMax)
	}
	newStacks := make([]int64, 0, newStackCap)
	for remaining > 0 {
		size := min(stackMax, remaining)
		newStacks = append(newStacks, size)
		remaining -= size
	}
	return updates, newStacks
}

func ensureGiveItemSlotCapacity(inv giveItemInventory, state giveItemInventoryState, newStackCount int) error {
	if !inv.hasSlotCap {
		return nil
	}
	freeSlots := inv.maxSlots - state.usedSlots
	if freeSlots < newStackCount {
		return fmt.Errorf("inventory full: need %d free slots, have %d", newStackCount, freeSlots)
	}
	return nil
}

func applyGiveItemChanges(
	ctx context.Context,
	pool *pgxpool.Pool,
	invID int64,
	template string,
	quality int64,
	maxPos int64,
	updates []giveItemStackUpdate,
	newStacks []int64,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, u := range updates {
		if _, err := tx.Exec(ctx, `
			UPDATE dune.items
			SET stack_size = stack_size + $1::bigint
			WHERE id = $2::bigint`, u.add, u.id); err != nil {
			return err
		}
	}

	nextPos := maxPos + 1
	for _, size := range newStacks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO dune.items (inventory_id, stack_size, position_index, template_id, quality_level, stats)
			VALUES ($1::bigint, $2::bigint, $3::bigint, $4::text, $5::bigint, '{}'::jsonb)`,
			invID, size, nextPos, template, quality); err != nil {
			return err
		}
		nextPos++
	}

	return tx.Commit(ctx)
}

func formatGiveItemResult(playerID int64, template string, qty int64, toppedUp, created int) string {
	msg := fmt.Sprintf("Added %d × %s to player %d", qty, template, playerID)
	if toppedUp > 0 || created > 0 {
		return fmt.Sprintf(
			"Added %d × %s to player %d (%d stack(s) topped up, %d new stack(s))",
			qty, template, playerID, toppedUp, created)
	}
	return msg
}

type inventoryCapacityProfile struct {
	id           int64
	maxSlots     int
	maxVolume    float64
	hasSlotCap   bool
	hasVolumeCap bool
}

type inventoryUsage struct {
	usedSlots  int
	usedVolume float64
}

func loadBackpackCapacity(ctx context.Context, pool *pgxpool.Pool, playerID int64) (inventoryCapacityProfile, bool) {
	var profile inventoryCapacityProfile
	err := pool.QueryRow(ctx, `
		SELECT id, COALESCE(max_item_count, -1), COALESCE(max_item_volume, -1)
		FROM dune.inventories
		WHERE actor_id = $1::bigint AND inventory_type = 0
		LIMIT 1`, playerID).Scan(&profile.id, &profile.maxSlots, &profile.maxVolume)
	if err != nil {
		// No inventory found — cannot validate; let the game server decide.
		return inventoryCapacityProfile{}, false
	}
	profile.hasSlotCap = profile.maxSlots > 0
	profile.hasVolumeCap = profile.maxVolume > 0
	return profile, true
}

func loadInventoryUsage(ctx context.Context, pool *pgxpool.Pool, inventoryID int64, includeVolume bool) (inventoryUsage, error) {
	rows, err := pool.Query(ctx, `
		SELECT template_id, stack_size, volume_override
		FROM dune.items
		WHERE inventory_id = $1::bigint`, inventoryID)
	if err != nil {
		return inventoryUsage{}, err
	}
	defer rows.Close()

	usage := inventoryUsage{}
	for rows.Next() {
		var templateID string
		var stackSize int64
		var volumeOverride pgtype.Float8
		if err := rows.Scan(&templateID, &stackSize, &volumeOverride); err != nil {
			continue
		}
		usage.usedSlots++
		if includeVolume {
			usage.usedVolume += inventoryItemVolume(templateID, volumeOverride) * float64(stackSize)
		}
	}
	return usage, nil
}

func maxItemsByVolume(maxVolume, usedVolume, perItemVol float64) int64 {
	availableVolume := maxVolume - usedVolume
	if availableVolume < 0 {
		availableVolume = 0
	}
	return int64(math.Floor(availableVolume / perItemVol))
}

func requiredStackCount(qty, stackMax int64) int {
	return int((qty + stackMax - 1) / stackMax)
}

func checkInventoryVolumeLimit(
	ctx context.Context,
	pool *pgxpool.Pool,
	profile inventoryCapacityProfile,
	usage inventoryUsage,
	template string,
	qty int64,
) error {
	if !profile.hasVolumeCap {
		return nil
	}
	perItemVol, err := resolveItemVolume(ctx, pool, template)
	if err != nil || perItemVol <= 0 {
		return nil
	}
	maxByVolume := maxItemsByVolume(profile.maxVolume, usage.usedVolume, perItemVol)
	if maxByVolume < qty {
		return fmt.Errorf(
			"over weight limit: room for %d more %s (%.2f/%.2f volume used)",
			maxByVolume, template, usage.usedVolume, profile.maxVolume)
	}
	return nil
}

func checkInventorySlotLimit(ctx context.Context, pool *pgxpool.Pool, profile inventoryCapacityProfile, usage inventoryUsage, template string, qty int64) error {
	if !profile.hasSlotCap {
		return nil
	}
	stackMax, known, err := resolveStackMax(ctx, pool, template, 0)
	if err != nil {
		known = false
	}
	stackMax = effectiveStackMax(stackMax, known, qty)
	freeSlots := profile.maxSlots - usage.usedSlots
	newStacks := requiredStackCount(qty, stackMax)
	if freeSlots < newStacks {
		return fmt.Errorf(
			"inventory full: need %d free slots, have %d",
			newStacks, freeSlots)
	}
	return nil
}

// checkInventoryCapacity verifies that qty items of template fit in the player's
// backpack (inventory_type=0). Returns an error if the inventory is over volume
// or slot limits. Used to pre-validate RMQ give-item commands since the game
// server's cheat function bypasses these checks.
// cmdListWelcomeOnlineAccounts returns currently-online characters eligible for
// the welcome package using the provided pool. Online-only so the live RMQ grant
// path applies — this is the "on first login" trigger.
func cmdListWelcomeOnlineAccounts(ctx context.Context, pool *pgxpool.Pool) ([]welcomeAccount, error) {
	if pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	rows, err := pool.Query(ctx, `
		SELECT ps.account_id, ps.player_pawn_id,
		       COALESCE(ac."user", ''), COALESCE(ps.character_name, ''),
		       COALESCE(a.map, '')
		FROM dune.player_state ps
		JOIN dune.actors a ON a.id = ps.player_pawn_id
		JOIN dune.accounts ac ON ac.id = a.owner_account_id
		WHERE ps.online_status = 'Online' AND ps.player_pawn_id IS NOT NULL AND ps.account_id <> $1`, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("list welcome accounts: %w", err)
	}
	defer rows.Close()

	out := make([]welcomeAccount, 0)
	for rows.Next() {
		var acc welcomeAccount
		if err := rows.Scan(&acc.AccountID, &acc.PawnID, &acc.FlsID, &acc.CharacterName, &acc.Region); err != nil {
			return nil, fmt.Errorf("scan welcome account: %w", err)
		}
		out = append(out, acc)
	}
	return out, rows.Err()
}

// listWelcomeOnlineAccounts is the global-pool wrapper used by the handler path.
// Handler conversion to dbFromCtx is Phase 3.
func listWelcomeOnlineAccounts(ctx context.Context) ([]welcomeAccount, error) {
	return cmdListWelcomeOnlineAccounts(ctx, globalDB)
}

// cmdFetchWelcomeAccount returns one account's welcome-grant identity (pawn id,
// FLS id, character name) by account id — the same joins as
// listWelcomeOnlineAccounts but without the online filter, so a manual override
// can target any player regardless of presence.
func cmdFetchWelcomeAccount(ctx context.Context, pool *pgxpool.Pool, accountID int64) (welcomeAccount, error) {
	if pool == nil {
		return welcomeAccount{}, fmt.Errorf("not connected")
	}
	row := pool.QueryRow(ctx, `
		SELECT ps.account_id, ps.player_pawn_id,
		       COALESCE(ac."user", ''), COALESCE(ps.character_name, '')
		FROM dune.player_state ps
		JOIN dune.actors a ON a.id = ps.player_pawn_id
		JOIN dune.accounts ac ON ac.id = a.owner_account_id
		WHERE ps.account_id = $1`, accountID)
	var acc welcomeAccount
	if err := row.Scan(&acc.AccountID, &acc.PawnID, &acc.FlsID, &acc.CharacterName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return welcomeAccount{}, errNotFound
		}
		return welcomeAccount{}, fmt.Errorf("fetch welcome account %d: %w", accountID, err)
	}
	return acc, nil
}

// checkInventoryCapacityPool verifies that qty items of template fit in the
// player's inventory using the given pool.
func checkInventoryCapacityPool(ctx context.Context, pool *pgxpool.Pool, playerID int64, template string, qty int64) error {
	if pool == nil {
		return fmt.Errorf("not connected")
	}
	profile, ok := loadBackpackCapacity(ctx, pool, playerID)
	if !ok {
		return nil
	}
	if !profile.hasSlotCap && !profile.hasVolumeCap {
		return nil
	}
	usage, err := loadInventoryUsage(ctx, pool, profile.id, profile.hasVolumeCap)
	if err != nil {
		return nil
	}
	if err := checkInventoryVolumeLimit(ctx, pool, profile, usage, template, qty); err != nil {
		return err
	}
	if err := checkInventorySlotLimit(ctx, pool, profile, usage, template, qty); err != nil {
		return err
	}
	return nil
}

func checkInventoryCapacity(ctx context.Context, playerID int64, template string, qty int64) error {
	return checkInventoryCapacityPool(ctx, globalDB, playerID, template, qty)
}

// cmdGrantLive inserts into landsraad_house_rewards which fires a pg_notify trigger.
// The game server receives the notification immediately and shows "Claim Rewards" to the player.
func cmdGrantLive(pool *pgxpool.Pool, controllerID int64, templateID string, amount int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := pool.Exec(context.Background(), `
			DELETE FROM dune.landsraad_house_rewards
			WHERE player_id = $1 AND house_name = 'AdminGrant'`,
			controllerID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("grant live clear: %w", err)}
		}
		_, err = pool.Exec(context.Background(), `
			INSERT INTO dune.landsraad_house_rewards (player_id, house_name, amount, template_id, last_updated)
			VALUES ($1, 'AdminGrant', $2, $3, NOW())`,
			controllerID, amount, templateID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("grant live: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Queued live grant: %dx %s — player will see Claim Rewards", amount, templateID)}
	}
}

func cmdGiveCurrency(pool *pgxpool.Pool, playerID int64, amount int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		// Route through adjust_player_virtual_currency_balance for audit logging
		// and negative-balance guards. The casts match the live function signature.
		_, err := pool.Exec(ctx, `
			SELECT dune.adjust_player_virtual_currency_balance(
				$1::bigint,
				dune.get_solaris_id(),
				$2::bigint
			)`,
			playerID, amount)
		if err != nil {
			return msgMutate{err: err}
		}
		var balance int64
		_ = pool.QueryRow(ctx, `
			SELECT balance FROM dune.player_virtual_currency_balances
			WHERE player_controller_id = $1::bigint AND currency_id = dune.get_solaris_id()`,
			playerID).Scan(&balance)
		return msgMutate{ok: fmt.Sprintf(
			"Added %d Solaris to player %d — new balance %d",
			amount, playerID, balance)}
	}
}

// cmdGiveCurrencyCtx is the context-aware, injectable form of cmdGiveCurrency,
// used by the Discord bot and any caller that needs explicit ctx/db injection
// for testability. It routes through the same audit-logged DB function.
func cmdGiveCurrencyCtx(ctx context.Context, db *pgxpool.Pool, controllerID, amount int64) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("database not connected")
	}
	if controllerID == 0 {
		return 0, fmt.Errorf("player controller ID required")
	}
	_, err := db.Exec(ctx, `
		SELECT dune.adjust_player_virtual_currency_balance(
			$1::bigint,
			dune.get_solaris_id(),
			$2::bigint
		)`, controllerID, amount)
	if err != nil {
		return 0, fmt.Errorf("adjust currency player=%d: %w", controllerID, err)
	}
	var balance int64
	if err := db.QueryRow(ctx, `
		SELECT balance FROM dune.player_virtual_currency_balances
		WHERE player_controller_id = $1::bigint AND currency_id = dune.get_solaris_id()`,
		controllerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("read new balance player=%d: %w", controllerID, err)
	}
	return balance, nil
}

// findPlayersByNameSQL uses the canonical player_state join (#290) so an
// account with duplicate state rows can't make a unique character name look
// ambiguous to the Discord command that consumes this lookup.
var findPlayersByNameSQL = `
	SELECT a.id,
	       COALESCE(a.owner_account_id, 0),
	       COALESCE(ps.character_name, ''),
	       COALESCE(ps.player_controller_id, 0),
	       COALESCE(ac."user", ''),
	       a.class,
	       COALESCE(a.map, ''),
	       COALESCE(af.faction_id, 0),
	       COALESCE(ps.online_status::text, 'Offline')
	FROM dune.actors a` + playerStateCanonicalJoin + `
	LEFT JOIN dune.encrypted_accounts e ON e.id = a.owner_account_id
	LEFT JOIN dune.accounts ac ON ac.id = a.owner_account_id` + factionByAccountJoin + `
	WHERE ps.character_name ILIKE '%' || $1 || '%'
	  AND a.class ILIKE '%PlayerCharacter%'
	  AND a.owner_account_id <> $2
	ORDER BY a.id`

// cmdFindPlayersByName looks up player characters whose character_name contains
// the given substring (case-insensitive). Returns all matches; the caller is
// responsible for handling 0 (not found) or >1 (ambiguous) results.
// The GM identity account is excluded, matching the cmdFetchPlayers convention.
func cmdFindPlayersByName(ctx context.Context, db *pgxpool.Pool, name string) ([]playerInfo, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := db.Query(ctx, findPlayersByNameSQL, name, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("find players by name %q: %w", name, err)
	}
	defer rows.Close()

	var players []playerInfo
	for rows.Next() {
		var p playerInfo
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.ControllerID, &p.FLSID, &p.Class, &p.Map, &p.FactionID, &p.OnlineStatus); err != nil {
			return nil, fmt.Errorf("scan player row: %w", err)
		}
		p.Class = shortClass(p.Class)
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find players by name %q rows: %w", name, err)
	}
	return players, nil
}

func cmdGiveFactionRep(pool *pgxpool.Pool, actorID int64, factionID int16, delta int32) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		return applyFactionRepDelta(ctx, pool, actorID, factionID, delta)
	}
}

func cmdGiveLandsraadScrip(pool *pgxpool.Pool, actorID int64, delta int32) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		if actorID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		currencyID, err := resolveScripCurrencyID(ctx, pool)
		if err != nil {
			return msgMutate{err: err}
		}
		_, err = pool.Exec(ctx, `
			SELECT dune.adjust_player_virtual_currency_balance($1::bigint, $2::smallint, $3::bigint)`,
			actorID, currencyID, int64(delta))
		if err != nil {
			return msgMutate{err: err}
		}
		var balance int64
		_ = pool.QueryRow(ctx, `
			SELECT balance FROM dune.player_virtual_currency_balances
			WHERE player_controller_id = $1::bigint AND currency_id = $2::smallint`,
			actorID, currencyID).Scan(&balance)
		return msgMutate{ok: fmt.Sprintf(
			"Added %d scrips (currency %d) to player %d — new balance %d",
			delta, currencyID, actorID, balance)}
	}
}

func cmdAwardXP(pool *pgxpool.Pool, playerID int64, trackType string, delta int32) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		const maxXP int32 = 44182
		if delta > maxXP {
			delta = maxXP
		}
		res, err := pool.Exec(context.Background(), `
			UPDATE dune.specialization_tracks
			SET xp_amount = GREATEST(LEAST(xp_amount + $1::integer, $4::integer), 0)
			WHERE player_id = $2::bigint AND track_type::text = $3::text`,
			delta, playerID, trackType, maxXP)
		if err != nil {
			return msgMutate{err: err}
		}
		if res.RowsAffected() == 0 {
			_, err = pool.Exec(context.Background(), `
				INSERT INTO dune.specialization_tracks (player_id, track_type, xp_amount, level)
				VALUES ($1::bigint, $2::dune.specializationtracktype, LEAST($3::integer, $4::integer), 0::real)`,
				playerID, trackType, delta, maxXP)
			if err != nil {
				return msgMutate{err: err}
			}
		}
		return msgMutate{ok: fmt.Sprintf("Awarded %d XP (%s) to player %d", delta, trackType, playerID)}
	}
}

func cmdRenameCharacter(pool *pgxpool.Pool, accountID int64, name string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return msgMutate{err: fmt.Errorf("name required")}
		}
		_, err := pool.Exec(context.Background(), `SELECT dune.set_character_name($1, $2)`, accountID, name)
		if err != nil {
			return msgMutate{err: fmt.Errorf("rename character: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Renamed to %s", name)}
	}
}

// deleteCharacterParams bundles the injectable dependencies for
// processDeleteCharacter so validation/orchestration can be unit-tested
// without a live DB.
type deleteCharacterParams struct {
	accountID     int64
	reason        string
	resolveUser   func(accountID int64) (string, error)
	deleteAccount func(user, reason string) (bool, error)
	// cleanupOrphans, if set, runs after a successful deleteAccount to clean
	// up whatever deleteAccount itself doesn't (#290: dune.delete_account
	// deletes the player's actors and account row but never the
	// dune.player_state row, leaving it orphaned — see cmdDeleteCharacter's
	// wiring for what it actually does). Optional so existing/older callers
	// that don't need cleanup keep working unchanged.
	cleanupOrphans func() error
	// captureSnapshot, if set, runs BEFORE deleteAccount — the opposite timing
	// from cleanupOrphans — since the pawn/controller actors a snapshot needs
	// to scope its capture stop existing the moment delete_account succeeds.
	// Optional: nil when the admin didn't request a backup, or on any caller
	// that doesn't support it.
	captureSnapshot func() error
}

// processDeleteCharacter validates input, resolves the accounts."user" FLS id,
// and invokes dune.delete_account. A false result means the proc matched no
// account row — treated as an error so the caller surfaces a failure rather
// than a misleading success. captureSnapshot (if set) runs after validation
// but BEFORE the delete — the data it needs stops existing once delete_account
// succeeds. cleanupOrphans (if set) runs only after a genuinely successful
// delete — never on validation failure, a resolve/delete error, or a "not
// found" result, since there's nothing to clean up yet.
func processDeleteCharacter(p deleteCharacterParams) error {
	if p.accountID == 0 {
		return fmt.Errorf("account ID required")
	}
	reason := strings.TrimSpace(p.reason)
	if reason == "" {
		return fmt.Errorf("reason required")
	}
	user, err := p.resolveUser(p.accountID)
	if err != nil {
		return fmt.Errorf("resolve account %d: %w", p.accountID, err)
	}
	if user == "" {
		return fmt.Errorf("account %d has no FLS user id", p.accountID)
	}
	if p.captureSnapshot != nil {
		if err := p.captureSnapshot(); err != nil {
			return fmt.Errorf("capture backup snapshot: %w", err)
		}
	}
	ok, err := p.deleteAccount(user, reason)
	if err != nil {
		return fmt.Errorf("delete account %d: %w", p.accountID, err)
	}
	if !ok {
		return fmt.Errorf("no character deleted for account %d", p.accountID)
	}
	if p.cleanupOrphans != nil {
		if err := p.cleanupOrphans(); err != nil {
			return fmt.Errorf("cleanup orphaned player_state: %w", err)
		}
	}
	return nil
}

// cmdDeleteCharacter permanently deletes a character via the game's native
// dune.delete_account proc, which deletes the player actors (with row locking),
// respawn beacons, and account row; writes an account_removal_log audit entry;
// fires guild/party/ownership cascades; and pg_notifies the live server.
//
// dune.delete_account never deletes the character's dune.player_state row
// (a view over dune.encrypted_player_state) — it survives with a dangling
// account_id and stale player_pawn_id/player_controller_id pointing at the
// now-deleted actors (#290). That orphan is what causes duplicate rows in
// the Players list (cmdFetchPlayers LEFT JOINs player_state on account_id;
// more than one row per account fans the join out) and give-items/teleport
// resolving against a deleted pawn actor after the player rejoins. Once
// delete_account succeeds, cleanupOrphans removes every player_state row
// whose account no longer exists at all — this can never touch a row
// belonging to a currently-valid account, and being unscoped (not filtered
// to just this account) it also retroactively cleans up any orphans left
// over from deletions before this fix shipped, on every future delete.
// Matches the DELETE-against-base-table style already used by
// seedGMIdentity for the same underlying schema issue (see its doc comment).
//
// When backup is true, captureSnapshot captures a full native character
// transfer backup (dune.character_transfer_export — see
// cmdCaptureCharacterBackup) before the delete runs. The export requires the
// player to be OFFLINE; a failure there (including "player must be offline")
// aborts the whole delete rather than proceeding without the safety net the
// admin asked for.
func cmdDeleteCharacter(pool *pgxpool.Pool, store *characterBackupsStore, accountID int64, reason, characterName string, backup bool) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		params := deleteCharacterParams{
			accountID: accountID,
			reason:    reason,
			resolveUser: func(id int64) (string, error) {
				return rawFuncomID(ctx, pool, id)
			},
			deleteAccount: func(user, reason string) (bool, error) {
				var deleted bool
				if err := pool.QueryRow(ctx,
					`SELECT dune.delete_account($1, $2)`, user, reason).Scan(&deleted); err != nil {
					return false, err
				}
				return deleted, nil
			},
			cleanupOrphans: func() error {
				_, err := pool.Exec(ctx, `
					DELETE FROM dune.encrypted_player_state
					WHERE NOT EXISTS (SELECT 1 FROM dune.accounts a WHERE a.id = encrypted_player_state.account_id)`)
				return err
			},
		}
		if backup {
			params.captureSnapshot = func() error {
				// store is the REQUEST-scoped backups store (see handleDeleteCharacter)
				// — never the unscoped global, or the safety backup's metadata row
				// lands under the wrong server_id and the restore path can't find it.
				return cmdCaptureCharacterBackup(ctx, pool, store, accountID, characterName, "delete_character", reason)
			}
		}
		err := processDeleteCharacter(params)
		if err != nil {
			return msgMutate{err: err}
		}
		invalidateAllJourneyCache()
		return msgMutate{ok: "Character deleted"}
	}
}

// ── character backup & restore (native transfer) ────────────────────────────
// Backs a character up via the game's own full-character transfer subsystem
// (dune.character_transfer_export / dune.character_transfer_import in
// db-routines/functions/transfer/) rather than a hand-rolled dump: the same
// ~50-table footprint (accounts, actors, inventories, items, fgl_entities,
// vehicles, base backups, progression, everything) the game uses for
// server-to-server character transfers, with local ids remapped to portable
// "transfer ids" on export and remapped back to fresh non-colliding ids on
// import. Export requires the player OFFLINE (the proc raises an exception
// otherwise); import additionally requires the game patch to match the
// exported '_patches_checksum' and is a full replace for that FLS id.

// captureCharacterBackupParams bundles the injectable dependencies for
// processCaptureCharacterBackup so orchestration can be unit-tested without a
// live DB or filesystem.
type captureCharacterBackupParams struct {
	accountID       int64
	characterName   string
	action          string
	reason          string
	resolveFLSID    func(accountID int64) (string, error)
	exportCharacter func(flsID string) (string, error)
	writeFile       func(contents string) (path string, err error)
	createRecord    func(b characterBackup) error
}

// characterTransferExportEnvelope is the subset of dune.character_transfer_export's
// jsonb result this package needs — just enough to record which game patch a
// backup was taken on, so a restore attempt against a later patch fails with
// a clear message instead of the game's raw checksum-mismatch error.
type characterTransferExportEnvelope struct {
	Checksum string `json:"_patches_checksum"`
}

// characterTransferChecksum extracts '_patches_checksum' from a
// character_transfer_export result. A missing field is not an error (returns
// "") — only malformed JSON is, since that means the export itself is
// unusable.
func characterTransferChecksum(data string) (string, error) {
	var env characterTransferExportEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return "", fmt.Errorf("invalid transfer export json: %w", err)
	}
	return env.Checksum, nil
}

// processCaptureCharacterBackup resolves the account's FLS id, exports the
// character via the native transfer proc, writes the result to a file, and
// records the backup's metadata — in that order, stopping at the first
// failure. A backup an admin can't trust (partial write, unrecorded) is worse
// than no backup at all.
func processCaptureCharacterBackup(p captureCharacterBackupParams) error {
	if p.accountID == 0 {
		return fmt.Errorf("account ID required")
	}
	flsID, err := p.resolveFLSID(p.accountID)
	if err != nil {
		return fmt.Errorf("resolve account %d: %w", p.accountID, err)
	}
	if flsID == "" {
		return fmt.Errorf("account %d has no FLS user id", p.accountID)
	}
	data, err := p.exportCharacter(flsID)
	if err != nil {
		return fmt.Errorf("export character: %w", err)
	}
	checksum, err := characterTransferChecksum(data)
	if err != nil {
		return fmt.Errorf("parse transfer export: %w", err)
	}
	path, err := p.writeFile(data)
	if err != nil {
		return fmt.Errorf("write backup file: %w", err)
	}
	if err := p.createRecord(characterBackup{
		AccountID:       p.accountID,
		FLSID:           flsID,
		CharacterName:   p.characterName,
		Action:          p.action,
		Reason:          p.reason,
		FilePath:        path,
		PatchesChecksum: checksum,
	}); err != nil {
		return fmt.Errorf("record backup metadata: %w", err)
	}
	return nil
}

// writeCharacterBackupFile writes a character_transfer_export result under
// the existing backups directory (dbBackupDir, db_backup.go — the same place
// full-database dumps live) in a character-transfers subdirectory, named for
// the account and capture time.
func writeCharacterBackupFile(accountID int64, contents string) (string, error) {
	baseDir, err := dbBackupDir()
	if err != nil {
		return "", fmt.Errorf("resolve backups dir: %w", err)
	}
	dir := filepath.Join(baseDir, "character-transfers")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create character-transfers dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d-%s.json", accountID, now().UTC().Format("20060102-150405")))
	// #nosec G304 -- path is built from the configured backups dir + an internal account id + timestamp, never request input
	if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}
	return path, nil
}

// cmdCaptureCharacterBackup is the production wiring for
// processCaptureCharacterBackup: resolves the FLS id via rawFuncomID,
// exports via dune.character_transfer_export, writes the file, and records
// it in store. A nil store means the metadata won't be discoverable via the
// backups list (mirrors initCharacterBackupsStore's "non-fatal" philosophy
// when the SQLite store failed to open) but the file itself is still
// written — not treated as a capture failure.
func cmdCaptureCharacterBackup(ctx context.Context, pool *pgxpool.Pool, store *characterBackupsStore, accountID int64, characterName, action, reason string) error {
	if pool == nil {
		return fmt.Errorf("not connected")
	}
	return processCaptureCharacterBackup(captureCharacterBackupParams{
		accountID:     accountID,
		characterName: characterName,
		action:        action,
		reason:        reason,
		resolveFLSID: func(id int64) (string, error) {
			return rawFuncomID(ctx, pool, id)
		},
		exportCharacter: func(flsID string) (string, error) {
			var result string
			err := pool.QueryRow(ctx, `SELECT dune.character_transfer_export($1)::text`, flsID).Scan(&result)
			return result, err
		},
		writeFile: func(contents string) (string, error) {
			return writeCharacterBackupFile(accountID, contents)
		},
		createRecord: func(b characterBackup) error {
			if store == nil {
				return nil
			}
			_, err := store.create(b)
			return err
		},
	})
}

// restoreCharacterBackupParams bundles the injectable dependencies for
// processRestoreCharacterBackup so orchestration can be unit-tested without a
// live DB or filesystem.
type restoreCharacterBackupParams struct {
	backupID  int64
	getBackup func(id int64) (*characterBackup, error)
	readFile  func(path string) (string, error)
	// resolveOldAccountID, if set, looks up the account currently holding
	// this FLS id BEFORE importCharacter runs — dune.character_transfer_import
	// calls dune.delete_account internally, which destroys that account, so
	// this must be resolved first or there's nothing left to look it up by.
	// Returns 0 (no error) when there's no current account for this FLS id
	// (e.g. restoring onto a never-before-seen id) — nothing to clean up.
	resolveOldAccountID func(flsID string) (int64, error)
	importCharacter     func(data, flsID, characterName string) (int64, error)
	// cleanupOrphans, if set, runs after a successful importCharacter — the
	// native dune.character_transfer_import proc calls dune.delete_account
	// internally to clear out the FLS id's current character before
	// reinserting the restored one (see cmdDeleteCharacter's doc comment for
	// what delete_account does and doesn't clean up: #290). That means every
	// restore reproduces #290's orphaned dune.player_state row for whatever
	// account previously held this fls_id — surfacing as a second, blank
	// entry in the players list — unless cleaned up here exactly like
	// cmdDeleteCharacter's cleanupOrphans already does for plain deletes.
	cleanupOrphans func() error
	// cleanupOrphanActors, if set, runs after a successful importCharacter
	// when resolveOldAccountID found a prior account — dune.delete_account's
	// own actor deletion has been observed to silently leave the old
	// controller/pawn/player-state actors behind (owner_account_id pointing
	// at the now-deleted account) even though it successfully deletes the
	// accounts/player_state rows. cmdFetchPlayers is rooted on dune.actors,
	// so a surviving PlayerCharacter-class actor here shows up as a second,
	// blank-named entry in the players list. Scoped to exactly the account
	// this restore replaced — never an unscoped actor sweep.
	cleanupOrphanActors func(oldAccountID int64) error
	invalidateCache     func()
}

// processRestoreCharacterBackup loads a backup record, reads its file, and
// restores it via the native transfer import proc — a full replace of the
// account's current character data for that FLS id. Returns the new
// player_controller_id the game assigned. resolveOldAccountID (if set) runs
// before the import, since the account it looks up is destroyed by it.
// cleanupOrphans and cleanupOrphanActors (if set) run only after a genuinely
// successful import; the cache is only invalidated after that — a failed
// restore hasn't changed anything worth invalidating for.
func processRestoreCharacterBackup(p restoreCharacterBackupParams) (int64, error) {
	b, err := p.getBackup(p.backupID)
	if err != nil {
		return 0, fmt.Errorf("load backup %d: %w", p.backupID, err)
	}
	data, err := p.readFile(b.FilePath)
	if err != nil {
		return 0, fmt.Errorf("read backup file: %w", err)
	}
	var oldAccountID int64
	if p.resolveOldAccountID != nil {
		oldAccountID, err = p.resolveOldAccountID(b.FLSID)
		if err != nil {
			return 0, fmt.Errorf("resolve current account for %s: %w", b.FLSID, err)
		}
	}
	newID, err := p.importCharacter(data, b.FLSID, b.CharacterName)
	if err != nil {
		return 0, fmt.Errorf("restore character: %w", err)
	}
	if p.cleanupOrphans != nil {
		if err := p.cleanupOrphans(); err != nil {
			return newID, fmt.Errorf("cleanup orphaned player_state: %w", err)
		}
	}
	if oldAccountID != 0 && p.cleanupOrphanActors != nil {
		if err := p.cleanupOrphanActors(oldAccountID); err != nil {
			return newID, fmt.Errorf("cleanup orphaned actors: %w", err)
		}
	}
	if p.invalidateCache != nil {
		p.invalidateCache()
	}
	return newID, nil
}

// cmdRestoreCharacterBackup is the production wiring for
// processRestoreCharacterBackup: loads the record from store, reads the file
// from disk, and restores via dune.character_transfer_import. Requires the
// player offline and the exported '_patches_checksum' to match the current
// game patch — the proc itself enforces both and its error messages surface
// through unchanged.
func cmdRestoreCharacterBackup(ctx context.Context, pool *pgxpool.Pool, store *characterBackupsStore, backupID int64) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("not connected")
	}
	if store == nil {
		return 0, fmt.Errorf("character backups store not available")
	}
	return processRestoreCharacterBackup(restoreCharacterBackupParams{
		backupID:  backupID,
		getBackup: store.get,
		readFile: func(path string) (string, error) {
			// #nosec G304 -- path comes from a stored backup record written by writeCharacterBackupFile, never request input directly
			data, err := os.ReadFile(path)
			return string(data), err
		},
		resolveOldAccountID: func(flsID string) (int64, error) {
			var id int64
			err := pool.QueryRow(ctx, `SELECT id FROM dune.accounts WHERE "user" = $1`, flsID).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, nil
			}
			return id, err
		},
		importCharacter: func(data, flsID, characterName string) (int64, error) {
			var newID int64
			err := pool.QueryRow(ctx, `SELECT dune.character_transfer_import($1::jsonb, $2, $3)`, data, flsID, characterName).Scan(&newID)
			return newID, err
		},
		cleanupOrphans: func() error {
			_, err := pool.Exec(ctx, `
				DELETE FROM dune.encrypted_player_state
				WHERE NOT EXISTS (SELECT 1 FROM dune.accounts a WHERE a.id = encrypted_player_state.account_id)`)
			return err
		},
		cleanupOrphanActors: func(oldAccountID int64) error {
			return cleanupOrphanActorsForAccount(ctx, pool, oldAccountID)
		},
		invalidateCache: invalidateAllJourneyCache,
	})
}

// orphanPlayerActorsDeleteSQL removes ONLY the leaked player actor trio
// (character pawn / controller / player-state) still owned by a destroyed
// account. The class scoping is a hard safety property, not an optimisation:
// dune.delete_account (and therefore character_transfer_import, which calls
// it) strips ownership/permission ranks from the player's bases, storage
// boxes, totems, and vehicles but deliberately leaves their actor rows alive
// — so after a restore, all of the old owner's property shares this same
// dangling owner_account_id. An unscoped delete keyed on owner_account_id
// alone would permanently destroy every one of those structures and their
// inventories. Never widen this predicate.
const orphanPlayerActorsDeleteSQL = `
	DELETE FROM dune.actors
	WHERE owner_account_id = $1
	  AND (class ILIKE '%PlayerCharacter%'
	    OR class ILIKE '%PlayerController%'
	    OR class ILIKE '%PlayerState%')`

// cleanupOrphanActorsForAccount deletes the player actor trio still pointing
// at oldAccountID after it's been destroyed (see cleanupOrphanActors' doc
// comment on restoreCharacterBackupParams). Mirrors dune.delete_account's own
// cascade: guild/party/ownership cleanup keyed on the controller actor, then
// the player actor rows themselves — scoped to the player actor classes only
// (see orphanPlayerActorsDeleteSQL for why that scoping is load-bearing).
func cleanupOrphanActorsForAccount(ctx context.Context, pool *pgxpool.Pool, oldAccountID int64) error {
	var controllerID int64
	err := pool.QueryRow(ctx, `
		SELECT id FROM dune.actors
		WHERE owner_account_id = $1 AND class ILIKE '%PlayerController%'
		LIMIT 1`, oldAccountID).Scan(&controllerID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("resolve surviving controller actor: %w", err)
	}
	if controllerID != 0 {
		if _, err := pool.Exec(ctx, `SELECT dune.guild_handle_actor_delete($1)`, controllerID); err != nil {
			return fmt.Errorf("guild cascade: %w", err)
		}
		if _, err := pool.Exec(ctx, `SELECT dune.remove_party_member($1, 0::SMALLINT)`, controllerID); err != nil {
			return fmt.Errorf("party cascade: %w", err)
		}
		if _, err := pool.Exec(ctx, `SELECT dune.ownership_handle_actor_delete($1)`, controllerID); err != nil {
			return fmt.Errorf("ownership cascade: %w", err)
		}
	}
	if _, err := pool.Exec(ctx, orphanPlayerActorsDeleteSQL, oldAccountID); err != nil {
		return fmt.Errorf("delete surviving player actors: %w", err)
	}
	return nil
}

func cmdGetPlayerTags(pool *pgxpool.Pool, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgTags{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		keyCol, keyVal, err := playerKeyFor(ctx, pool, "player_tags", accountID)
		if err != nil {
			return msgTags{err: err}
		}
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
		rows, err := pool.Query(ctx,
			fmt.Sprintf(`SELECT tag FROM dune.player_tags WHERE %s = $1 ORDER BY tag`, keyCol), keyVal)
		if err != nil {
			return msgTags{err: err}
		}
		defer rows.Close()
		var tags []string
		for rows.Next() {
			var tag string
			if err := rows.Scan(&tag); err != nil {
				continue
			}
			tags = append(tags, tag)
		}
		if err := rows.Err(); err != nil {
			return msgTags{err: err}
		}
		return msgTags{rows: tags}
	}
}

func cmdUpdatePlayerTags(pool *pgxpool.Pool, accountID int64, add []string, remove []string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		keyCol, keyVal, err := playerKeyFor(ctx, pool, "player_tags", accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		if err := upsertPlayerTags(ctx, pool, keyCol, keyVal, add, remove); err != nil {
			return msgMutate{err: fmt.Errorf("update player tags: %w", err)}
		}
		return msgMutate{ok: "Tags updated"}
	}
}

// rawFuncomID returns the accounts."user" value (hex Funcom ID) for a given
// account_id. This is the ID format expected by character_transfer_export,
// complete_journey_story_nodes_for_player, update_returning_player_status,
// delete_account, and other procs — distinct from encrypted_funcom_id which
// stores the human-readable display name (e.g. "Icehunter#55381").
func rawFuncomID(ctx context.Context, pool *pgxpool.Pool, accountID int64) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `SELECT "user" FROM dune.accounts WHERE id = $1`, accountID).Scan(&id)
	return id, err
}

// Seeded "GM/Server" chat persona sentinels. The account id is held in a high,
// out-of-range slot so it never collides with a real player (Phase 0 recon: real
// accounts max out near id 2, actors near 54) and so operator-facing queries can
// exclude it. The actor ids derive from the account id. Single source of truth;
// the seed routine and the blast-radius exclusions both key off these.
const (
	gmIdentityAccountID     int64  = 9000001
	gmIdentityHexID         string = "DA5EBA11DA5EBA11" // accounts."user" (AMQP user_id)
	gmIdentityFuncomID      string = "GM#0001"          // chat id (m_FuncomIdFrom)
	gmIdentityCharacterName string = "GM"               // in-game display name
)

// errGMNotProvisioned signals that the GM/Server chat persona has not been seeded
// yet, so admin chat cannot resolve a sender identity. Mapped to 503 by handlers.
var errGMNotProvisioned = errors.New("gm identity not provisioned")

// cmdGetGMIdentity reads the seeded GM/Server persona used as the sender for admin
// chat. Returns its hex FLS id (the AMQP user_id) and funcom id (m_FuncomIdFrom).
// Reads the dune.accounts VIEW, which decrypts funcom_id from the encrypted base
// table — so this stays correct even if user-data encryption is enabled. Returns
// errGMNotProvisioned if the identity row does not exist yet.
func cmdGetGMIdentity(ctx context.Context) (gmIdentity, error) {
	gm := gmIdentity{AccountID: gmIdentityAccountID}
	err := globalDB.QueryRow(ctx, `
		SELECT "user", funcom_id
		FROM dune.accounts
		WHERE id = $1`, gmIdentityAccountID).Scan(&gm.HexID, &gm.FuncomID)
	if errors.Is(err, pgx.ErrNoRows) {
		return gmIdentity{}, errGMNotProvisioned
	}
	if err != nil {
		return gmIdentity{}, fmt.Errorf("read gm identity: %w", err)
	}
	return gm, nil
}

// cmdResolveRecipientChatIdentity resolves a whisper recipient by account id into
// the values the whisper wire body needs: funcom id (m_SubChannelId + AMQP routing
// key) and character name (m_UserNameTo). Reads the decrypting accounts/player_state
// views so it is correct regardless of the user-data encryption setting.
func cmdResolveRecipientChatIdentity(ctx context.Context, accountID int64) (funcomID, charName string, err error) {
	err = globalDB.QueryRow(ctx, `
		SELECT a.funcom_id, COALESCE(ps.character_name, '')
		FROM dune.accounts a
		LEFT JOIN dune.player_state ps ON ps.account_id = a.id
		WHERE a.id = $1`, accountID).Scan(&funcomID, &charName)
	if err != nil {
		return "", "", fmt.Errorf("resolve recipient %d: %w", accountID, err)
	}
	return funcomID, charName, nil
}

// gmSeed holds the fixed values written for the GM/Server persona. Centralised so
// the sentinel ids, the actor class paths (which MUST match the live schema or the
// game's player-info lookup fails), and the blast-radius-safe defaults are testable
// and have one source of truth. Class paths + partition are from Phase 0 recon.
type gmSeed struct {
	AccountID       int64
	HexID           string
	FuncomID        string
	CharacterName   string
	ControllerID    int64
	StateID         int64
	PawnID          int64
	ControllerClass string
	StateClass      string
	PawnClass       string
	Map             string
	PartitionID     int64
	DimensionIndex  int
	OnlineStatus    string
	LifeState       string
}

func gmSeedSpec() gmSeed {
	return gmSeed{
		AccountID:       gmIdentityAccountID,
		HexID:           gmIdentityHexID,
		FuncomID:        gmIdentityFuncomID,
		CharacterName:   gmIdentityCharacterName,
		ControllerID:    gmIdentityAccountID*100 + 1, // 900000101
		StateID:         gmIdentityAccountID*100 + 2, // 900000102
		PawnID:          gmIdentityAccountID*100 + 3, // 900000103
		ControllerClass: "/Game/Dune/Characters/Player/BP_DunePlayerController.BP_DunePlayerController_C",
		StateClass:      "/Script/DuneSandbox.DunePlayerState",
		PawnClass:       "/Game/Dune/Characters/Player/BP_DunePlayerCharacter.BP_DunePlayerCharacter_C",
		Map:             "HaggaBasin",
		PartitionID:     1,
		DimensionIndex:  0,
		OnlineStatus:    "Offline", // blast-radius safe; flip to Online only if verify needs it
		LifeState:       "Alive",
	}
}

// seedGMIdentity executes the GM persona seed writes against db (a transaction or
// pool). Extracted for testability via the pgExecutor interface. Called by
// cmdEnsureGMIdentity inside a transaction.
//
// Player-state deduplication: the game's schema migration (post-1.5) removed the
// unique constraint on dune.encrypted_player_state.account_id, so the old bare
// INSERT … ON CONFLICT DO NOTHING no longer deduplicates — a new GM row was
// created on every dune-admin startup, and the director crashed when it saw two
// player_state rows for the same fls_id. The fix:
//
//  1. DELETE any duplicate GM player_state rows, keeping only the lowest-id one
//     (self-heals servers that already accumulated duplicates).
//  2. INSERT the GM row only when none exists (WHERE NOT EXISTS guard) so a second
//     row can never be created even if the constraint is absent.
//
// The accounts and actors inserts keep ON CONFLICT DO NOTHING — they use explicit
// synthetic ids as the PK, so the conflict target is still valid.
func seedGMIdentity(ctx context.Context, db pgExecutor, s gmSeed) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO dune.encrypted_accounts (id, "user", encrypted_funcom_id, takeoverable, platform_id, platform_name)
		VALUES ($1, $2, dune.encrypt_user_data($3), false, 'dune-admin', 'DuneAdmin')
		ON CONFLICT DO NOTHING`, s.AccountID, s.HexID, s.FuncomID); err != nil {
		return fmt.Errorf("seed gm account: %w", err)
	}

	actors := []struct {
		id    int64
		class string
	}{
		{s.ControllerID, s.ControllerClass},
		{s.StateID, s.StateClass},
		{s.PawnID, s.PawnClass},
	}
	for _, a := range actors {
		if _, err := db.Exec(ctx, `
			INSERT INTO dune.actors (id, class, map, partition_id, dimension_index, gas_attributes, properties, owner_account_id, serial)
			VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, '{}'::jsonb, $6, 1)
			ON CONFLICT DO NOTHING`,
			a.id, a.class, s.Map, s.PartitionID, s.DimensionIndex, s.AccountID); err != nil {
			return fmt.Errorf("seed gm actor %d: %w", a.id, err)
		}
	}

	// Step 1 — collapse any existing duplicate GM player_state rows to the
	// canonical lowest-id one. Safe with zero rows: id <> (SELECT MIN(id) …)
	// returns NULL when the subquery has no rows, so the WHERE never matches.
	if _, err := db.Exec(ctx, `
		DELETE FROM dune.encrypted_player_state
		WHERE account_id = $1
		  AND id <> (SELECT MIN(id) FROM dune.encrypted_player_state WHERE account_id = $1)`,
		s.AccountID); err != nil {
		return fmt.Errorf("dedupe gm player_state: %w", err)
	}

	// Step 2 — insert the GM row only if it doesn't already exist. Using
	// INSERT … SELECT … WHERE NOT EXISTS instead of ON CONFLICT DO NOTHING
	// because the table no longer has a unique constraint on account_id.
	// server_id reuses a real one if any player has logged in (the game's
	// lookup expects a valid server); NULL is fine on a never-populated DB.
	if _, err := db.Exec(ctx, `
		INSERT INTO dune.encrypted_player_state
			(account_id, encrypted_character_name, life_state, online_status, is_coriolis_processed,
			 server_id, player_controller_id, player_pawn_id, player_state_id, last_login_time)
		SELECT $1, dune.encrypt_user_data($2), $3::dune.playerlifestate, $4::dune.playerconnectionstatus, false,
			(SELECT server_id FROM dune.encrypted_player_state WHERE server_id IS NOT NULL LIMIT 1),
			$5, $6, $7, now()
		WHERE NOT EXISTS (
			SELECT 1 FROM dune.encrypted_player_state WHERE account_id = $1
		)`,
		s.AccountID, s.CharacterName, s.LifeState, s.OnlineStatus, s.ControllerID, s.PawnID, s.StateID); err != nil {
		return fmt.Errorf("seed gm player_state: %w", err)
	}

	return nil
}

// cmdEnsureGMIdentity idempotently seeds the GM/Server persona used as the sender
// for admin chat. It writes the BASE tables (dune.accounts / dune.player_state are
// VIEWS over them) plus the three linked actor rows the game's player-info lookup
// requires. Names go through dune.encrypt_user_data so the seed stays correct if
// user-data encryption is ever enabled. actors.transform is left NULL so the GM
// never plots on the live map. Self-heals duplicate player_state rows (see
// seedGMIdentity). Safe to call on every startup; connectAll logs-and-continues.
func cmdEnsureGMIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("not connected")
	}
	s := gmSeedSpec()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin gm seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := seedGMIdentity(ctx, tx, s); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit gm seed: %w", err)
	}
	return nil
}

func cmdGrantReturningPlayerAward(pool *pgxpool.Pool, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		rawID, err := rawFuncomID(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up funcom id: %w", err)}
		}
		_, err = pool.Exec(ctx, `
			UPDATE dune.encrypted_player_state
			SET last_returning_player_awarded_time = NULL,
			    last_returning_player_event_time = NULL
			WHERE account_id = $1`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset returning player timestamps: %w", err)}
		}
		_, err = pool.Exec(ctx, `SELECT dune.update_returning_player_status($1, 0)`, rawID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update_returning_player_status: %w", err)}
		}
		return msgMutate{ok: "Returning player award reset — will trigger on next login"}
	}
}

func cmdDismissReturningPlayerAward(pool *pgxpool.Pool, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		_, err := pool.Exec(ctx, `
			UPDATE dune.encrypted_player_state
			SET last_returning_player_awarded_time = NOW(),
			    last_returning_player_event_time = NOW()
			WHERE account_id = $1`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("dismiss returning player award: %w", err)}
		}
		return msgMutate{ok: "Returning player popup dismissed"}
	}
}

func cmdDeleteAccount(pool *pgxpool.Pool, accountID int64, reason string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		rawID, err := rawFuncomID(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("look up funcom id: %w", err)}
		}
		var result bool
		err = pool.QueryRow(ctx, `SELECT dune.delete_account($1, $2)`, rawID, reason).Scan(&result)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete account: %w", err)}
		}
		return msgMutate{ok: "Account deleted"}
	}
}

func cmdDeleteItem(pool *pgxpool.Pool, itemID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if itemID == 0 {
			return msgMutate{err: fmt.Errorf("item ID required")}
		}
		_, err := pool.Exec(context.Background(), `SELECT dune.delete_item($1::bigint)`, itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete item: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Deleted item %d — relog to see in-game", itemID)}
	}
}

func cmdResetSpecializations(pool *pgxpool.Pool, playerID int64, trackType string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()

		if trackType == "" || strings.EqualFold(trackType, "all") {
			if _, err := pool.Exec(ctx, `SELECT dune.reset_specialization_tracks($1)`, playerID); err != nil {
				return msgMutate{err: fmt.Errorf("reset tracks: %w", err)}
			}
			if _, err := pool.Exec(ctx, `SELECT dune.reset_specialization_keystones($1)`, playerID); err != nil {
				return msgMutate{err: fmt.Errorf("reset keystones: %w", err)}
			}
			return msgMutate{ok: fmt.Sprintf("Reset all spec tracks + keystones for player %d", playerID)}
		}

		res, err := pool.Exec(ctx, `
			DELETE FROM dune.specialization_tracks
			WHERE player_id = $1::bigint AND track_type::text = $2::text`, playerID, trackType)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset track: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf(
			"Reset %s track for player %d (%d row(s) cleared)", trackType, playerID, res.RowsAffected())}
	}
}

// onlineStateRow holds a single row from the player online state query.
type onlineStateRow struct {
	PlayerID int64
	Name     string
	Map      string
	Status   string
	LastSeen string
}

type msgOnlineState struct {
	rows []onlineStateRow
	err  error
}

// onlineStateSQL reads dune.player_state directly, so it applies the #290
// canonical-row rules itself: DISTINCT ON (account) picks the
// most-recently-active row when an account has duplicates, and the accounts
// EXISTS filter keeps orphaned state rows (account already deleted) from
// showing up as phantom players in the activity view.
const onlineStateSQL = `
	SELECT sub.player_controller_id, sub.character_name, sub.map, sub.status, sub.last_seen
	FROM (
		SELECT DISTINCT ON (ps.account_id)
		       ps.player_controller_id,
		       COALESCE(ps.character_name, '') AS character_name,
		       COALESCE(a.map, '') AS map,
		       ps.online_status::text AS status,
		       COALESCE(to_char(ps.last_avatar_activity AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '') AS last_seen,
		       ps.last_avatar_activity
		FROM dune.player_state ps
		LEFT JOIN dune.actors a ON a.id = ps.player_controller_id
		WHERE ps.account_id <> $1
		  AND EXISTS (SELECT 1 FROM dune.accounts ac WHERE ac.id = ps.account_id)
		ORDER BY ps.account_id, ps.last_login_time DESC NULLS LAST, ps.id DESC
	) sub
	ORDER BY sub.status DESC, sub.last_avatar_activity DESC`

func cmdFetchOnlineState(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgOnlineState{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), onlineStateSQL, gmIdentityAccountID)
	if err != nil {
		return msgOnlineState{err: err}
	}
	defer rows.Close()

	var out []onlineStateRow
	for rows.Next() {
		var r onlineStateRow
		if err := rows.Scan(&r.PlayerID, &r.Name, &r.Map, &r.Status, &r.LastSeen); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgOnlineState{err: err}
	}
	return msgOnlineState{rows: out}
}

// ── private helpers ───────────────────────────────────────────────────────────

// resolveStackMax returns the per-slot stack cap for a template and whether
// that value is actually known. known=false means we had to fall back to a
// guess (no item-data rule, no existing stacks of this template, no configured
// default). Callers must treat an unknown cap as "stacks freely" rather than
// "one per slot" — otherwise stackables we lack data for (e.g. Ammo) get
// counted as one inventory slot per unit. See effectiveStackMax.
func resolveStackMax(ctx context.Context, pool *pgxpool.Pool, template string, quality int64) (stackMax int64, known bool, err error) {
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok && rule.StackMax > 0 {
			return rule.StackMax, true, nil
		}
	}
	var maxStack int64
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(stack_size), 0)
		FROM dune.items
		WHERE template_id = $1::text AND quality_level = $2::bigint`, template, quality).Scan(&maxStack); err != nil {
		return 0, false, err
	}
	if maxStack > 0 {
		return maxStack, true, nil
	}
	if itemData.DefaultStackMax > 0 {
		return itemData.DefaultStackMax, true, nil
	}
	return 1, false, nil
}

// effectiveStackMax picks the stack size to assume for slot/stack planning.
// When the real cap is unknown (or nonsensical), items are assumed to stack
// into the requested quantity — i.e. one slot — instead of one slot per unit.
// This keeps qty=1 grants unchanged while preventing unknown stackables like
// ammo from being rejected as needing thousands of free slots.
func effectiveStackMax(stackMax int64, known bool, qty int64) int64 {
	if !known || stackMax < 1 {
		return qty
	}
	return stackMax
}

func resolveItemVolume(ctx context.Context, pool *pgxpool.Pool, template string) (float64, error) {
	if itemData.Items != nil {
		if rule, ok := itemData.Items[strings.ToLower(template)]; ok {
			// volume=0 is valid (item takes no inventory space).
			return rule.Volume, nil
		}
	}
	var vol pgtype.Float8
	err := pool.QueryRow(ctx, `
		SELECT MAX(volume_override)
		FROM dune.items
		WHERE template_id = $1::text AND volume_override IS NOT NULL`, template).Scan(&vol)
	if err != nil {
		return 0, err
	}
	if vol.Valid && vol.Float64 > 0 {
		return vol.Float64, nil
	}
	if itemData.DefaultVolume > 0 {
		return itemData.DefaultVolume, nil
	}
	return 0, nil // unknown volume — treat as zero (no space consumed)
}

func formatCurrencyIDs(ids []int16) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ", ")
}

func resolveScripCurrencyID(ctx context.Context, pool *pgxpool.Pool) (int16, error) {
	if scripCurrencyID >= 0 {
		return int16(scripCurrencyID), nil
	}
	rows, err := pool.Query(ctx, `
		SELECT currency_id, COALESCE(SUM(balance), 0) AS total
		FROM dune.player_virtual_currency_balances
		WHERE currency_id <> dune.get_solaris_id()
		GROUP BY currency_id
		ORDER BY total DESC, currency_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int16
	for rows.Next() {
		var id int16
		var total int64
		if err := rows.Scan(&id, &total); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("no non-solaris currency rows found; pass -scripcurrency")
	}
	return 0, fmt.Errorf("multiple non-solaris currency IDs found (%s); pass -scripcurrency", formatCurrencyIDs(ids))
}

// factionDataEntry mirrors one element of
// actors.properties.FactionPlayerComponent.m_FactionDataArray — the cache the
// in-game faction UI reads for rank display and per-territory vendor gating.
// Shape verified live: {"Faction":{"Name":...},"timestamp":<float>,"ReputationAmount":<int>}.
type factionDataEntry struct {
	Faction          factionDataName `json:"Faction"`
	Timestamp        float64         `json:"timestamp"`
	ReputationAmount int32           `json:"ReputationAmount"`
}

type factionDataName struct {
	Name string `json:"Name"`
}

// greatHouseFactions are the two houses the in-game faction rank/vendor system
// tracks. m_FactionDataArray always lists both so each territory's vendor reads
// its own house's standing (a missing house reads as 0). Verified in-game:
// Arrakeen reads the Atreides entry, Harko Village reads the Harkonnen one.
var greatHouseFactions = []struct {
	id   int16
	name string
}{
	{1, "Atreides"},
	{2, "Harkonnen"},
}

// buildFactionDataArray produces the canonical m_FactionDataArray from the
// player's per-faction reputation. It always emits both great houses (missing
// → 0) and ignores non-great-house factions (None=3, Smuggler=4).
func buildFactionDataArray(reps map[int16]int32, ts float64) []factionDataEntry {
	out := make([]factionDataEntry, 0, len(greatHouseFactions))
	for _, h := range greatHouseFactions {
		out = append(out, factionDataEntry{
			Faction:          factionDataName{Name: h.name},
			Timestamp:        ts,
			ReputationAmount: reps[h.id],
		})
	}
	return out
}

// factionComponentExecer is the minimal surface writeFactionComponent needs —
// satisfied by *pgxpool.Pool and pgx.Tx (and stubbed in tests).
type factionComponentExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// factionComponentDB adds row reads for syncFactionComponent.
type factionComponentDB interface {
	factionComponentExecer
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// factionComponentRebuildSQL replaces m_FactionDataArray wholesale. The previous
// per-element update no-oped silently when the array was empty (the state of
// every fresh character) — the bug that made faction unlocks display as rank
// "Outsider" in-game despite correct rep/alignment/tags in the DB.
const factionComponentRebuildSQL = `
	UPDATE dune.actors
	SET properties = jsonb_set(
		properties, '{FactionPlayerComponent,m_FactionDataArray}', $1::jsonb, true)
	WHERE id = $2`

// writeFactionComponent rebuilds the controller actor's m_FactionDataArray.
// Returns an error if no row was updated — turning the old silent no-op into a
// loud failure so a misleading success can never be reported again.
func writeFactionComponent(ctx context.Context, exec factionComponentExecer, controllerID int64, arr []factionDataEntry) error {
	payload, err := json.Marshal(arr)
	if err != nil {
		return fmt.Errorf("marshal faction data array: %w", err)
	}
	tag, err := exec.Exec(ctx, factionComponentRebuildSQL, payload, controllerID)
	if err != nil {
		return fmt.Errorf("rebuild FactionPlayerComponent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("controller actor %d not found; FactionPlayerComponent not updated", controllerID)
	}
	return nil
}

// syncFactionComponent reads the controller's current great-house reputation and
// rebuilds m_FactionDataArray to match. Call after any
// set_player_faction_reputation so the in-game rank/vendor UI reflects the DB.
// Accepts *pgxpool.Pool or a pgx.Tx.
func syncFactionComponent(ctx context.Context, db factionComponentDB, controllerID int64) error {
	rows, err := db.Query(ctx, `
		SELECT faction_id, reputation_amount
		FROM dune.player_faction_reputation
		WHERE actor_id = $1 AND faction_id IN (1, 2)`, controllerID)
	if err != nil {
		return fmt.Errorf("read faction reputation: %w", err)
	}
	defer rows.Close()

	reps := make(map[int16]int32, 2)
	for rows.Next() {
		var fid int16
		var rep int32
		if err := rows.Scan(&fid, &rep); err != nil {
			return fmt.Errorf("scan faction reputation: %w", err)
		}
		reps[fid] = rep
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate faction reputation: %w", err)
	}

	arr := buildFactionDataArray(reps, float64(time.Now().UnixNano())/1e9)
	return writeFactionComponent(ctx, db, controllerID, arr)
}

func applyFactionRepDelta(ctx context.Context, pool *pgxpool.Pool, actorID int64, factionID int16, delta int32) msgMutate {
	// Route through set_player_faction_reputation which handles tier tags correctly.
	// First get current rep to compute the new absolute value.
	var currentRep int32
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(reputation_amount, 0) FROM dune.player_faction_reputation
		WHERE actor_id = $1::bigint AND faction_id = $2::smallint`, actorID, factionID).Scan(&currentRep)

	newRep := max(currentRep+delta, 0)
	newRep = min(newRep, factionRepCap)

	_, err := pool.Exec(ctx, `
		SELECT dune.set_player_faction_reputation($1::bigint, $2::smallint, $3::integer)`,
		actorID, factionID, newRep)
	if err != nil {
		return msgMutate{err: fmt.Errorf("set_player_faction_reputation: %w", err)}
	}
	if err = syncFactionComponent(ctx, pool, actorID); err != nil {
		return msgMutate{err: fmt.Errorf("update FactionPlayerComponent rep: %w", err)}
	}

	tier := repToTier(newRep)
	fName := factionDisplayName(factionID)
	return msgMutate{ok: fmt.Sprintf(
		"Set %s rep to %d → tier %d (%s) for actor %d",
		fName, newRep, tier, factionTierName(factionID, tier), actorID)}
}

// factionRepCap is the maximum reputation for any faction (tier 20).
const factionRepCap = int32(12474)

// factionTierThresholds[i] = cumulative rep required to reach tier i (0–20).
// Both Atreides and Harkonnen share identical thresholds.
var factionTierThresholds = [21]int32{
	0, 99, 249, 499, 999, 1999, 2224, 2524, 2899, 3349, 3874,
	4474, 5149, 5899, 6724, 7624, 8599, 9649, 10774, 11974, 12474,
}

// repToTier returns the tier (0–20) for a given reputation amount.
func repToTier(rep int32) int {
	tier := 0
	for i := 1; i <= 20; i++ {
		if rep >= factionTierThresholds[i] {
			tier = i
		} else {
			break
		}
	}
	return tier
}

// factionTierName returns the named tier string for a faction+tier combination.
func factionTierName(factionID int16, tier int) string {
	named := map[int]string{
		0: "Outsider", 1: "Mercenary", 2: "Recruit", 3: "Contractor",
		4: "Agent", 5: "House Operator",
	}
	if tier20 := map[int16]string{1: "Envoy", 2: "Enforcer"}; tier == 20 {
		if n, ok := tier20[factionID]; ok {
			return n
		}
	}
	if n, ok := named[tier]; ok {
		return n
	}
	return fmt.Sprintf("Tier %d", tier)
}

func factionDisplayName(id int16) string {
	switch id {
	case 1:
		return "Atreides"
	case 2:
		return "Harkonnen"
	case 3:
		return "None"
	case 4:
		return "Smuggler"
	default:
		return fmt.Sprintf("Faction%d", id)
	}
}

func cmdSetFactionTier(pool *pgxpool.Pool, actorID int64, factionID int16, tier int) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if tier < 0 || tier > 20 {
			return msgMutate{err: fmt.Errorf("tier must be 0–20")}
		}
		// Nudge +1 over the threshold — the game UI floors at the threshold
		// (rep == threshold shows the tier below), except at tier 0 where 0 is
		// the legitimate minimum.
		rep := factionTierThresholds[tier]
		if tier > 0 {
			rep++
		}
		ctx := context.Background()
		// Align the player to this house first (Gap 1): set-tier on an unaligned
		// character previously wrote rep with no player_faction row, so the game
		// treated them as unaligned. change_player_faction upserts alignment and
		// fires pg_notify('faction_notify_channel'). neutral_faction_id = 3 ("None").
		if _, err := pool.Exec(ctx,
			`SELECT dune.change_player_faction($1::bigint, $2::smallint, 3::smallint, NOW()::timestamp)`,
			actorID, factionID); err != nil {
			return msgMutate{err: fmt.Errorf("change_player_faction: %w", err)}
		}
		if _, err := pool.Exec(ctx, `SELECT dune.set_player_faction_reputation($1, $2, $3)`,
			actorID, factionID, rep); err != nil {
			return msgMutate{err: fmt.Errorf("set_player_faction_reputation: %w", err)}
		}
		if err := syncFactionComponent(ctx, pool, actorID); err != nil {
			return msgMutate{err: fmt.Errorf("update FactionPlayerComponent rep: %w", err)}
		}
		fName := factionDisplayName(factionID)
		return msgMutate{ok: fmt.Sprintf(
			"Set %s to tier %d (%s) — rep %d for actor %d",
			fName, tier, factionTierName(factionID, tier), rep, actorID)}
	}
}

func cmdFetchItemTemplates() Msg {
	if globalDB == nil {
		return msgItemTemplates{}
	}
	rows, err := globalDB.Query(context.Background(),
		`SELECT DISTINCT template_id FROM dune.items ORDER BY template_id`)
	if err != nil {
		return msgItemTemplates{}
	}
	defer rows.Close()
	var templates []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			templates = append(templates, t)
		}
	}
	return msgItemTemplates{templates: templates}
}

// ── database tab types and fetch functions ────────────────────────────────────

type tableRow struct {
	Name     string
	RowCount int64
}

type columnInfo struct {
	Name     string
	DataType string
	Nullable string
}

type msgTables struct {
	rows []tableRow
	err  error
}

type msgDescribe struct {
	table string
	cols  []columnInfo
	err   error
}

type msgSample struct {
	table   string
	headers []string
	rows    [][]string
	err     error
}

type msgSearchCols struct {
	headers []string
	rows    [][]string
	err     error
}

func cmdFetchTables(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgTables{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT relname, COALESCE(n_live_tup, 0)
		FROM pg_stat_user_tables
		ORDER BY relname`)
	if err != nil {
		return msgTables{err: err}
	}
	defer rows.Close()
	var result []tableRow
	for rows.Next() {
		var r tableRow
		if err := rows.Scan(&r.Name, &r.RowCount); err != nil {
			return msgTables{err: err}
		}
		result = append(result, r)
	}
	return msgTables{rows: result}
}

func cmdDescribeTable(pool *pgxpool.Pool, tbl string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgDescribe{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT column_name, data_type,
			       CASE is_nullable WHEN 'YES' THEN 'null' ELSE 'not null' END
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1::text
			ORDER BY ordinal_position`, tbl)
		if err != nil {
			return msgDescribe{table: tbl, err: err}
		}
		defer rows.Close()
		var cols []columnInfo
		for rows.Next() {
			var c columnInfo
			if err := rows.Scan(&c.Name, &c.DataType, &c.Nullable); err != nil {
				return msgDescribe{table: tbl, err: err}
			}
			cols = append(cols, c)
		}
		if err := rows.Err(); err != nil {
			return msgDescribe{table: tbl, err: err}
		}
		return msgDescribe{table: tbl, cols: cols}
	}
}

func sampleTableQuery(tbl string, limit int) string {
	// Sanitize table name defensively even though tbl comes from pg_stat_user_tables.
	// pgx.Identifier handles quoting and escaping to prevent SQL injection.
	//
	// Deliberately a bare (unqualified) identifier: it resolves through the
	// connection's search_path, not the package-level dbSchema global, which
	// is empty at runtime after the multi-server refactor (#283). Qualifying
	// with the empty global used to produce `FROM "".tbl`, a Postgres
	// "zero-length delimited identifier" error (SQLSTATE 42601).
	safeTable := pgx.Identifier{tbl}.Sanitize()
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", safeTable, limit)
}

func sampleTableHeaders(rows pgx.Rows) []string {
	descriptions := rows.FieldDescriptions()
	headers := make([]string, 0, len(descriptions))
	for _, description := range descriptions {
		headers = append(headers, description.Name)
	}
	return headers
}

func formatSampleRow(values []any) []string {
	row := make([]string, 0, len(values))
	for _, value := range values {
		row = append(row, fmt.Sprintf("%v", value))
	}
	return row
}

func sampleTableRows(rows pgx.Rows) ([][]string, error) {
	result := make([][]string, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		result = append(result, formatSampleRow(values))
	}
	return result, nil
}

func cmdSampleTable(pool *pgxpool.Pool, tbl string, limit int) Cmd {
	return func() Msg {
		if pool == nil {
			return msgSample{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), sampleTableQuery(tbl, limit))
		if err != nil {
			return msgSample{table: tbl, err: err}
		}
		defer rows.Close()

		headers := sampleTableHeaders(rows)
		result, err := sampleTableRows(rows)
		if err != nil {
			return msgSample{table: tbl, err: err}
		}
		if err := rows.Err(); err != nil {
			return msgSample{table: tbl, err: err}
		}
		return msgSample{table: tbl, headers: headers, rows: result}
	}
}

func cmdSearchColumns(pool *pgxpool.Pool, term string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgSearchCols{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT table_name, column_name, data_type
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND (column_name ILIKE $1::text OR table_name ILIKE $1::text)
			ORDER BY table_name, column_name`, "%"+term+"%")
		if err != nil {
			return msgSearchCols{err: err}
		}
		defer rows.Close()
		headers := []string{"table", "column", "type"}
		var result [][]string
		for rows.Next() {
			var table, col, dtype string
			if err := rows.Scan(&table, &col, &dtype); err != nil {
				return msgSearchCols{err: err}
			}
			result = append(result, []string{table, col, dtype})
		}
		if err := rows.Err(); err != nil {
			return msgSearchCols{err: err}
		}
		return msgSearchCols{headers: headers, rows: result}
	}
}

// ── per-character table key (issue #267) ─────────────────────────────────────
//
// Current Dune servers migrated several per-player tables (journey_story_node,
// player_tags) from an account_id key to a character_id key, where character_id
// is dune.encrypted_player_state.id — a per-character surrogate reached from the
// account id (1:1 for real players; journey_story_node.character_id is a foreign
// key to it). dune-admin still addresses these tables by account, so we resolve
// the character id at runtime and fall back to account_id on un-migrated servers.

const (
	playerKeyCharacterID = "character_id" // current schema
	playerKeyAccountID   = "account_id"   // legacy schema
)

// pgExecutor is the subset of pgxpool.Pool / pgx.Tx used by the schema-aware
// player-table writers, so they work inside a transaction or directly.
type pgExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	playerKeyColMu    sync.RWMutex
	playerKeyColCache = map[string]string{} // dune table name -> key column
)

// playerKeyColumnFromProbe maps "does this table have a character_id column?"
// to the key column to use. Pure, for testability.
func playerKeyColumnFromProbe(hasCharacterID bool) string {
	if hasCharacterID {
		return playerKeyCharacterID
	}
	return playerKeyAccountID
}

// selectPlayerKey picks the (column, value) addressing a per-character table,
// given the live key column and the resolved character/account ids. Pure, so
// the account_id -> character_id branching is unit-testable without a database.
func selectPlayerKey(liveColumn string, characterID, accountID int64) (string, int64) {
	if liveColumn == playerKeyCharacterID {
		return playerKeyCharacterID, characterID
	}
	return playerKeyAccountID, accountID
}

// playerKeyColumn reports which key column dune.<table> uses on the connected
// server: "character_id" on current servers, "account_id" on legacy ones.
// Probed once per table via information_schema and cached. On probe error it
// assumes the current schema and does not cache, so a later call can re-probe.
func playerKeyColumn(ctx context.Context, pool *pgxpool.Pool, table string) string {
	playerKeyColMu.RLock()
	col, ok := playerKeyColCache[table]
	playerKeyColMu.RUnlock()
	if ok {
		return col
	}

	// Resolve the table through to_regclass so the probe honours the
	// connection's search_path (set from the active server's schema) instead of
	// the package-level dbSchema, which is empty until a config save. This
	// matches how the data queries resolve dune.<table>.
	var hasCharacterID bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_attribute
			WHERE attrelid = to_regclass($1::text)
			  AND attname  = 'character_id'
			  AND NOT attisdropped
		)`, table).Scan(&hasCharacterID); err != nil {
		// Probe failed — assume the current schema rather than silently
		// reintroducing the account_id bug; don't cache the guess.
		return playerKeyCharacterID
	}

	col = playerKeyColumnFromProbe(hasCharacterID)
	playerKeyColMu.Lock()
	playerKeyColCache[table] = col
	playerKeyColMu.Unlock()
	return col
}

// resolvePlayerCharacterID maps an account id to its character id
// (dune.encrypted_player_state.id) — the key the migrated per-character tables
// use. journey_story_node.character_id is a foreign key to this column.
// resolvePlayerCharacterIDSQL picks the character id under the SAME canonical
// definition as playerStateCanonicalJoinOn (#290): most-recently-active. It
// previously ordered by oldest id, which for an account with duplicate state
// rows resolved a DIFFERENT row than the players list showed — tag/journey
// edits silently targeted the stale duplicate. The game keys its own runtime
// writes (journey, tags) to the row it loaded at login, i.e. the live one.
const resolvePlayerCharacterIDSQL = `
	SELECT id FROM dune.encrypted_player_state WHERE account_id = $1
	ORDER BY last_login_time DESC NULLS LAST, id DESC
	LIMIT 1`

func resolvePlayerCharacterID(ctx context.Context, pool *pgxpool.Pool, accountID int64) (int64, error) {
	var characterID int64
	err := pool.QueryRow(ctx, resolvePlayerCharacterIDSQL, accountID).Scan(&characterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("no character found for account %d", accountID)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve character for account %d: %w", accountID, err)
	}
	return characterID, nil
}

// playerKeyFor returns the key column and bound value to address dune.<table>
// for an account on the live schema. The character id is resolved only when the
// server uses the migrated character_id key. Use for single-table operations.
func playerKeyFor(ctx context.Context, pool *pgxpool.Pool, table string, accountID int64) (string, int64, error) {
	if playerKeyColumn(ctx, pool, table) != playerKeyCharacterID {
		return playerKeyAccountID, accountID, nil
	}
	characterID, err := resolvePlayerCharacterID(ctx, pool, accountID)
	if err != nil {
		return "", 0, err
	}
	return playerKeyCharacterID, characterID, nil
}

// playerRef caches the resolved character id for operations that touch several
// per-character tables (and account-keyed tables like player_state) for the
// same account, so the character id is resolved once. Build via newPlayerRef.
type playerRef struct {
	accountID   int64
	characterID int64 // dune.encrypted_player_state.id; 0 on legacy servers
	migrated    bool
}

func newPlayerRef(ctx context.Context, pool *pgxpool.Pool, accountID int64) (playerRef, error) {
	ref := playerRef{accountID: accountID}
	if playerKeyColumn(ctx, pool, "journey_story_node") == playerKeyCharacterID {
		characterID, err := resolvePlayerCharacterID(ctx, pool, accountID)
		if err != nil {
			return playerRef{}, err
		}
		ref.characterID = characterID
		ref.migrated = true
	}
	return ref, nil
}

// keyFor returns the (column, value) addressing dune.<table> for this character
// on the live schema, without re-resolving the character id.
func (p playerRef) keyFor(ctx context.Context, pool *pgxpool.Pool, table string) (string, int64) {
	return selectPlayerKey(playerKeyColumn(ctx, pool, table), p.characterID, p.accountID)
}

// upsertPlayerTags adds and/or removes player_tags for a character, replacing
// the legacy dune.update_player_tags proc — which is account_id-keyed and so is
// absent/renamed on migrated servers (issue #267). The caller passes the
// resolved key from playerKeyFor / playerRef.keyFor.
func upsertPlayerTags(ctx context.Context, db pgExecutor, keyCol string, keyVal int64, add, remove []string) error {
	if len(add) > 0 {
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id), never user input
		q := fmt.Sprintf(`INSERT INTO dune.player_tags (%s, tag)
			SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`, keyCol)
		if _, err := db.Exec(ctx, q, keyVal, add); err != nil {
			return fmt.Errorf("add tags: %w", err)
		}
	}
	if len(remove) > 0 {
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id), never user input
		q := fmt.Sprintf(`DELETE FROM dune.player_tags WHERE %s = $1 AND tag = ANY($2::text[])`, keyCol)
		if _, err := db.Exec(ctx, q, keyVal, remove); err != nil {
			return fmt.Errorf("remove tags: %w", err)
		}
	}
	return nil
}

// ── journey / progression commands ───────────────────────────────────────────

func cmdFetchJourneyNodes(pool *pgxpool.Pool, scope string, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgJourney{err: fmt.Errorf("not connected")}
		}
		key := journeyCacheKey(scope, accountID)

		// Cache hit?
		journeyCacheMu.RLock()
		entry, ok := journeyCache[key]
		journeyCacheMu.RUnlock()
		if ok && time.Since(entry.cached) < journeyCacheTTL {
			return msgJourney{rows: entry.nodes}
		}

		ctx := context.Background()
		keyCol, keyVal, err := playerKeyFor(ctx, pool, "journey_story_node", accountID)
		if err != nil {
			return msgJourney{err: err}
		}
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
		rows, err := pool.Query(ctx, fmt.Sprintf(`
			SELECT story_node_id,
			       (complete_condition_state = 'true'::jsonb) AS is_complete,
			       (reveal_condition_state   = 'true'::jsonb) AS is_revealed,
			       has_pending_reward
			FROM dune.journey_story_node
			WHERE %s = $1
			ORDER BY story_node_id`, keyCol), keyVal)
		if err != nil {
			return msgJourney{err: err}
		}
		defer rows.Close()

		var nodes []journeyNode
		for rows.Next() {
			var n journeyNode
			var isComplete, isRevealed pgtype.Bool
			if err := rows.Scan(&n.NodeID, &isComplete, &isRevealed, &n.HasPendingReward); err != nil {
				continue
			}
			n.IsComplete = isComplete.Bool
			n.IsRevealed = isRevealed.Bool
			nodes = append(nodes, n)
		}
		if err := rows.Err(); err != nil {
			return msgJourney{err: err}
		}
		journeyCacheMu.Lock()
		journeyCache[key] = journeyCacheEntry{nodes: nodes, cached: time.Now()}
		journeyCacheMu.Unlock()
		return msgJourney{rows: nodes}
	}
}

// tagsForJourneyNodeSubtree returns the union of m_TagsToAdd for the named
// node and every descendant (matching the SQL completion behavior in
// cmdCompleteJourneyNode which flips children too). Order preserved, deduped.
func tagsForJourneyNodeSubtree(nodeID string) []string {
	if tagsData.JourneyNodeTags == nil {
		return nil
	}
	prefix := nodeID + "."
	seen := map[string]bool{}
	var out []string
	add := func(tags []string) {
		for _, t := range tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	add(tagsData.JourneyNodeTags[nodeID])
	for id, tags := range tagsData.JourneyNodeTags {
		if strings.HasPrefix(id, prefix) {
			add(tags)
		}
	}
	return out
}

// tierBumpFromTags scans applied tags for Faction.<X>.Tier<N> (N ∈ [0,5]) and
// returns the highest implied reputation per faction. Used to fire the rep
// promotion side effect when admin completion applies a tier tag.
func tierBumpFromTags(tags []string) map[string]int32 {
	out := map[string]int32{}
	// e.g. "Faction.Atreides.Tier3"
	for _, t := range tags {
		const prefix = "Faction."
		if !strings.HasPrefix(t, prefix) {
			continue
		}
		rest := t[len(prefix):]
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 {
			continue
		}
		faction := rest[:dot]
		tail := rest[dot+1:]
		if !strings.HasPrefix(tail, "Tier") {
			continue
		}
		n, err := strconv.Atoi(tail[len("Tier"):])
		if err != nil || n < 0 || n > 5 {
			continue
		}
		// +1 over the tier threshold so the in-game UI doesn't floor a tier low
		// (rep == threshold displays the tier below). Tier 0 stays at 0 — it's
		// the legitimate starting state.
		rep := factionTierThresholds[n]
		if n > 0 {
			rep++
		}
		if rep > out[faction] {
			out[faction] = rep
		}
	}
	return out
}

func factionIDByName(name string) int16 {
	switch name {
	case "Atreides":
		return 1
	case "Harkonnen":
		return 2
	case "None":
		return 3
	case "Smuggler":
		return 4
	}
	return 0
}

func cmdCompleteJourneyNode(pool *pgxpool.Pool, accountID int64, nodeID string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		keyCol, keyVal := ref.keyFor(ctx, pool, "journey_story_node")
		// Complete the node itself plus all child nodes (nodeID + ".anything").
		// The game checks sub-nodes to determine quest completion state.
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
		res, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'true'::jsonb,
			    reveal_condition_state   = 'true'::jsonb
			WHERE %s = $1
			  AND (story_node_id = $2 OR story_node_id LIKE $2 || '.%%')`, keyCol),
			keyVal, nodeID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("complete node: %w", err)}
		}
		updated := res.RowsAffected()
		if updated == 0 {
			// Node doesn't exist yet — insert it.
			// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
			_, err = pool.Exec(ctx, fmt.Sprintf(`
				INSERT INTO dune.journey_story_node
					(%s, story_node_id, has_pending_reward,
					 complete_condition_state, reveal_condition_state,
					 fail_condition_state, metadata_state, reset_group)
				VALUES ($1, $2, false,
					'true'::jsonb, 'true'::jsonb,
					'{}'::jsonb, '{}'::jsonb,
					'Default'::dune.JourneyStoryResetGroup)`, keyCol),
				keyVal, nodeID)
			if err != nil {
				return msgMutate{err: fmt.Errorf("insert node: %w", err)}
			}
			updated = 1
		}

		// Apply tags that in-game completion of the node + its descendants
		// would emit (via m_TagsToAdd). Without this the DB row is flipped
		// but the player is missing the side effects the game would have
		// written — which is why journey-only completion historically did not
		// "stick" without login/logout cycles.
		appliedTags := tagsForJourneyNodeSubtree(nodeID)
		extra, err := applyTagsWithTierBump(ctx, pool, ref, appliedTags)
		if err != nil {
			return msgMutate{err: err}
		}

		svExtra, svErr := maybeGrantSpiceVision(ctx, pool, ref.accountID, nodeID)
		if svErr != nil {
			return msgMutate{err: svErr}
		}
		extra += svExtra

		return msgMutate{ok: fmt.Sprintf("Completed %s + %d node(s)%s — takes effect on next login", nodeID, updated, extra)}
	}
}

// applyTagsWithTierBump writes `tags` into dune.player_tags and, for any
// Faction.<X>.Tier<N> (N ∈ 0–5) it sees, also raises that faction's rep + the
// FactionPlayerComponent ReputationAmount on the controller actor so the
// in-game rank UI reflects the promotion. Never lowers existing rep.
// Returns a short " , +K tag(s), bumped rep for N faction(s)" fragment for
// inclusion in the caller's success message (empty when no tags applied).
func applyTagsWithTierBump(ctx context.Context, pool *pgxpool.Pool, ref playerRef, tags []string) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	keyCol, keyVal := ref.keyFor(ctx, pool, "player_tags")
	if err := upsertPlayerTags(ctx, pool, keyCol, keyVal, tags, nil); err != nil {
		return "", fmt.Errorf("apply tags: %w", err)
	}

	extra := fmt.Sprintf(", +%d tag(s)", len(tags))

	bumps := tierBumpFromTags(tags)
	if len(bumps) == 0 {
		return extra, nil
	}

	var controllerID int64
	_ = pool.QueryRow(ctx, `
		SELECT player_controller_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, ref.accountID).Scan(&controllerID)
	if controllerID == 0 {
		// Fresh character without a player_state row — can't bump rep yet.
		// Tags landed, the rep side effect will have to wait until the
		// character first logs in. Surface in the message.
		return extra + ", rep bump skipped (no controller yet)", nil
	}

	bumped := 0
	for faction, rep := range bumps {
		fid := factionIDByName(faction)
		if fid == 0 {
			continue
		}
		var current int32
		_ = pool.QueryRow(ctx, `
			SELECT COALESCE(reputation_amount, 0)
			FROM dune.player_faction_reputation
			WHERE actor_id = $1 AND faction_id = $2`,
			controllerID, fid).Scan(&current)
		if current >= rep {
			continue
		}
		if _, err := pool.Exec(ctx,
			`SELECT dune.set_player_faction_reputation($1::bigint, $2::smallint, $3::integer)`,
			controllerID, fid, rep); err != nil {
			return "", fmt.Errorf("bump %s rep: %w", faction, err)
		}
		bumped++
	}
	if bumped > 0 {
		// Rebuild the component once from the now-updated rep table (Gap 2 fix:
		// the old per-faction update no-oped on an empty m_FactionDataArray).
		if err := syncFactionComponent(ctx, pool, controllerID); err != nil {
			return "", fmt.Errorf("sync FactionPlayerComponent: %w", err)
		}
		extra += fmt.Sprintf(", bumped rep for %d faction(s)", bumped)
	}
	return extra, nil
}

// resolveContractTags resolves a contract id (full DA_CT_ name or short alias)
// to its AddedFlagsOnCompletion list. Returns the resolved canonical name and
// the tags, or ("", nil, err) if unknown.
func resolveContractTags(contractID string) (string, []string, error) {
	name := contractID
	if full, ok := tagsData.ContractAliases[contractID]; ok {
		name = full
	}
	tags, ok := tagsData.ContractTags[name]
	if !ok || len(tags) == 0 {
		return "", nil, fmt.Errorf("unknown contract %q (check tags-data.json)", contractID)
	}
	return name, tags, nil
}

// cmdCompleteContract applies the AddedFlagsOnCompletion tags for one contract.
func cmdCompleteContract(pool *pgxpool.Pool, accountID int64, contractID string) Cmd {
	return cmdCompleteContracts(pool, accountID, []string{contractID})
}

// cmdCompleteContracts applies the union of AddedFlagsOnCompletion across
// multiple contracts in one go — one update_player_tags call, one tier-bump
// pass, plus any SkillsKeyRewards skill-block unlocks. Unknown contracts
// cause the whole batch to fail before any write so the operation is
// all-or-nothing.
func cmdCompleteContracts(pool *pgxpool.Pool, accountID int64, contractIDs []string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if err := validateContractMutationInput(accountID, contractIDs); err != nil {
			return msgMutate{err: err}
		}

		set, err := buildContractRemovalSet(contractIDs)
		if err != nil {
			return msgMutate{err: err}
		}

		ctx := context.Background()
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		extra, err := applyTagsWithTierBump(ctx, pool, ref, set.removeTags)
		if err != nil {
			return msgMutate{err: err}
		}

		// Skill grants and contract-item dismissal target account-level tables,
		// which were not part of the account_id -> character_id migration.
		grantedExtra, err := applyContractSkillGrants(ctx, pool, ref.accountID, set.removeSkills)
		if err != nil {
			return msgMutate{err: err}
		}
		extra += grantedExtra

		// Strip any in-progress ContractItem rows so the in-game quest
		// tracker doesn't keep showing the conditions for a contract we just
		// force-completed. ContractName.Name uses the short alias form
		// (no DA_CT_ prefix).
		shortNames := contractShortNames(set.resolvedNames)
		dismissedExtra, err := dismissActiveContracts(ctx, pool, ref.accountID, shortNames)
		if err != nil {
			return msgMutate{err: err}
		}
		extra += dismissedExtra

		summary := contractBatchSummary(set.resolvedNames)
		return msgMutate{ok: fmt.Sprintf("Applied %s%s — takes effect on next login", summary, extra)}
	}
}

func validateContractMutationInput(accountID int64, contractIDs []string) error {
	if accountID == 0 {
		return fmt.Errorf("account ID required")
	}
	if len(contractIDs) == 0 {
		return fmt.Errorf("at least one contract required")
	}
	return nil
}

type contractRemovalSet struct {
	resolvedNames []string
	removeTags    []string
	removeSkills  []string
}

func buildContractRemovalSet(contractIDs []string) (contractRemovalSet, error) {
	seenTag := map[string]bool{}
	seenSkill := map[string]bool{}
	set := contractRemovalSet{
		resolvedNames: make([]string, 0, len(contractIDs)),
		removeTags:    make([]string, 0, len(contractIDs)),
		removeSkills:  make([]string, 0, len(contractIDs)),
	}

	for _, id := range contractIDs {
		name, tags, err := resolveContractTags(id)
		if err != nil {
			return contractRemovalSet{}, err
		}
		set.resolvedNames = append(set.resolvedNames, name)
		for _, tag := range tags {
			if seenTag[tag] {
				continue
			}
			seenTag[tag] = true
			set.removeTags = append(set.removeTags, tag)
		}
		for _, skill := range tagsData.ContractSkillGrants[name] {
			if seenSkill[skill] {
				continue
			}
			seenSkill[skill] = true
			set.removeSkills = append(set.removeSkills, skill)
		}
	}

	return set, nil
}

func applyContractSkillGrants(ctx context.Context, pool *pgxpool.Pool, accountID int64, skills []string) (string, error) {
	if len(skills) == 0 {
		return "", nil
	}
	return grantSkillBlocks(ctx, pool, accountID, skills)
}

func contractShortNames(resolvedNames []string) []string {
	shortNames := make([]string, 0, len(resolvedNames))
	for _, full := range resolvedNames {
		shortNames = append(shortNames, strings.TrimPrefix(full, "DA_CT_"))
	}
	return shortNames
}

func removeContractTags(ctx context.Context, pool *pgxpool.Pool, ref playerRef, removeTags []string) error {
	if len(removeTags) == 0 {
		return nil
	}
	keyCol, keyVal := ref.keyFor(ctx, pool, "player_tags")
	if err := upsertPlayerTags(ctx, pool, keyCol, keyVal, nil, removeTags); err != nil {
		return fmt.Errorf("remove tags: %w", err)
	}
	return nil
}

func loadContractPawnID(ctx context.Context, pool *pgxpool.Pool, accountID int64) int64 {
	var pawnID int64
	_ = pool.QueryRow(ctx,
		`SELECT player_pawn_id FROM dune.player_state WHERE account_id = $1 LIMIT 1`,
		accountID).Scan(&pawnID)
	return pawnID
}

func stripContractSkillBlocks(ctx context.Context, pool *pgxpool.Pool, pawnID int64, removeSkills []string) (int, error) {
	if pawnID == 0 || len(removeSkills) == 0 {
		return 0, nil
	}

	stripped := 0
	for _, skill := range removeSkills {
		key := fmt.Sprintf(`(TagName="%s")`, skill)
		tag, err := pool.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				fe.components,
				ARRAY['FLevelComponent','1','ModuleData'],
				(fe.components->'FLevelComponent'->1->'ModuleData') - $2::text)
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)
			AND COALESCE(
				(fe.components->'FLevelComponent'->1->'ModuleData'->$2->>'SkillPointsSpent')::int,
				0
			) <= 1`,
			pawnID, key)
		if err != nil {
			return 0, fmt.Errorf("strip %s: %w", skill, err)
		}
		if tag.RowsAffected() > 0 {
			stripped++
		}
	}

	return stripped, nil
}

func contractBatchSummary(resolvedNames []string) string {
	summary := resolvedNames[0]
	if len(resolvedNames) > 1 {
		summary = fmt.Sprintf("%d contracts", len(resolvedNames))
	}
	return summary
}

// cmdReverseContracts removes the AddedFlagsOnCompletion tags and strips the
// Skills.Key.* ModuleData entries that cmdCompleteContracts wrote. Skill blocks
// are only removed when SkillPointsSpent <= 1 — branches the player genuinely
// levelled beyond the admin grant are left intact.
func cmdReverseContracts(pool *pgxpool.Pool, accountID int64, contractIDs []string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if err := validateContractMutationInput(accountID, contractIDs); err != nil {
			return msgMutate{err: err}
		}

		set, err := buildContractRemovalSet(contractIDs)
		if err != nil {
			return msgMutate{err: err}
		}

		ctx := context.Background()
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		if err := removeContractTags(ctx, pool, ref, set.removeTags); err != nil {
			return msgMutate{err: err}
		}

		pawnID := loadContractPawnID(ctx, pool, ref.accountID)
		stripped, err := stripContractSkillBlocks(ctx, pool, pawnID, set.removeSkills)
		if err != nil {
			return msgMutate{err: err}
		}

		summary := contractBatchSummary(set.resolvedNames)
		return msgMutate{ok: fmt.Sprintf(
			"Reversed %s: removed %d tag(s), stripped %d skill block(s) — takes effect on next login",
			summary, len(set.removeTags), stripped)}
	}
}

// cmdResetJobSkills removes every ModuleData entry whose SkillArea matches
// the named job — Key blocks, Abilities, Attributes, Perks — fully nuking
// that class's skill tree. Key-block removal alone leaves orphaned ability
// rows (e.g. SuspensorGrenade_Reduction lingers after Skills.Key.Trooper1
// is gone) which the game still treats as refundable for 1 SP each (the
// "phantom SP" bug). Removing every SkillArea-matching module avoids that.
func cmdResetJobSkills(pool *pgxpool.Pool, accountID int64, job string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		modules := tagsData.JobAllModules[job]
		if len(modules) == 0 {
			return msgMutate{err: fmt.Errorf("unknown job %q (check tags-data.json job_all_modules)", job)}
		}
		ctx := context.Background()

		var pawnID int64
		_ = pool.QueryRow(ctx, `
			SELECT player_pawn_id FROM dune.player_state
			WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
		if pawnID == 0 {
			return msgMutate{err: fmt.Errorf("no pawn for account %d", accountID)}
		}

		// Build the (TagName="...") keyed names in one pass and use the
		// jsonb minus-text[] operator to drop them all in a single UPDATE.
		keys := make([]string, len(modules))
		for i, m := range modules {
			keys[i] = fmt.Sprintf(`(TagName="%s")`, m)
		}
		tag, err := pool.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				fe.components,
				ARRAY['FLevelComponent','1','ModuleData'],
				(fe.components->'FLevelComponent'->1->'ModuleData') - $2::text[])
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)`,
			pawnID, keys)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset %s tree: %w", job, err)}
		}
		if tag.RowsAffected() == 0 {
			return msgMutate{ok: fmt.Sprintf("Reset %s skill tree — no ModuleData on pawn", job)}
		}
		return msgMutate{ok: fmt.Sprintf("Reset %s skill tree — scanned %d module slot(s)", job, len(modules))}
	}
}

// starterAbilityByJob is the canonical tier-1 starter ability the game
// auto-grants on character creation for each class — empirically observed
// for BG (VoiceCompel) and Trooper (SuspensorGrenade_Reduction); the others
// derived from DT_TrainingModules.json by picking the unique
// PrereqModuleTags_And = [Skills.Key.<Job>1] ability at GridPosition (3,0),
// which is the slot the game uses for the "middle of the first row" starter.
var starterAbilityByJob = map[string]string{
	"BeneGesserit":  "Skills.Ability.VoiceCompel",
	"Mentat":        "Skills.Ability.PoisonCapsuleLauncher",
	"Planetologist": "Skills.Ability.SuspensorPad",
	"Swordmaster":   "Skills.Ability.DeflectionSlow",
	"Trooper":       "Skills.Ability.SuspensorGrenade_Reduction",
}

func resolveStarterClassAbility(job string) (string, error) {
	if _, ok := tagsData.JobSkillBlocks[job]; !ok {
		return "", fmt.Errorf("unknown job %q", job)
	}
	ability, ok := starterAbilityByJob[job]
	if !ok {
		return "", fmt.Errorf("no starter ability mapping for %q", job)
	}
	return ability, nil
}

func loadPawnIDForAccount(ctx context.Context, pool *pgxpool.Pool, accountID int64) (int64, error) {
	var pawnID int64
	_ = pool.QueryRow(ctx, `
		SELECT player_pawn_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
	if pawnID == 0 {
		return 0, fmt.Errorf("no pawn for account %d", accountID)
	}
	return pawnID, nil
}

func loadStarterTagForPawn(ctx context.Context, pool *pgxpool.Pool, pawnID int64) string {
	var starterTag string
	_ = pool.QueryRow(ctx, `
		SELECT fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName'
		FROM dune.fgl_entities fe
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
		WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`,
		pawnID).Scan(&starterTag)
	return starterTag
}

func starterKeysToRemove(oldStarterTag, newJob string) []string {
	if !strings.HasPrefix(oldStarterTag, "Skills.Key.") || !strings.HasSuffix(oldStarterTag, "1") {
		return nil
	}
	oldJob := strings.TrimSuffix(strings.TrimPrefix(oldStarterTag, "Skills.Key."), "1")
	if oldJob == "" || oldJob == newJob {
		return nil
	}
	keys := []string{fmt.Sprintf(`(TagName="%s")`, oldStarterTag)}
	if oldAbility, ok := starterAbilityByJob[oldJob]; ok {
		keys = append(keys, fmt.Sprintf(`(TagName="%s")`, oldAbility))
	}
	return keys
}

func starterClassTagAndKeys(job, ability string) (starterTag, starterKey, abilityKey string) {
	starterTag = fmt.Sprintf("Skills.Key.%s1", job)
	starterKey = fmt.Sprintf(`(TagName="%s")`, starterTag)
	abilityKey = fmt.Sprintf(`(TagName="%s")`, ability)
	return starterTag, starterKey, abilityKey
}

func applyStarterClassUpdate(
	ctx context.Context,
	pool *pgxpool.Pool,
	pawnID int64,
	newStarterTag, newStarterKey string,
	keysToRemove []string,
	newAbilityKey string,
) error {
	_, err := pool.Exec(ctx, `
		UPDATE dune.fgl_entities fe
		SET components = jsonb_set(
			jsonb_set(
				jsonb_set(
					jsonb_set(
						fe.components,
						ARRAY['FLevelComponent','1','ModuleData'],
						(fe.components->'FLevelComponent'->1->'ModuleData') - $4::text[]),
					ARRAY['FLevelComponent','1','StarterSkillTreeTag','TagName'],
					to_jsonb($2::text)),
				ARRAY['FLevelComponent','1','ModuleData',$3],
				'{"SkillPointsSpent": 1}'::jsonb,
				true),
			ARRAY['FLevelComponent','1','ModuleData',$5],
			'{"SkillPointsSpent": 1}'::jsonb,
			true)
		WHERE fe.entity_id = (
			SELECT entity_id FROM dune.actor_fgl_entities
			WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
		)`, pawnID, newStarterTag, newStarterKey, keysToRemove, newAbilityKey)
	if err != nil {
		return fmt.Errorf("set starter tag: %w", err)
	}
	return nil
}

func formatStarterClassMessage(job, newStarterTag, newAbility string, removedCount int) string {
	msg := fmt.Sprintf("Starter class set to %s (%s + %s active)", job, newStarterTag, newAbility)
	if removedCount > 0 {
		msg += fmt.Sprintf(", cleared previous starter (%d module(s))", removedCount)
	}
	return msg
}

// cmdSetStarterClass swaps the player's starter class:
//  1. removes the previous starter's Skills.Key.<Old>1 block + its starter
//     ability from ModuleData (so you don't end up with two starters
//     stacked after switching), then
//  2. writes the new StarterSkillTreeTag pointer,
//  3. activates the new Skills.Key.<Job>1 block at SpSpent: 1,
//  4. grants the new tier-1 starter ability at SpSpent: 1.
//
// Result on next login: only one class is recognised as starter, with its
// canonical first ability already learned.
func cmdSetStarterClass(pool *pgxpool.Pool, accountID int64, job string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		newAbility, err := resolveStarterClassAbility(job)
		if err != nil {
			return msgMutate{err: err}
		}
		ctx := context.Background()

		pawnID, err := loadPawnIDForAccount(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}

		// Look up the current starter so we can deactivate it. Format is
		// "Skills.Key.<Job>1"; we strip the prefix/suffix to recover the
		// job name and look up its starter-ability for removal.
		oldStarterTag := loadStarterTagForPawn(ctx, pool, pawnID)
		keysToRemove := starterKeysToRemove(oldStarterTag, job)
		newStarterTag, newStarterKey, newAbilityKey := starterClassTagAndKeys(job, newAbility)

		// One chained jsonb update: strip old keys, write new tag, activate
		// new starter block, grant new starter ability. - operator on an
		// empty text[] is a no-op so it's safe when there's no old starter
		// to clean up (e.g. fresh character with StarterSkillTreeTag=None).
		if err := applyStarterClassUpdate(ctx, pool, pawnID, newStarterTag, newStarterKey, keysToRemove, newAbilityKey); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: formatStarterClassMessage(job, newStarterTag, newAbility, len(keysToRemove))}
	}
}

// cmdGrantJobSkills unlocks every bExternal Skills.Key.* module in the named
// job's skill tree (e.g. "Trooper" → Trooper1/2/3 + CapstoneGadgets +
// CapstoneWeaponry + CapstoneSuspensorTech). Only ~⅓ of these blocks are
// contract-granted via SkillsKeyRewards; the rest are normally unlocked by
// trainer dialogue or auto on level progression, so the admin Unlock Trainer
// action calls this after the contract batch to bypass those gates.
func cmdGrantJobSkills(pool *pgxpool.Pool, accountID int64, job string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		blocks := tagsData.JobSkillBlocks[job]
		if len(blocks) == 0 {
			return msgMutate{err: fmt.Errorf("unknown job %q (check tags-data.json job_skill_blocks)", job)}
		}
		ctx := context.Background()
		extra, err := grantSkillBlocks(ctx, pool, accountID, blocks)
		if err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf("Unlocked %s skill tree%s — takes effect on next login", job, extra)}
	}
}

// dismissActiveContracts deletes any ContractItem inventory entries whose
// stats.FContractItemStats.ContractName.Name matches one of shortNames.
// Active contract items drive the in-game quest tracker, so after force-
// completing a contract via tags we need to remove the live instance
// otherwise the player keeps seeing "Deploy Assault Seekers" / etc as
// outstanding. No-op if the player never had the contract active.
func dismissActiveContracts(ctx context.Context, pool *pgxpool.Pool, accountID int64, shortNames []string) (string, error) {
	if len(shortNames) == 0 {
		return "", nil
	}
	var pawnID int64
	_ = pool.QueryRow(ctx, `
		SELECT player_pawn_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
	if pawnID == 0 {
		return "", nil
	}
	tag, err := pool.Exec(ctx, `
		DELETE FROM dune.items
		WHERE template_id = 'ContractItem'
		  AND inventory_id IN (
		      SELECT id FROM dune.inventories
		      WHERE actor_id = $1 AND inventory_type = 29
		  )
		  AND stats->'FContractItemStats'->1->'ContractName'->>'Name' = ANY($2::text[])`,
		pawnID, shortNames)
	if err != nil {
		return "", fmt.Errorf("dismiss active contracts: %w", err)
	}
	n := tag.RowsAffected()
	if n == 0 {
		return "", nil
	}
	return fmt.Sprintf(", dismissed %d active contract(s)", n), nil
}

// grantSkillBlocks ensures each Skills.Key.<X> entry exists in the player's
// FLevelComponent.ModuleData with SkillPointsSpent: 1 (the format the game
// itself writes when a trainer's SkillsKeyRewards fires). If an entry already
// exists it's left alone — preserves any further SP the player may have
// already spent on that branch's child nodes. Returns a short fragment to
// append to the caller's success message.
func grantSkillBlocks(ctx context.Context, pool *pgxpool.Pool, accountID int64, skillKeys []string) (string, error) {
	var pawnID int64
	_ = pool.QueryRow(ctx, `
		SELECT player_pawn_id FROM dune.player_state
		WHERE account_id = $1 LIMIT 1`, accountID).Scan(&pawnID)
	if pawnID == 0 {
		return ", skill grants skipped (no pawn yet)", nil
	}

	granted := 0
	for _, sk := range skillKeys {
		key := fmt.Sprintf(`(TagName="%s")`, sk)
		// Set ModuleData[key] = {"SkillPointsSpent": 1} when:
		//   - key doesn't exist yet (game never created a placeholder), OR
		//   - key exists with SpSpent <= 0 (game-created placeholder that
		//     means "available but not yet purchased").
		// SpSpent >= 1 is left alone so any further SP the player has
		// already spent on child nodes survives.
		tag, err := pool.Exec(ctx, `
			UPDATE dune.fgl_entities fe
			SET components = jsonb_set(
				fe.components,
				ARRAY['FLevelComponent','1','ModuleData',$2],
				'{"SkillPointsSpent": 1}'::jsonb,
				true)
			WHERE fe.entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
			)
			AND COALESCE(
				(fe.components->'FLevelComponent'->1->'ModuleData'->$2->>'SkillPointsSpent')::int,
				0
			) < 1`,
			pawnID, key)
		if err != nil {
			return "", fmt.Errorf("grant %s: %w", sk, err)
		}
		if tag.RowsAffected() > 0 {
			granted++
		}
	}
	if granted == 0 {
		return ", no skill blocks needed (all already unlocked)", nil
	}
	return fmt.Sprintf(", unlocked %d skill block(s)", granted), nil
}

// spiceVisionEnableSQL sets FSpiceAddictionComponent.SpiceVisionEnabledStatus
// to "FullyEnabled" on the player's DuneCharacter FGL entity. This is the
// persistent flag the game reads to determine whether the player has unlocked
// the Prescience state (3rd ability slot + spice-vision buff). In-game it is
// written by the 4th Trial of Aql quest script, not by a journey tag — hence
// it must be applied explicitly when admin-completing FindTheFremen.
const spiceVisionEnableSQL = `
	UPDATE dune.fgl_entities fe
	SET components = jsonb_set(
		fe.components,
		ARRAY['FSpiceAddictionComponent','1','SpiceVisionEnabledStatus'],
		'"FullyEnabled"'::jsonb,
		true)
	WHERE fe.entity_id = (
		SELECT entity_id FROM dune.actor_fgl_entities
		WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
	)
	AND COALESCE(
		fe.components->'FSpiceAddictionComponent'->1->>'SpiceVisionEnabledStatus',
		''
	) <> 'FullyEnabled'`

// maybeGrantSpiceVision conditionally enables SpiceVision for the account
// when nodeID is within the FindTheFremen quest. It is a thin wrapper so
// cmdCompleteJourneyNode stays under the complexity gate.
func maybeGrantSpiceVision(ctx context.Context, pool *pgxpool.Pool, accountID int64, nodeID string) (string, error) {
	if !nodeIDTriggersSpiceVision(nodeID) {
		return "", nil
	}
	var pawnID int64
	_ = pool.QueryRow(ctx,
		`SELECT player_pawn_id FROM dune.player_state WHERE account_id = $1 LIMIT 1`,
		accountID).Scan(&pawnID)
	return grantSpiceVision(ctx, pool, pawnID)
}

// nodeIDTriggersSpiceVision reports whether completing nodeID should also
// enable SpiceVision (Prescience). Only DA_MQ_FindTheFremen and its subtree
// warrant this — it is the quest that contains the 4th Trial of Aql.
func nodeIDTriggersSpiceVision(nodeID string) bool {
	const root = "DA_MQ_FindTheFremen"
	return nodeID == root || len(nodeID) > len(root) && nodeID[:len(root)+1] == root+"."
}

// grantSpiceVision enables the Prescience / SpiceVision state on the player's
// DuneCharacter FGL entity. It is idempotent — a no-op if already enabled.
// Returns a short extra fragment for the caller's success message, or "" if
// the pawn was not found or the flag was already set.
func grantSpiceVision(ctx context.Context, pool *pgxpool.Pool, pawnID int64) (string, error) {
	if pawnID == 0 {
		return ", spice vision skipped (no pawn yet)", nil
	}
	res, err := pool.Exec(ctx, spiceVisionEnableSQL, pawnID)
	if err != nil {
		return "", fmt.Errorf("grant spice vision: %w", err)
	}
	if res.RowsAffected() == 0 {
		return "", nil
	}
	return ", enabled Prescience (SpiceVision)", nil
}

// allJourneyTags returns the union of every tag any journey node would emit
// on completion. Used by Wipe All to also strip tags that prior completions
// may have applied. Rep is intentionally not touched — natural progression
// is monotonic and we don't try to roll it back.
func allJourneyTags() []string {
	if tagsData.JourneyNodeTags == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, tags := range tagsData.JourneyNodeTags {
		for _, t := range tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func cmdResetJourneyNode(pool *pgxpool.Pool, accountID int64, nodeID string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		jCol, jVal := ref.keyFor(ctx, pool, "journey_story_node")
		// #nosec G201 -- jCol is a fixed internal allowlist (character_id|account_id)
		_, err = pool.Exec(ctx, fmt.Sprintf(`
			UPDATE dune.journey_story_node
			SET complete_condition_state = 'false'::jsonb,
			    has_pending_reward       = false
			WHERE %s = $1
			  AND (story_node_id = $2 OR story_node_id LIKE $2 || '.%%')`, jCol),
			jVal, nodeID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("reset node: %w", err)}
		}

		// Also strip any tags this node + its descendants would have emitted
		// on completion.
		removeTags := tagsForJourneyNodeSubtree(nodeID)
		extra := ""
		if len(removeTags) > 0 {
			tCol, tVal := ref.keyFor(ctx, pool, "player_tags")
			if err = upsertPlayerTags(ctx, pool, tCol, tVal, nil, removeTags); err != nil {
				return msgMutate{err: fmt.Errorf("remove node tags: %w", err)}
			}
			extra = fmt.Sprintf(", removed %d tag(s)", len(removeTags))
		}
		return msgMutate{ok: fmt.Sprintf("Reset %s%s", nodeID, extra)}
	}
}

func cmdWipeJourneyNodes(pool *pgxpool.Pool, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}
		jCol, jVal := ref.keyFor(ctx, pool, "journey_story_node")
		// Replaces the legacy dune.delete_all_journey_story_nodes proc, which is
		// account_id-keyed and absent/renamed on migrated servers (issue #267).
		// #nosec G201 -- jCol is a fixed internal allowlist (character_id|account_id)
		if _, err = pool.Exec(ctx,
			fmt.Sprintf(`DELETE FROM dune.journey_story_node WHERE %s = $1`, jCol), jVal); err != nil {
			return msgMutate{err: fmt.Errorf("wipe journey: %w", err)}
		}

		// Strip every tag any journey node could have emitted, so the
		// player's tag state matches the post-wipe journey state.
		removeTags := allJourneyTags()
		extra := ""
		if len(removeTags) > 0 {
			tCol, tVal := ref.keyFor(ctx, pool, "player_tags")
			if err = upsertPlayerTags(ctx, pool, tCol, tVal, nil, removeTags); err != nil {
				return msgMutate{err: fmt.Errorf("remove journey tags: %w", err)}
			}
			extra = fmt.Sprintf(", removed %d journey tag(s)", len(removeTags))
		}
		return msgMutate{ok: fmt.Sprintf("Wiped all journey nodes for account %d%s", ref.accountID, extra)}
	}
}

// climbTheRanksNodes are the journey nodes that gate access to Landsraad
// rank 5–20 progression (DA_FQ = Dune Awakening Faction Quest).
// Both parent and child nodes must be completed — confirmed by in-game observation.
// These are faction-independent.
var climbTheRanksNodes = []string{
	"DA_FQ_ClimbTheRanks.Rank5To20.MeetSponsor",
	"DA_FQ_ClimbTheRanks.Rank5To20.MeetSponsor.TalkToSponsor",
	"DA_FQ_ClimbTheRanks.Rank5To20.StartLandsraadOnboarding",
	"DA_FQ_ClimbTheRanks.Rank5To20.StartLandsraadOnboarding.ReportToMasterOfAssassins",
	"DA_FQ_ClimbTheRanks.Rank5To20.CompleteLandsraadMission",
	"DA_FQ_ClimbTheRanks.Rank5To20.CompleteLandsraadMission.CompleteOnboardingJourney1",
	"DA_FQ_ClimbTheRanks.Rank5To20.CraftAugmentation",
	"DA_FQ_ClimbTheRanks.Rank5To20.CraftAugmentation.CompleteOnboardingJourney2",
}

// climbTheRanksStoryNodes are the faction-neutral storyline beats observed
// completed on both rank-up reference characters (rank 19 Atreides and rank 8
// Harkonnen). These cover the chapter-2 → rank-5-onboarding journey beats.
var climbTheRanksStoryNodes = []string{
	"DA_FQ_ClimbTheRanks.HuntingSkorda",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInArrakeen",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInMysaTarrill",
	"DA_FQ_ClimbTheRanks.HuntingSkorda.FindSkorda.SkordaInOodham",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.FindCanister",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.InvestigateSandflies",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.TrackDownPilot",
	"DA_FQ_ClimbTheRanks.GatheringIntelligence.TrackDownContainer.TrackDownRedScorpion",
	"DA_FQ_ClimbTheRanks.JoinAHouse",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself.ChooseASide",
	"DA_FQ_ClimbTheRanks.JoinAHouse.ProveYourself.Rank1Contracts",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.FindTheSpy",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.GetSpyMission",
	"DA_FQ_ClimbTheRanks.JoinAHouse.StrikeADeal.TalkToARecruiter",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2.ContributeToWarEffort_Atreides",
	"DA_FQ_ClimbTheRanks.ClimbTheRanksR2.ContributeToWarEffort_Atreides.CompleteContractsR2",
}

// climbTheRanksStoryNodesAtreides are the Atreides-side storyline beats
// (Ch2→Ch3 transition + Test of Loyalty + Atreides investigations).
var climbTheRanksStoryNodesAtreides = []string{
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre.TheCall",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Atre.TheCall.AnswerTheCall",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty.GetMaximToBackOff",
	"DA_FQ_ClimbTheRanks.ATestOfLoyalty.GetMaximToBackOff.FindSemuta",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides",
	`DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides.Complete "Track Down Skorda" Contract`,
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Atreides.InvestigateWreck_Atreides.MeetAndreaGanan",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.DeviseAPlan_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.DeviseAPlan_Atreides.TellThufirAboutDelphis",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.PledgeAllegiance_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.PledgeAllegiance_Atreides.PledgeAllegiance_Atreides_Sub",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.SecureLastContainer_Atreides",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Atreides.SecureLastContainer_Atreides.RecoverSheolContainer_Atreides",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.ChoosePoisonOrSpare",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.CompleteWarProfiteerContract",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.FindBusinessman",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PunishTraitor.TalkToThufirAgain",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.MeetThufir",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.ReturnToGanan",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Atreides.PutFindingsToTest.SpeakWithGanan",
}

// climbTheRanksStoryNodesHarkonnen are the Harkonnen-side storyline beats
// (Ch2→Ch3 transition + Test of Treachery + Harkonnen investigations).
var climbTheRanksStoryNodesHarkonnen = []string{
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark.TheCall",
	"DA_FQ_ClimbTheRanks.TransitionToCh3_Hark.TheCall.AnswerTheCall",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery.GetAntonToBackOff",
	"DA_FQ_ClimbTheRanks.ATestOfTreachery.GetAntonToBackOff.FindCounterfeitEvidence",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen",
	`DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen.Complete "Track Down Skorda" Contract`,
	"DA_FQ_ClimbTheRanks.InvestigateKytheria_Harkonnen.InvestigateWreck_Harkonnen.MeetSimoneVonKonig",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.DeviseAPlan_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.DeviseAPlan_Harkonnen.TellPiterAboutEuporia",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.PledgeAllegiance_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.PledgeAllegiance_Harkonnen.PledgeAllegiance_Harkonnen_Sub",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.SecureLastContainer_Harkonnen",
	"DA_FQ_ClimbTheRanks.InvestigateDelphis_Harkonnen.SecureLastContainer_Harkonnen.RecoverSheolContainer_Harkonnen",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.DeliverResults",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.MeetPiter",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.ReturnToVonKonig",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.LeverageYourFindings.SpeakWithVonKonig",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap.PoisonOrWarnPiter",
	"DA_FQ_ClimbTheRanks.PoisonedSpice_Harkonnen.TakeALeap.TalkToPiterAgain",
}

// landsraadMissionNodes* are the weekly Landsraad mission journey nodes (DA_SQ =
// Dune Awakening Side Quest). Completed naturally by doing one Landsraad mission
// in-game; required alongside climbTheRanksNodes for rank 5→20 progression.
var landsraadMissionNodesAtreides = []string{
	"DA_SQ_OverlandMap.AtreLandsraadMission",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreAccept",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreKeyStone",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreComplete",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreReturn",
	"DA_SQ_OverlandMap.AtreLandsraadMission.AtreMission.AtreClaimReward",
}

var landsraadMissionNodesHarkonnen = []string{
	"DA_SQ_OverlandMap.HarkLandsraadMission",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkAccept",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkKeyStone",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkComplete",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkReturn",
	"DA_SQ_OverlandMap.HarkLandsraadMission.HarkMission.HarkClaimReward",
}

// nodesForPreset returns the journey node IDs to complete for a faction+preset.
// ch3_start: Rank5To20 onboarding + faction-neutral chapter-2 storyline + chosen
// faction's Ch2→Ch3 transition / Test of Loyalty(Treachery) / investigations /
// poisoned spice arc — i.e. everything required for a fresh character to land
// at rank 5 (House Operator), so rank 6-19 can be earned organically.
// rank19_eligible: same set + the weekly Landsraad mission tree, fast-forwarded
// to tier 19.
func nodesForPreset(faction, preset string) []string {
	nodes := append([]string{}, climbTheRanksNodes...)
	nodes = append(nodes, climbTheRanksStoryNodes...)
	switch faction {
	case "atreides":
		nodes = append(nodes, climbTheRanksStoryNodesAtreides...)
	case "harkonnen":
		nodes = append(nodes, climbTheRanksStoryNodesHarkonnen...)
	}
	if preset == "rank19_eligible" {
		switch faction {
		case "atreides":
			nodes = append(nodes, landsraadMissionNodesAtreides...)
		case "harkonnen":
			nodes = append(nodes, landsraadMissionNodesHarkonnen...)
		}
	}
	return nodes
}

const progressionUnlockMaxTier = 5

type progressionFactionConfig struct {
	factionID        int16
	dialogueFlag     string
	alignedFlag      string
	metRecruiterFlag string
	factionUnlocked  string
	recruitmentDone  string
}

func progressionFactionConfigFor(faction string) (progressionFactionConfig, error) {
	switch faction {
	case "atreides":
		return progressionFactionConfig{
			factionID:        1,
			dialogueFlag:     "DialogueFlags.Factions.SentToMeetHawat",
			alignedFlag:      "DialogueFlags.Factions.AlignedAtreides",
			metRecruiterFlag: "DialogueFlags.Factions.MetHawat",
			factionUnlocked:  "Contract.Tracking.AtreidesFactionUnlocked",
			recruitmentDone:  "Contract.Tracking.AtreidesRecruitmentCompleted",
		}, nil
	case "harkonnen":
		return progressionFactionConfig{
			factionID:        2,
			dialogueFlag:     "DialogueFlags.Factions.SentToPiterDeVries",
			alignedFlag:      "DialogueFlags.Factions.AlignedHarkonnen",
			metRecruiterFlag: "DialogueFlags.Factions.MetPiterDeVries",
			factionUnlocked:  "Contract.Tracking.HarkonnenFactionUnlocked",
			recruitmentDone:  "Contract.Tracking.HarkonnenRecruitmentCompleted",
		}, nil
	default:
		return progressionFactionConfig{}, fmt.Errorf("faction must be atreides or harkonnen")
	}
}

func progressionTargetTierForPreset(preset string) (int, error) {
	switch preset {
	case "ch3_start":
		return 5, nil
	case "rank19_eligible":
		return 19, nil
	default:
		return 0, fmt.Errorf("preset must be ch3_start or rank19_eligible")
	}
}

func progressionUnlockTags(cfg progressionFactionConfig, targetTier int) []string {
	factionName := factionDisplayName(cfg.factionID)
	// Faction.<X>.TierN is only a real gameplay tag for N ∈ [0,5] — see
	// DA_Atreides.json / DA_Harkonnen.json m_FactionTiers, where Tier 6+
	// all have m_FactionTierTag.TagName == "None". Tier 5 flips
	// m_bAllowPromotionThroughReputation to true, after which rep alone
	// advances the displayed rank. So Tier0–5 + a rep >= threshold[19] is
	// enough to display rank 19 — no need to write phantom Tier6..19 tags.
	allTags := []string{
		cfg.dialogueFlag, cfg.alignedFlag, cfg.metRecruiterFlag,
		cfg.factionUnlocked, cfg.recruitmentDone,
		"DialogueFlags.Factions.FactionIntro",
		"DialogueFlags.Factions.FactionRank1",
		"DialogueFlags.Factions.FactionRank3",
		"DialogueFlags.Factions.MetARecruiter",
		"DialogueFlags.Factions.PlayedAllegianceCinematic",
		"DialogueFlags.Factions.SeenAnvilCinematic",
	}
	if targetTier >= 19 {
		allTags = append(allTags, "Journey.LandsraadContractsUnlocked")
	}
	for tier := 0; tier <= progressionUnlockMaxTier; tier++ {
		allTags = append(allTags, fmt.Sprintf("Faction.%s.Tier%d", factionName, tier))
	}
	return allTags
}

func resolveProgressionUnlockPlayer(ctx context.Context, pool *pgxpool.Pool, actorID int64) (accountID, controllerID int64, flsID string, err error) {
	if err = pool.QueryRow(ctx, `
		SELECT COALESCE(a.owner_account_id, 0),
		       COALESCE(ps.player_controller_id, 0)
		FROM dune.actors a
		LEFT JOIN dune.player_state ps ON ps.account_id = a.owner_account_id
		WHERE a.id = $1`, actorID,
	).Scan(&accountID, &controllerID); err != nil || accountID == 0 {
		return 0, 0, "", fmt.Errorf("player %d not found or has no account", actorID)
	}
	if controllerID == 0 {
		return 0, 0, "", fmt.Errorf("player %d has no controller actor", actorID)
	}
	flsID, err = rawFuncomID(ctx, pool, accountID)
	if err != nil || flsID == "" {
		return 0, 0, "", fmt.Errorf("player %d has no FLS ID", actorID)
	}
	return accountID, controllerID, flsID, nil
}

func applyProgressionUnlock(
	ctx context.Context,
	pool *pgxpool.Pool,
	ref playerRef,
	controllerID int64,
	flsID string,
	factionID int16,
	targetTier int,
	journeyNodes, allTags []string,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// NOTE(issue #267): this still routes through the game's own
	// complete_journey_story_nodes_for_player proc (keyed by FLS/Funcom id).
	// If a migrated server's copy of that proc fails, this needs an inline
	// character_id-keyed upsert like the other journey writes — tracked as a
	// follow-up; the operator journey/tag paths are the reported regressions.
	if _, err = tx.Exec(ctx,
		`SELECT dune.complete_journey_story_nodes_for_player($1, $2::text[])`,
		flsID, journeyNodes); err != nil {
		return fmt.Errorf("complete journey nodes: %w", err)
	}

	// Align the player with the chosen faction. Required for fresh / unaligned
	// characters (no player_faction row) — without this the rank UI doesn't
	// reflect tier changes because the game treats the player as unaligned.
	// neutral_faction_id = 3 ("None") so this proc takes the upsert branch.
	if _, err = tx.Exec(ctx,
		`SELECT dune.change_player_faction($1::bigint, $2::smallint, 3::smallint, NOW()::timestamp)`,
		controllerID, factionID); err != nil {
		return fmt.Errorf("change_player_faction: %w", err)
	}

	tCol, tVal := ref.keyFor(ctx, pool, "player_tags")
	if err = upsertPlayerTags(ctx, tx, tCol, tVal, allTags, nil); err != nil {
		return fmt.Errorf("update player tags: %w", err)
	}

	// +1 over the tier threshold: the game UI floors at the threshold
	// (rep == threshold shows the tier below), so we nudge just over.
	targetRep := factionTierThresholds[targetTier] + 1
	if _, err = tx.Exec(ctx,
		`SELECT dune.set_player_faction_reputation($1, $2, $3)`,
		controllerID, factionID, targetRep); err != nil {
		return fmt.Errorf("set faction rep: %w", err)
	}
	if err = syncFactionComponent(ctx, tx, controllerID); err != nil {
		return fmt.Errorf("update FactionPlayerComponent rep: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func formatProgressionUnlockSuccess(
	preset, faction string,
	journeyNodeCount int,
	factionName string,
	targetTier int,
	controllerID int64,
) string {
	return fmt.Sprintf(
		"Progression unlock (%s/%s): %d journey nodes completed + %s tier tags 0–%d + rep tier %d on controller %d — takes effect on next login",
		preset, faction, journeyNodeCount, factionName, progressionUnlockMaxTier, targetTier, controllerID)
}

func resolveProgressionAccountID(ctx context.Context, pool *pgxpool.Pool, actorID int64) (int64, error) {
	var accountID int64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(owner_account_id, 0) FROM dune.actors WHERE id = $1`,
		actorID).Scan(&accountID); err != nil || accountID == 0 {
		return 0, fmt.Errorf("player %d not found or has no account", actorID)
	}
	return accountID, nil
}

func progressionReverseTags(baseTags, nodes []string) []string {
	allTags := append([]string{}, baseTags...)
	seen := make(map[string]bool, len(allTags))
	for _, tag := range allTags {
		seen[tag] = true
	}
	for _, node := range nodes {
		for _, tag := range tagsForJourneyNodeSubtree(node) {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			allTags = append(allTags, tag)
		}
	}
	return allTags
}

func applyProgressionReverse(ctx context.Context, pool *pgxpool.Pool, ref playerRef, allTags, nodes []string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tCol, tVal := ref.keyFor(ctx, pool, "player_tags")
	if err = upsertPlayerTags(ctx, tx, tCol, tVal, nil, allTags); err != nil {
		return 0, fmt.Errorf("remove tags: %w", err)
	}

	jCol, jVal := ref.keyFor(ctx, pool, "journey_story_node")
	// #nosec G201 -- jCol is a fixed internal allowlist (character_id|account_id)
	result, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE dune.journey_story_node
		SET complete_condition_state = 'false'::jsonb,
		    has_pending_reward       = false
		WHERE %s = $1
		  AND story_node_id = ANY($2::text[])`, jCol),
		jVal, nodes)
	if err != nil {
		return 0, fmt.Errorf("reset journey nodes: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func formatProgressionReverseSuccess(preset, faction string, resetNodes int64, removedTags int) string {
	return fmt.Sprintf(
		"Reversed progression unlock (%s/%s): reset %d node(s), removed %d tag(s) — takes effect on next login",
		preset, faction, resetNodes, removedTags)
}

// cmdProgressionUnlock completes all prerequisite faction story journey nodes,
// writes the corresponding gameplay tags, and sets reputation to the preset's
// target tier.
//
// faction: "atreides" | "harkonnen"
// preset:  "ch3_start" (rank 5 — House Operator) | "rank19_eligible" (rank 19)
func cmdProgressionUnlock(pool *pgxpool.Pool, actorID int64, faction, preset string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		cfg, err := progressionFactionConfigFor(faction)
		if err != nil {
			return msgMutate{err: err}
		}
		targetTier, err := progressionTargetTierForPreset(preset)
		if err != nil {
			return msgMutate{err: err}
		}

		journeyNodes := nodesForPreset(faction, preset)
		factionName := factionDisplayName(cfg.factionID)
		allTags := progressionUnlockTags(cfg, targetTier)

		ctx := context.Background()
		accountID, controllerID, flsID, err := resolveProgressionUnlockPlayer(ctx, pool, actorID)
		if err != nil {
			return msgMutate{err: err}
		}
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}

		if err := applyProgressionUnlock(
			ctx,
			pool,
			ref,
			controllerID,
			flsID,
			cfg.factionID,
			targetTier,
			journeyNodes,
			allTags,
		); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: formatProgressionUnlockSuccess(
			preset,
			faction,
			len(journeyNodes),
			factionName,
			targetTier,
			controllerID,
		)}
	}
}

// cmdReverseProgressionUnlock undoes cmdProgressionUnlock: resets the journey
// nodes from nodesForPreset back to not-complete and removes all tags the
// forward function wrote. Reputation and faction alignment are not touched —
// matching the existing per-node reset behaviour.
func cmdReverseProgressionUnlock(pool *pgxpool.Pool, actorID int64, faction, preset string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		cfg, err := progressionFactionConfigFor(faction)
		if err != nil {
			return msgMutate{err: err}
		}
		targetTier, err := progressionTargetTierForPreset(preset)
		if err != nil {
			return msgMutate{err: err}
		}

		ctx := context.Background()
		accountID, err := resolveProgressionAccountID(ctx, pool, actorID)
		if err != nil {
			return msgMutate{err: err}
		}
		ref, err := newPlayerRef(ctx, pool, accountID)
		if err != nil {
			return msgMutate{err: err}
		}

		nodes := nodesForPreset(faction, preset)
		baseTags := progressionUnlockTags(cfg, targetTier)
		allTags := progressionReverseTags(baseTags, nodes)

		resetNodes, err := applyProgressionReverse(ctx, pool, ref, allTags, nodes)
		if err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: formatProgressionReverseSuccess(
			preset,
			faction,
			resetNodes,
			len(allTags),
		)}
	}
}

func cmdDeleteAllTutorials(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		_, err := pool.Exec(context.Background(),
			`SELECT dune.delete_all_tutorial_entries($1)`, playerID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("delete tutorials: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Deleted all tutorial entries for player %d", playerID)}
	}
}

func cmdWipeCodex(pool *pgxpool.Pool, accountID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if accountID == 0 {
			return msgMutate{err: fmt.Errorf("account ID required")}
		}
		_, err := pool.Exec(context.Background(),
			`SELECT dune.delete_mnemonic_recall_lesson_all($1)`, accountID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("wipe codex: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Wiped all codex entries for account %d", accountID)}
	}
}

const maxCharXP = int64(344440) // XP required for level 200 (hard cap)

// cumulativeXPByLevel[i] = total XP needed to reach level i (from SkillXPPerLevel.json).
var cumulativeXPByLevel = [201]int64{
	0, 40, 215, 440, 740, 1240, 1790, 2390, 2990, 3590, 4190, // 0-10
	4790, 5390, 5990, 6590, 7190, 7790, 8390, 8990, 9590, 10190, // 11-20
	10790, 11390, 11990, 12590, 13190, 13790, 14390, 14990, 15590, 16190, // 21-30
	16790, 17390, 17990, 18590, 19190, 19790, 20390, 20990, 21590, 22190, // 31-40
	22790, 23390, 23990, 24590, 25190, 25790, 26390, 26990, 27590, 28190, // 41-50
	28790, 29390, 29990, 30590, 31190, 31790, 32390, 32990, 33590, 34190, // 51-60
	34790, 35390, 35990, 36590, 37190, 37790, 38390, 38990, 39590, 40190, // 61-70
	40790, 41390, 41990, 42590, 43190, 43790, 44390, 44990, 45590, 46190, // 71-80
	46790, 47390, 47990, 48590, 49190, 49790, 50390, 50990, 51590, 52190, // 81-90
	52790, 53390, 53990, 54590, 55190, 55790, 56390, 56990, 57590, 58190, // 91-100
	58840, 59490, 60140, 60790, 61440, 62090, 62740, 63390, 64040, 64690, // 101-110
	65340, 65990, 66640, 67290, 67940, 68590, 69240, 69890, 70540, 71190, // 111-120
	71840, 72490, 73140, 73790, 74440, 75090, 75740, 76391, 77044, 77699, // 121-130
	78357, 79018, 79683, 80353, 81030, 81714, 82407, 83110, 83825, 84554, // 131-140
	85298, 86060, 86842, 87646, 88475, 89332, 90220, 91141, 92100, 93099, // 141-150
	94143, 95235, 96380, 97582, 98845, 100175, 101576, 103054, 104614, 106263, // 151-160
	108006, 109849, 111799, 113862, 116046, 118358, 120806, 123397, 126139, 129041, // 161-170
	132112, 135360, 138795, 142426, 146263, 150316, 154596, 159114, 163880, 168906, // 171-180
	174203, 179784, 185661, 191846, 198353, 205195, 212385, 219938, 227868, 236190, // 181-190
	244918, 254069, 263657, 273700, 284213, 295214, 306719, 318746, 331314, 344440, // 191-200
}

// xpToLevel returns the character level for the given cumulative XP (1–200).
func xpToLevel(xp int64) int {
	if xp <= 0 {
		return 0
	}
	lo, hi := 1, 200
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if cumulativeXPByLevel[mid] <= xp {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// maxIntelPoints is the most intel a character can spend — the cumulative total
// at the level cap. Granting beyond this (e.g. via battlepass) only accumulates
// unspendable points (#208), so grants are clamped to this ceiling.
const maxIntelPoints int64 = 2779

// intelGrantDelta returns how much intel to actually add so that current+delta
// never exceeds maxIntelPoints and never reduces an existing balance. Mirrors
// the SQL headroom clamp in cmdAwardIntelCtx and is the unit-tested source of
// truth for that arithmetic.
func intelGrantDelta(current, requested int64) int64 {
	headroom := maxIntelPoints - current
	if headroom <= 0 || requested <= 0 {
		return 0
	}
	return min(requested, headroom)
}

// intelSetValue returns the value a set-intel request actually writes: the
// requested value clamped to [0, maxIntelPoints]. Unlike intelGrantDelta this
// MAY reduce an existing balance — that is its purpose (#293 cleanup: intel
// over-granted by the battlepass incident is otherwise unremovable). Mirrors
// the SQL clamp in cmdSetIntelCtx and is the unit-tested source of truth.
func intelSetValue(requested int64) int64 {
	return min(max(requested, 0), maxIntelPoints)
}

// intelAtLevel returns cumulative intel points earned through a given level.
// Based on IntelPointsRewarded curve in SkillXPPerLevel.json:
//
//	L1=4, L2-3=+2, L4-15=+3, L16-30=+5, L31-50=+10,
//	L51-69=+20, L70-85=+30, L86-125=+40, L126+=0 (cap 2779)
func intelAtLevel(level int) int64 {
	switch {
	case level <= 0:
		return 0
	case level == 1:
		return 4
	case level <= 3:
		return 4 + int64(level-1)*2
	case level <= 15:
		return 8 + int64(level-3)*3
	case level <= 30:
		return 44 + int64(level-15)*5
	case level <= 50:
		return 119 + int64(level-30)*10
	case level <= 69:
		return 319 + int64(level-50)*20
	case level <= 85:
		return 699 + int64(level-69)*30
	case level <= 125:
		return 1179 + int64(level-85)*40
	default:
		return maxIntelPoints
	}
}

// errPlayerOnline classifies a checkPlayerOfflinePool failure as "the player
// is online" specifically, as opposed to a genuine error (DB failure, etc).
// Callers that can defer instead of failing outright — the battlepass
// auto-grant loop (#259/#280) — use errors.Is(err, errPlayerOnline) to retry
// on a short backoff without spending one of their limited attempts.
var errPlayerOnline = errors.New("player is online")

// playerOnlineError is returned by checkPlayerOfflinePool when the player is
// online. Its Error() text is the existing admin-facing message; Is lets
// callers classify it via errors.Is(err, errPlayerOnline) without changing
// that message (many call sites — giveItems, blueprints, welcome packages —
// surface .Error() directly to the operator).
type playerOnlineError struct{ status string }

// playerOnlineErrMarker is the stable prefix of playerOnlineError's message.
// battlepassStore.healExhaustedOnlineGrantLedger matches ledger rows on it to
// identify grants exhausted by the pre-#259/#280 policy — keep the two in
// sync via this constant, never by retyping the string.
const playerOnlineErrMarker = "player is currently"

func (e *playerOnlineError) Error() string {
	return fmt.Sprintf("%s %s — log out first, then apply the edit", playerOnlineErrMarker, e.status)
}

func (e *playerOnlineError) Is(target error) bool { return target == errPlayerOnline }

// checkPlayerOffline returns an error if the player is currently online.
// playerID is the pawn actor ID (PlayerCharacter).
func checkPlayerOffline(ctx context.Context, pool *pgxpool.Pool, playerID int64) error {
	return checkPlayerOfflinePool(ctx, pool, playerID)
}

// checkPlayerOfflinePool is the injectable (ctx+pool) form of checkPlayerOffline.
// checkPlayerOfflineSQL guards live-state writes. If duplicate player_state
// rows share a pawn id (#290), it must fail CLOSED: any non-Offline row sorts
// first (false < true in Postgres), so the guard sees "online" if any
// duplicate says so, instead of reading an arbitrary first row.
const checkPlayerOfflineSQL = `
	SELECT online_status::text FROM dune.player_state
	WHERE player_pawn_id = $1
	ORDER BY (online_status::text = 'Offline'), last_login_time DESC NULLS LAST
	LIMIT 1`

func checkPlayerOfflinePool(ctx context.Context, pool *pgxpool.Pool, playerID int64) error {
	var status string
	err := pool.QueryRow(ctx, checkPlayerOfflineSQL, playerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		// No player_state row means the player has never connected or their
		// session record was cleaned up — treat as offline.
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not check online status: %w", err)
	}
	if status != "Offline" {
		return &playerOnlineError{status: status}
	}
	return nil
}

type msgCharXP struct {
	xp    int64
	level int
	err   error
}

type charXPOutcome struct {
	newXP        int64
	newLevel     int64
	newTotalSP   int64
	newUnspentSP int64
	newIntel     int64
	capped       bool
}

func cmdFetchCharXP(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgCharXP{err: fmt.Errorf("not connected")}
		}
		var xp int64
		err := pool.QueryRow(context.Background(), `
			SELECT COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`, playerID).Scan(&xp)
		if err != nil {
			return msgCharXP{err: fmt.Errorf("read char xp: %w", err)}
		}
		return msgCharXP{xp: xp, level: xpToLevel(xp)}
	}
}

func readCharXPState(ctx context.Context, pool *pgxpool.Pool, playerID int64) (currentXP, spentSP int64, err error) {
	err = pool.QueryRow(ctx, `
		SELECT
			(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
			COALESCE((
				SELECT SUM((v->>'SkillPointsSpent')::int)
				FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
				WHERE k != format('(TagName="%s")',
					fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
			), 0)
		FROM dune.fgl_entities fe
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
		WHERE afe.actor_id = $1 AND afe.slot_name = 'DuneCharacter'`, playerID).Scan(&currentXP, &spentSP)
	if err != nil {
		return 0, 0, fmt.Errorf("read current state: %w", err)
	}
	return currentXP, spentSP, nil
}

func resolveControllerIDForPawn(ctx context.Context, pool *pgxpool.Pool, playerID int64) (int64, error) {
	var controllerID int64
	err := pool.QueryRow(ctx, `
		SELECT player_controller_id FROM dune.player_state
		WHERE player_pawn_id = $1 LIMIT 1`, playerID).Scan(&controllerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("resolve controller id: %w", err)
	}
	return controllerID, nil
}

func loadControllerKeystoneIDs(ctx context.Context, pool *pgxpool.Pool, controllerID int64) ([]int16, error) {
	if controllerID == 0 {
		return nil, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT keystone_id FROM dune.purchased_specialization_keystones
		WHERE player_id = $1::bigint`, controllerID)
	if err != nil {
		return nil, fmt.Errorf("read keystones: %w", err)
	}
	defer rows.Close()

	ids := make([]int16, 0, 8)
	for rows.Next() {
		var id int16
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan keystone: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan keystone: %w", err)
	}
	return ids, nil
}

func fetchKeystoneBonusForPawn(ctx context.Context, pool *pgxpool.Pool, playerID int64) (int64, error) {
	controllerID, err := resolveControllerIDForPawn(ctx, pool, playerID)
	if err != nil {
		return 0, err
	}
	ids, err := loadControllerKeystoneIDs(ctx, pool, controllerID)
	if err != nil {
		return 0, err
	}
	return keystoneSPBonus(ids), nil
}

func computeAwardCharXPOutcome(currentXP, spentSP, keystoneBonus, amount int64) charXPOutcome {
	newXP := min(currentXP+amount, maxCharXP)
	newLevel := int64(xpToLevel(newXP))
	newTotalSP := newLevel + keystoneBonus
	// Starter job always occupies 1 SP that is excluded from spentSP.
	newUnspentSP := max(newTotalSP-spentSP-1, 0)
	newIntel := intelAtLevel(int(newLevel))
	return charXPOutcome{
		newXP:        newXP,
		newLevel:     newLevel,
		newTotalSP:   newTotalSP,
		newUnspentSP: newUnspentSP,
		newIntel:     newIntel,
		capped:       newXP == maxCharXP,
	}
}

func applyAwardCharXPFLevelUpdate(ctx context.Context, pool *pgxpool.Pool, playerID int64, outcome charXPOutcome) error {
	_, err := pool.Exec(ctx, `
		UPDATE dune.fgl_entities
		SET components = jsonb_set(jsonb_set(jsonb_set(
			components,
			'{FLevelComponent,1,TotalXPEarned}',    to_jsonb($2::bigint)),
			'{FLevelComponent,1,TotalSkillPoints}',  to_jsonb($3::bigint)),
			'{FLevelComponent,1,UnspentSkillPoints}', to_jsonb($4::bigint))
		WHERE entity_id = (
			SELECT entity_id FROM dune.actor_fgl_entities
			WHERE actor_id = $1 AND slot_name = 'DuneCharacter'
		)`, playerID, outcome.newXP, outcome.newTotalSP, outcome.newUnspentSP)
	if err != nil {
		return fmt.Errorf("update fgl xp/sp: %w", err)
	}
	return nil
}

func applyAwardCharXPIntelUpdate(ctx context.Context, pool *pgxpool.Pool, playerID int64, newIntel int64) error {
	_, err := pool.Exec(ctx, `
		UPDATE dune.actors
		SET properties = jsonb_set(
			properties,
			'{TechKnowledgePlayerComponent,m_TechKnowledgePoints}',
			to_jsonb($2::bigint))
		WHERE id = $1 AND properties ? 'TechKnowledgePlayerComponent'`,
		playerID, newIntel)
	if err != nil {
		return fmt.Errorf("update intel: %w", err)
	}
	return nil
}

func formatAwardCharXPSuccess(playerID int64, outcome charXPOutcome, spentSP int64) string {
	capped := ""
	if outcome.capped {
		capped = " (capped at level 200)"
	}
	return fmt.Sprintf(
		"Player %d → level %d%s | XP %d | SP %d unspent (%d spent) | Intel %d",
		playerID, outcome.newLevel, capped, outcome.newXP, outcome.newUnspentSP, spentSP, outcome.newIntel)
}

func cmdAwardCharXP(pool *pgxpool.Pool, playerID int64, amount int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if playerID == 0 {
			return msgMutate{err: fmt.Errorf("player ID required")}
		}
		ctx := context.Background()
		if err := checkPlayerOfflinePool(ctx, pool, playerID); err != nil {
			return msgMutate{err: err}
		}

		currentXP, spentSP, err := readCharXPState(ctx, pool, playerID)
		if err != nil {
			return msgMutate{err: err}
		}

		keystoneBonus, err := fetchKeystoneBonusForPawn(ctx, pool, playerID)
		if err != nil {
			return msgMutate{err: err}
		}

		outcome := computeAwardCharXPOutcome(currentXP, spentSP, keystoneBonus, amount)

		// Update FLevelComponent: XP + both skill point fields.
		if err := applyAwardCharXPFLevelUpdate(ctx, pool, playerID, outcome); err != nil {
			return msgMutate{err: err}
		}

		// Update intel points on the PlayerCharacter actor.
		if err := applyAwardCharXPIntelUpdate(ctx, pool, playerID, outcome.newIntel); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: formatAwardCharXPSuccess(playerID, outcome, spentSP)}
	}
}

func cmdAwardIntel(pool *pgxpool.Pool, playerID int64, amount int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		if err := cmdAwardIntelCtx(context.Background(), pool, playerID, amount); err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf("Awarded %d intel points to player %d", amount, playerID)}
	}
}

// cmdAwardIntelCtx is the injectable (ctx+pool) form of cmdAwardIntel, used by
// the battlepass grant flow. The player must be offline — the game caches
// TechKnowledgePlayerComponent in memory and would clobber a live edit.
func cmdAwardIntelCtx(ctx context.Context, pool *pgxpool.Pool, playerID, amount int64) error {
	if playerID == 0 {
		return fmt.Errorf("player ID required")
	}
	if err := checkPlayerOfflinePool(ctx, pool, playerID); err != nil {
		return err
	}
	// Clamp to maxIntelPoints headroom so a grant never pushes the balance above
	// the spendable cap (#208). delta = current + min(max(amount,0),
	// max(cap-current,0)) — never reduces an existing balance, never exceeds cap.
	// Mirrors intelGrantDelta (unit-tested).
	res, err := pool.Exec(ctx, `
		UPDATE dune.actors
		SET properties = jsonb_set(
			properties,
			'{TechKnowledgePlayerComponent,m_TechKnowledgePoints}',
			to_jsonb(
				(properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::bigint
				+ LEAST(
					GREATEST($2::bigint, 0),
					GREATEST($3::bigint - (properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::bigint, 0)
				)
			)
		)
		WHERE id = $1
		  AND properties ? 'TechKnowledgePlayerComponent'`, playerID, amount, maxIntelPoints)
	if err != nil {
		return fmt.Errorf("award intel: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("TechKnowledgePlayerComponent not found for player %d — ensure player is a PlayerCharacter actor", playerID)
	}
	return nil
}

// cmdFetchIntelCtx returns the character's current intel points from the pawn
// actor's TechKnowledgePlayerComponent. Returns errNotFound when the actor has
// no such component (not a PlayerCharacter).
func cmdFetchIntelCtx(ctx context.Context, pool *pgxpool.Pool, playerID int64) (int64, error) {
	var intel int64
	err := pool.QueryRow(ctx, `
		SELECT (properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::bigint
		FROM dune.actors
		WHERE id = $1 AND properties ? 'TechKnowledgePlayerComponent'`, playerID).Scan(&intel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("fetch intel for player %d: %w", playerID, err)
	}
	return intel, nil
}

// cmdSetIntelCtx sets the character's intel points to an explicit value,
// clamped to [0, maxIntelPoints]. Unlike cmdAwardIntelCtx this may REDUCE the
// balance — the cleanup path for intel over-granted by the #293 battlepass
// incident. The player must be offline: the game caches
// TechKnowledgePlayerComponent in memory and would clobber a live edit.
// Mirrors intelSetValue (unit-tested).
func cmdSetIntelCtx(ctx context.Context, pool *pgxpool.Pool, playerID, value int64) error {
	if playerID == 0 {
		return fmt.Errorf("player ID required")
	}
	if err := checkPlayerOfflinePool(ctx, pool, playerID); err != nil {
		return err
	}
	res, err := pool.Exec(ctx, `
		UPDATE dune.actors
		SET properties = jsonb_set(
			properties,
			'{TechKnowledgePlayerComponent,m_TechKnowledgePoints}',
			to_jsonb(LEAST(GREATEST($2::bigint, 0), $3::bigint))
		)
		WHERE id = $1
		  AND properties ? 'TechKnowledgePlayerComponent'`, playerID, value, maxIntelPoints)
	if err != nil {
		return fmt.Errorf("set intel: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("TechKnowledgePlayerComponent not found for player %d — ensure player is a PlayerCharacter actor", playerID)
	}
	return nil
}

// intelAuditRow is one character in the intel audit: current intel next to the
// expected cumulative intel for the character's level.
type intelAuditRow struct {
	AccountID     int64  `json:"account_id"`
	PawnID        int64  `json:"pawn_id"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	Intel         int64  `json:"intel"`
	ExpectedIntel int64  `json:"expected_intel"`
	Online        bool   `json:"online"`
}

// intelAuditOverages keeps only the rows whose intel exceeds the expected
// value for their level, filling ExpectedIntel on every returned row. Pure —
// unit-tested separately from the query.
func intelAuditOverages(rows []intelAuditRow) []intelAuditRow {
	out := make([]intelAuditRow, 0)
	for _, r := range rows {
		r.ExpectedIntel = intelAtLevel(r.Level)
		if r.Intel > r.ExpectedIntel {
			out = append(out, r)
		}
	}
	return out
}

// cmdFetchIntelAuditRows returns every character whose intel exceeds the
// expected cumulative intel for their level (#293 cleanup: finds all
// mass-grant victims). Level derives from char XP; expected from intelAtLevel.
func cmdFetchIntelAuditRows(ctx context.Context, pool *pgxpool.Pool) ([]intelAuditRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT ps.account_id,
		       ps.player_pawn_id,
		       COALESCE(ps.character_name, ''),
		       (ps.online_status::text = 'Online'),
		       COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0),
		       COALESCE((a.properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::bigint, 0)
		FROM dune.player_state ps
		JOIN dune.actors a ON a.id = ps.player_pawn_id
		LEFT JOIN dune.actor_fgl_entities afe
		       ON afe.actor_id = ps.player_pawn_id AND afe.slot_name = 'DuneCharacter'
		LEFT JOIN dune.fgl_entities fe ON fe.entity_id = afe.entity_id
		WHERE ps.player_pawn_id IS NOT NULL
		  AND a.properties ? 'TechKnowledgePlayerComponent'
		  AND ps.account_id <> $1`, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("fetch intel audit rows: %w", err)
	}
	defer rows.Close()

	out := make([]intelAuditRow, 0)
	for rows.Next() {
		var r intelAuditRow
		var xp int64
		if err := rows.Scan(&r.AccountID, &r.PawnID, &r.Name, &r.Online, &xp, &r.Intel); err != nil {
			return nil, fmt.Errorf("scan intel audit row: %w", err)
		}
		r.Level = xpToLevel(xp)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return intelAuditOverages(out), nil
}

// ── blueprint JSON types ──────────────────────────────────────────────────────

type blueprintInstance struct {
	InstanceID        *int    `json:"instance_id,omitempty"`
	BuildingType      string  `json:"building_type"`
	X                 float64 `json:"x"`
	Y                 float64 `json:"y"`
	Z                 float64 `json:"z"`
	Rotation          float64 `json:"rotation"`
	ProvidesStability *bool   `json:"provides_stability,omitempty"`
}

type blueprintPlaceable struct {
	PlaceableID  *int    `json:"placeable_id,omitempty"`
	BuildingType string  `json:"building_type"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	RX           float64 `json:"rx,omitempty"`
	RY           float64 `json:"ry"`
	RZ           float64 `json:"rz,omitempty"`
}

type blueprintPentashield struct {
	PlaceableID int    `json:"placeable_id"`
	Scale       [3]int `json:"scale"` // [width, height, depth] stored as SMALLINT[3]
}

type blueprintFile struct {
	Name         string                 `json:"name,omitempty"`
	Instances    []blueprintInstance    `json:"instances"`
	Placeables   []blueprintPlaceable   `json:"placeables"`
	Pentashields []blueprintPentashield `json:"pentashields,omitempty"`
}

// ── blueprint commands ────────────────────────────────────────────────────────

func cmdListBlueprints(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgBlueprintList{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT bb.id,
		       COALESCE(ps.character_name, '') AS owner,
		       COALESCE(bb.item_id, 0),
		       COALESCE(inst.cnt, 0) AS pieces,
		       COALESCE(plac.cnt, 0) AS placeables,
		       COALESCE(i.stats->'FBuildingBlueprintItemStats'->1->>'BuildingBlueprintName', '') AS name
		FROM dune.building_blueprints bb
		LEFT JOIN dune.items i ON i.id = bb.item_id
		LEFT JOIN dune.inventories inv ON inv.id = i.inventory_id
		LEFT JOIN dune.actors a ON a.id = inv.actor_id
		LEFT JOIN dune.player_state ps ON ps.player_pawn_id = a.id
		LEFT JOIN (
		    SELECT building_blueprint_id, COUNT(*) AS cnt
		    FROM dune.building_blueprint_instances
		    GROUP BY building_blueprint_id
		) inst ON inst.building_blueprint_id = bb.id
		LEFT JOIN (
		    SELECT building_blueprint_id, COUNT(*) AS cnt
		    FROM dune.building_blueprint_placeables
		    GROUP BY building_blueprint_id
		) plac ON plac.building_blueprint_id = bb.id
		ORDER BY bb.id`)
	if err != nil {
		return msgBlueprintList{err: err}
	}
	defer rows.Close()
	var out []blueprintRow
	for rows.Next() {
		var r blueprintRow
		if err := rows.Scan(&r.ID, &r.OwnerName, &r.ItemID, &r.Pieces, &r.Placeables, &r.Name); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgBlueprintList{err: err}
	}
	return msgBlueprintList{rows: out}
}

func cmdGrantMaxSpec(pool *pgxpool.Pool, playerID int64, trackType string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		_, err := pool.Exec(context.Background(),
			`SELECT dune.set_specialization_xp_and_level($1, $2::dune.specializationtracktype, $3, $4)`,
			playerID, trackType, 44182, 100.0)
		if err != nil {
			return msgMutate{err: err}
		}
		return msgMutate{ok: fmt.Sprintf("Granted max %s spec to player %d", trackType, playerID)}
	}
}

func cmdFetchPlayerSpecs(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgSpecs{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT player_id, track_type::text, xp_amount, level
			FROM dune.specialization_tracks
			WHERE player_id = $1::bigint
			ORDER BY track_type`, playerID)
		if err != nil {
			return msgSpecs{err: err}
		}
		defer rows.Close()
		var out []specTrack
		for rows.Next() {
			var r specTrack
			if err := rows.Scan(&r.PlayerID, &r.TrackType, &r.XP, &r.Level); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgSpecs{err: err}
		}
		return msgSpecs{rows: out}
	}
}

// keystoneSPBonus returns the total extra skill points granted by a set of keystone IDs.
// SkillPoint = +1, SkillPoint_Major = +3, SkillPoint_Super = +5 (Combat track only).
func keystoneSPBonus(ids []int16) int64 {
	var total int64
	for _, id := range ids {
		info, ok := keystoneMap[id]
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(info.Name, "_SkillPoint_Super"):
			total += 5
		case strings.HasSuffix(info.Name, "_SkillPoint_Major"):
			total += 3
		case strings.HasSuffix(info.Name, "_SkillPoint"):
			total += 1
		}
	}
	return total
}

func insertAllPurchasedKeystones(ctx context.Context, pool *pgxpool.Pool, playerID int64) error {
	_, err := pool.Exec(ctx, `
			INSERT INTO dune.purchased_specialization_keystones (player_id, keystone_id)
			SELECT $1::bigint, generate_series(1, 205)
			ON CONFLICT DO NOTHING`, playerID)
	return err
}

func allKeystoneIDs() []int16 {
	ids := make([]int16, 205)
	for i := range ids {
		ids[i] = int16(i + 1)
	}
	return ids
}

func readLevelComponentSkillState(ctx context.Context, pool *pgxpool.Pool, playerID int64) (int64, int64, int64, error) {
	var xp, currentTotal, spentSP int64
	err := pool.QueryRow(ctx, `
			SELECT
				(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
				(fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::bigint,
				COALESCE((
					SELECT SUM((v->>'SkillPointsSpent')::int)
					FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
					WHERE k != format('(TagName="%s")',
						fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
				), 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.slot_name = 'DuneCharacter'
			  AND afe.actor_id = (
				SELECT player_pawn_id FROM dune.player_state
				WHERE player_controller_id = $1 LIMIT 1
			  )`, playerID).Scan(&xp, &currentTotal, &spentSP)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read FLevelComponent: %w", err)
	}
	return xp, currentTotal, spentSP, nil
}

func grantAllKeystoneTargets(xp, spentSP int64) (int64, int64, int64) {
	keystoneBonus := keystoneSPBonus(allKeystoneIDs())
	level := int64(xpToLevel(xp))
	expectedTotal := level + keystoneBonus
	// UnspentSkillPoints = total - non-starter spent - 1 (starter job always occupies 1 SP).
	expectedUnspent := max(expectedTotal-spentSP-1, 0)
	return expectedTotal, expectedUnspent, keystoneBonus
}

func updateLevelComponentSkillPoints(ctx context.Context, pool *pgxpool.Pool, playerID, expectedTotal, expectedUnspent int64) error {
	_, err := pool.Exec(ctx, `
			UPDATE dune.fgl_entities
			SET components = jsonb_set(jsonb_set(
				components,
				'{FLevelComponent,1,TotalSkillPoints}',
				to_jsonb($2::bigint)),
				'{FLevelComponent,1,UnspentSkillPoints}',
				to_jsonb($3::bigint))
			WHERE entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE slot_name = 'DuneCharacter'
				  AND actor_id = (
					SELECT player_pawn_id FROM dune.player_state
					WHERE player_controller_id = $1 LIMIT 1
				  )
			)`, playerID, expectedTotal, expectedUnspent)
	if err != nil {
		return fmt.Errorf("update skill points: %w", err)
	}
	return nil
}

func cmdGrantAllKeystones(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		if err := checkPlayerOffline(ctx, pool, playerID); err != nil {
			return msgMutate{err: err}
		}

		if err := insertAllPurchasedKeystones(ctx, pool, playerID); err != nil {
			return msgMutate{err: err}
		}

		// Read XP, current TotalSkillPoints, and SP spent in non-starter modules.
		// Uses pawn actor id (purchased_specialization_keystones uses controller id).
		xp, currentTotal, spentSP, err := readLevelComponentSkillState(ctx, pool, playerID)
		if err != nil {
			return msgMutate{err: err}
		}

		expectedTotal, expectedUnspent, keystoneBonus := grantAllKeystoneTargets(xp, spentSP)

		if currentTotal >= expectedTotal {
			return msgMutate{ok: fmt.Sprintf(
				"Granted all keystones to player %d — SP already correct (%d total, %d unspent)",
				playerID, currentTotal, expectedUnspent)}
		}

		if err := updateLevelComponentSkillPoints(ctx, pool, playerID, expectedTotal, expectedUnspent); err != nil {
			return msgMutate{err: err}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Granted all keystones to player %d — SP %d → %d total, %d unspent (+%d keystone bonus)",
			playerID, currentTotal, expectedTotal, expectedUnspent, keystoneBonus)}
	}
}

// cmdResetAllKeystones is the inverse of cmdGrantAllKeystones: it deletes all
// purchased keystones and rolls TotalSkillPoints/UnspentSkillPoints back to
// the XP-derived baseline (no keystone bonus). Requires the player to be offline.
func cmdResetAllKeystones(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}

		ctx := context.Background()

		if err := checkPlayerOffline(ctx, pool, playerID); err != nil {
			return msgMutate{err: err}
		}

		// Read XP, current total SP, and non-starter spent SP — same query as
		// cmdGrantAllKeystones so the arithmetic is symmetric.
		var xp, currentTotal, spentSP int64
		err := pool.QueryRow(ctx, `
			SELECT
				(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
				(fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::bigint,
				COALESCE((
					SELECT SUM((v->>'SkillPointsSpent')::int)
					FROM jsonb_each(fe.components->'FLevelComponent'->1->'ModuleData') AS kv(k, v)
					WHERE k != format('(TagName="%s")',
						fe.components->'FLevelComponent'->1->'StarterSkillTreeTag'->>'TagName')
				), 0)
			FROM dune.fgl_entities fe
			JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
			WHERE afe.slot_name = 'DuneCharacter'
			  AND afe.actor_id = (
				SELECT player_pawn_id FROM dune.player_state
				WHERE player_controller_id = $1 LIMIT 1
			  )`, playerID).Scan(&xp, &currentTotal, &spentSP)
		if err != nil {
			return msgMutate{err: fmt.Errorf("read FLevelComponent: %w", err)}
		}

		if _, err := pool.Exec(ctx,
			`DELETE FROM dune.purchased_specialization_keystones WHERE player_id = $1`,
			playerID); err != nil {
			return msgMutate{err: fmt.Errorf("delete keystones: %w", err)}
		}

		level := int64(xpToLevel(xp))
		newTotal := level
		newUnspent := max(newTotal-spentSP-1, 0)

		if _, err := pool.Exec(ctx, `
			UPDATE dune.fgl_entities
			SET components = jsonb_set(jsonb_set(
				components,
				'{FLevelComponent,1,TotalSkillPoints}',
				to_jsonb($2::bigint)),
				'{FLevelComponent,1,UnspentSkillPoints}',
				to_jsonb($3::bigint))
			WHERE entity_id = (
				SELECT entity_id FROM dune.actor_fgl_entities
				WHERE slot_name = 'DuneCharacter'
				  AND actor_id = (
					SELECT player_pawn_id FROM dune.player_state
					WHERE player_controller_id = $1 LIMIT 1
				  )
			)`, playerID, newTotal, newUnspent); err != nil {
			return msgMutate{err: fmt.Errorf("update skill points: %w", err)}
		}

		return msgMutate{ok: fmt.Sprintf(
			"Reset all keystones for player %d — SP %d → %d total, %d unspent",
			playerID, currentTotal, newTotal, newUnspent)}
	}
}

func cmdFetchPlayerKeystones(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgKeystones{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT keystone_id FROM dune.purchased_specialization_keystones
			WHERE player_id = $1::bigint ORDER BY keystone_id`, playerID)
		if err != nil {
			return msgKeystones{err: err}
		}
		defer rows.Close()
		var ids []int16
		for rows.Next() {
			var id int16
			if err := rows.Scan(&id); err != nil {
				continue
			}
			ids = append(ids, id)
		}
		return msgKeystones{ids: ids}
	}
}

func cmdGetPlayerVehicles(pool *pgxpool.Pool, controllerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgVehicles{err: fmt.Errorf("not connected")}
		}
		// Look up account_id from controller_id — vehicle actors don't use owner_account_id.
		var accountID int64
		err := pool.QueryRow(context.Background(),
			`SELECT ps.account_id FROM dune.player_state ps WHERE ps.player_controller_id = $1 LIMIT 1`,
			controllerID).Scan(&accountID)
		if err != nil {
			return msgVehicles{err: fmt.Errorf("look up account: %w", err)}
		}

		// recovered_vehicles / backup_vehicles migrated account_id -> character_id
		// (issue #267); resolve the key (both tables migrated together).
		keyCol, keyVal, err := playerKeyFor(context.Background(), pool, "recovered_vehicles", accountID)
		if err != nil {
			return msgVehicles{err: err}
		}
		// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
		rows, err := pool.Query(context.Background(), fmt.Sprintf(`
			SELECT pa.actor_id, a.class, COALESCE(a.map, ''),
			       COALESCE(rv.chassis_durability::float8, 1.0),
			       COALESCE(pa.actor_name, rv.vehicle_name, ''),
			       (rv.vehicle_id IS NOT NULL) AS is_recovered,
			       false AS is_backup
			FROM dune.permission_actor pa
			JOIN dune.permission_actor_rank par ON par.permission_actor_id = pa.actor_id
			JOIN dune.actors a ON a.id = pa.actor_id
			LEFT JOIN dune.recovered_vehicles rv ON rv.vehicle_id = pa.actor_id AND rv.%[1]s = $2
			WHERE par.player_id = $1 AND pa.actor_type = 2

			UNION ALL

			SELECT a.id, a.class, '' AS map,
			       1.0 AS chassis_durability,
			       '' AS vehicle_name,
			       false AS is_recovered,
			       true AS is_backup
			FROM dune.backup_vehicles bv
			JOIN dune.actors a ON a.id = bv.vehicle_id
			WHERE bv.%[1]s = $2

			ORDER BY class`, keyCol), controllerID, keyVal)
		if err != nil {
			return msgVehicles{err: err}
		}
		defer rows.Close()
		var out []vehicleRow
		for rows.Next() {
			var r vehicleRow
			if err := rows.Scan(&r.ID, &r.Class, &r.Map, &r.ChassisDurability, &r.VehicleName, &r.IsRecovered, &r.IsBackup); err != nil {
				continue
			}
			r.Class = shortClass(r.Class)
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgVehicles{err: err}
		}
		return msgVehicles{rows: out}
	}
}

func lookupRepairItemOwner(ctx context.Context, pool *pgxpool.Pool, itemID int64) (int64, error) {
	var pawnID int64
	err := pool.QueryRow(ctx, `
			SELECT inv.actor_id
			FROM dune.items i
			JOIN dune.inventories inv ON inv.id = i.inventory_id
			WHERE i.id = $1::bigint`, itemID).Scan(&pawnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("item %d not found", itemID)
	}
	if err != nil {
		return 0, fmt.Errorf("look up item owner: %w", err)
	}
	return pawnID, nil
}

func repairItemDurability(ctx context.Context, pool *pgxpool.Pool, itemID int64) (int64, error) {
	res, err := pool.Exec(ctx, `
			UPDATE dune.items i
			SET stats = jsonb_set(
				jsonb_set(i.stats,
					'{FItemStackAndDurabilityStats,1,CurrentDurability}',
					to_jsonb(t.val), true),
				'{FItemStackAndDurabilityStats,1,DecayedMaxDurability}',
				to_jsonb(t.val), true)
			FROM (
				SELECT COALESCE(
					(stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability')::float8,
					100.0
				) AS val
				FROM dune.items
				WHERE id = $1::bigint
				  AND stats ? 'FItemStackAndDurabilityStats'
			) AS t
			WHERE i.id = $1::bigint
			  AND (
				abs(COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability')::float8, 0) - t.val) > 0.01
				OR abs(COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'DecayedMaxDurability')::float8, 0) - t.val) > 0.01
			  )`, itemID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func itemHasDurabilityStats(ctx context.Context, pool *pgxpool.Pool, itemID int64) (bool, error) {
	var hasDurability bool
	if err := pool.QueryRow(ctx, `
				SELECT stats ? 'FItemStackAndDurabilityStats'
				FROM dune.items WHERE id = $1::bigint`, itemID).Scan(&hasDurability); err != nil {
		return false, fmt.Errorf("check item: %w", err)
	}
	return hasDurability, nil
}

func repairItemNoChangeMessage(itemID int64, hasDurability bool) msgMutate {
	if !hasDurability {
		return msgMutate{err: fmt.Errorf("item %d has no durability field", itemID)}
	}
	return msgMutate{ok: fmt.Sprintf("Item %d already at full durability", itemID)}
}

func repairItemSuccessMessage(itemID int64) msgMutate {
	return msgMutate{ok: fmt.Sprintf("Repaired item %d — relog to see in-game", itemID)}
}

func cmdRepairItem(pool *pgxpool.Pool, itemID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		// Derive owning player from item → inventory → actor (pawn) so we can gate Offline.
		pawnID, err := lookupRepairItemOwner(ctx, pool, itemID)
		if err != nil {
			return msgMutate{err: err}
		}
		if err := checkPlayerOffline(ctx, pool, pawnID); err != nil {
			return msgMutate{err: err}
		}

		// Write both fields: Current-only gets clamped to surviving Decayed on reload.
		// Fallback target = 100.0 covers the 0-100 gear scale when MaxDurability is absent.
		rowsAffected, err := repairItemDurability(ctx, pool, itemID)
		if err != nil {
			return msgMutate{err: fmt.Errorf("repair item: %w", err)}
		}
		if rowsAffected == 0 {
			// Item exists (owner lookup succeeded). Either no durability field, or already at ceiling.
			hasDurability, err := itemHasDurabilityStats(ctx, pool, itemID)
			if err != nil {
				return msgMutate{err: err}
			}
			return repairItemNoChangeMessage(itemID, hasDurability)
		}
		return repairItemSuccessMessage(itemID)
	}
}

// cmdUpdateItem edits an existing item's stack size and quality grade
// directly (#256) — the only prior edit paths were Repair (durability only)
// and Delete; stack/quality were otherwise read-only in the admin UI despite
// the DB happily storing any value. Offline-gated like Repair since the game
// server owns the live copy while a player is connected.
func cmdUpdateItem(pool *pgxpool.Pool, itemID, stackSize, quality int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		pawnID, err := lookupRepairItemOwner(ctx, pool, itemID)
		if err != nil {
			return msgMutate{err: err}
		}
		if err := checkPlayerOffline(ctx, pool, pawnID); err != nil {
			return msgMutate{err: err}
		}

		res, err := pool.Exec(ctx, `
			UPDATE dune.items SET stack_size = $2::bigint, quality_level = $3::bigint
			WHERE id = $1::bigint`, itemID, stackSize, quality)
		if err != nil {
			return msgMutate{err: fmt.Errorf("update item: %w", err)}
		}
		if res.RowsAffected() == 0 {
			return msgMutate{err: fmt.Errorf("item %d not found", itemID)}
		}
		return msgMutate{ok: fmt.Sprintf("Updated item %d — relog to see in-game", itemID)}
	}
}

// Carried inventories: backpack, equipment, emote wheel, equipped weapons, action wheel, bank.
var repairGearInventoryTypes = []int32{0, 1, 14, 15, 27, 30}

type repairCandidate struct {
	id     int64
	target float64
}

func parseDurabilityText(value pgtype.Text) float64 {
	if !value.Valid {
		return 0
	}
	parsed, _ := strconv.ParseFloat(value.String, 64)
	return parsed
}

func repairTargetForItem(maxDurability pgtype.Text) float64 {
	// The in-row MaxDurability is the source of truth. Default to 100 only for
	// plain 0–100 gear that carries no MaxDurability. The PAK catalog is NOT
	// consulted — it under-reports some scales (e.g. transport modules).
	if maxDurability.Valid {
		if value, err := strconv.ParseFloat(maxDurability.String, 64); err == nil && value > 0 {
			return value
		}
	}
	return 100.0
}

func buildRepairCandidate(
	id int64,
	maxDurability, currentDurability, decayedDurability pgtype.Text,
) (repairCandidate, bool) {
	current := parseDurabilityText(currentDurability)
	decayed := parseDurabilityText(decayedDurability)
	// Never lower an existing value: a default-100 must not cap a higher-scale
	// item whose MaxDurability is absent but whose stored values exceed 100.
	target := repairTargetForItem(maxDurability)
	if current > target {
		target = current
	}
	if decayed > target {
		target = decayed
	}
	if math.Abs(current-target) < 0.01 && math.Abs(decayed-target) < 0.01 {
		return repairCandidate{}, false
	}
	return repairCandidate{id: id, target: target}, true
}

func loadPlayerGearRepairCandidates(ctx context.Context, pool *pgxpool.Pool, playerID int64) ([]repairCandidate, int, error) {
	rows, err := pool.Query(ctx, `
		SELECT i.id,
		       (i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'),
		       (i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'),
		       (i.stats->'FItemStackAndDurabilityStats'->1->>'DecayedMaxDurability')
		FROM dune.items i
		JOIN dune.inventories inv ON inv.id = i.inventory_id
		WHERE inv.actor_id = $1::bigint
		  AND inv.inventory_type = ANY($2::int[])
		  AND i.stats ? 'FItemStackAndDurabilityStats'`,
		playerID, repairGearInventoryTypes)
	if err != nil {
		return nil, 0, fmt.Errorf("scan items: %w", err)
	}
	defer rows.Close()

	toRepair := make([]repairCandidate, 0, 64)
	scanned := 0
	for rows.Next() {
		scanned++

		var id int64
		var maxDurability, currentDurability, decayedDurability pgtype.Text
		if err := rows.Scan(&id, &maxDurability, &currentDurability, &decayedDurability); err != nil {
			return nil, scanned, fmt.Errorf("scan item: %w", err)
		}

		candidate, needsRepair := buildRepairCandidate(id, maxDurability, currentDurability, decayedDurability)
		if needsRepair {
			toRepair = append(toRepair, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan rows: %w", err)
	}

	return toRepair, scanned, nil
}

func validateRepairPlayerGearInput(pool *pgxpool.Pool, playerID int64) error {
	if pool == nil {
		return fmt.Errorf("not connected")
	}
	if playerID == 0 {
		return fmt.Errorf("player ID required")
	}
	return nil
}

type gearRepairRunResult struct {
	repaired int
	err      error
}

func runPlayerGearRepairs(ctx context.Context, pool *pgxpool.Pool, toRepair []repairCandidate) gearRepairRunResult {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return gearRepairRunResult{err: fmt.Errorf("begin tx: %w", err)}
	}
	defer func() { _ = tx.Rollback(ctx) }()

	repaired := 0
	for _, rc := range toRepair {
		_, err := tx.Exec(ctx, `
			UPDATE dune.items
			SET stats = jsonb_set(
				jsonb_set(stats,
					'{FItemStackAndDurabilityStats,1,CurrentDurability}',
					to_jsonb($2::float8), true),
				'{FItemStackAndDurabilityStats,1,DecayedMaxDurability}',
				to_jsonb($2::float8), true)
			WHERE id = $1::bigint`, rc.id, rc.target)
		if err != nil {
			return gearRepairRunResult{
				repaired: repaired,
				err:      fmt.Errorf("repair item %d: %w", rc.id, err),
			}
		}
		repaired++
	}
	if err := tx.Commit(ctx); err != nil {
		// Keep the legacy response shape for commit failures: no repaired count.
		return gearRepairRunResult{err: fmt.Errorf("commit: %w", err)}
	}
	return gearRepairRunResult{repaired: repaired}
}

func cmdRepairPlayerGear(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if err := validateRepairPlayerGearInput(pool, playerID); err != nil {
			return msgRepairGear{err: err}
		}
		ctx := context.Background()
		if err := checkPlayerOffline(ctx, pool, playerID); err != nil {
			return msgRepairGear{err: err}
		}

		toRepair, scanned, err := loadPlayerGearRepairCandidates(ctx, pool, playerID)
		if err != nil {
			return msgRepairGear{scanned: scanned, err: err}
		}

		run := runPlayerGearRepairs(ctx, pool, toRepair)
		if run.err != nil {
			return msgRepairGear{repaired: run.repaired, scanned: scanned, err: run.err}
		}
		return msgRepairGear{repaired: run.repaired, scanned: scanned}
	}
}

func cmdFetchCheatLog(pool *pgxpool.Pool) Cmd {
	return func() Msg {
		if pool == nil {
			return msgCheatLog{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT ct.fls_id, ct.cheat_type::text,
			       to_char(ct.event_time AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
			       COALESCE(ps.character_name, ct.fls_id)
			FROM dune.cheater_tracking ct
			LEFT JOIN dune.encrypted_accounts e ON convert_from(e.encrypted_funcom_id, 'UTF8') = ct.fls_id
			LEFT JOIN dune.player_state ps ON ps.account_id = e.id
			WHERE ct.event_time > NOW() - INTERVAL '7 days'
			ORDER BY ct.event_time DESC
			LIMIT 500`)
		if err != nil {
			return msgCheatLog{err: err}
		}
		defer rows.Close()
		var out []cheatEntry
		for rows.Next() {
			var r cheatEntry
			if err := rows.Scan(&r.FLSID, &r.CheatType, &r.EventTime, &r.CharacterName); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgCheatLog{err: err}
		}
		return msgCheatLog{rows: out}
	}
}

func cmdFetchEventLog(pool *pgxpool.Pool, actorID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgEvents{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT actor_id,
			       to_char(universe_time AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'),
			       COALESCE(map, ''),
			       event_type,
			       COALESCE(x, 0)::float8, COALESCE(y, 0)::float8, COALESCE(z, 0)::float8,
			       COALESCE(custom_data::text, '{}')
			FROM dune.game_events
			WHERE actor_id = $1::bigint AND player_facing_event = true
			ORDER BY universe_time DESC
			LIMIT 200`, actorID)
		if err != nil {
			return msgEvents{err: err}
		}
		defer rows.Close()
		var out []gameEvent
		for rows.Next() {
			var r gameEvent
			if err := rows.Scan(&r.ActorID, &r.UniverseTime, &r.Map, &r.EventType, &r.X, &r.Y, &r.Z, &r.CustomData); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgEvents{err: err}
		}
		return msgEvents{rows: out}
	}
}

func cmdFetchPlayerDungeons(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgDungeons{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT dc.dungeon_id, dc.difficulty::text, dc.duration_ms, dc.players_num, dc.completion_id
			FROM dune.dungeon_completion_players dcp
			JOIN dune.dungeon_completion dc ON dc.completion_id = dcp.completion_id
			WHERE dcp.player_id = $1::bigint
			ORDER BY dc.completion_id DESC
			LIMIT 100`, playerID)
		if err != nil {
			return msgDungeons{err: err}
		}
		defer rows.Close()
		var out []dungeonRecord
		for rows.Next() {
			var r dungeonRecord
			if err := rows.Scan(&r.DungeonID, &r.Difficulty, &r.DurationMs, &r.PlayersNum, &r.CompletionID); err != nil {
				continue
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return msgDungeons{err: err}
		}
		return msgDungeons{rows: out}
	}
}

var cheatLocations = []teleportLocation{
	{Name: "Windsack", X: 974276.75, Y: 20084.312, Z: 5112.283},
	{Name: "EcoLabs", X: 826879.3, Y: -925967.2, Z: 4974.4277},
	{Name: "CrashSite", X: 330284.22, Y: 205236.98, Z: 2251.008},
	{Name: "MediumStarter", X: 268515.8, Y: 207559.39, Z: 5000.0},
	{Name: "ConvoyAmbush", X: -920080.0, Y: 909620.0, Z: 300.0},
	{Name: "SpiceRaid", X: 271590.0, Y: -493122.0, Z: 8471.0},
	{Name: "PS5_ESW_0", X: -113881.4, Y: -305252.1, Z: 20864.5},
	{Name: "PS5_ESW_1", X: -109861.8, Y: -307020.0, Z: 21192.9},
	{Name: "PS5_ESW_2", X: -129029.6, Y: -312757.8, Z: 21099.6},
	{Name: "PS5_ESW_3", X: -117312.0, Y: -305453.9, Z: 21649.8},
}

func cmdListPartitions(pool *pgxpool.Pool) Cmd {
	return func() Msg {
		if pool == nil {
			return msgPartitions{err: fmt.Errorf("not connected")}
		}
		if globalLocationStore != nil {
			locs, err := globalLocationStore.list()
			if err == nil {
				return msgPartitions{rows: locs}
			}
		}
		// Fallback to compile-time seeds when the store is unavailable.
		return msgPartitions{rows: cheatLocations}
	}
}

func cmdTeleportPlayer(pool *pgxpool.Pool, flsID string, locationName string) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		loc, err := resolveLocation(locationName)
		if err != nil {
			return msgMutate{err: err}
		}
		ctx := context.Background()
		// Use the player's current partition so the zone server is correct.
		var partitionID int64
		if scanErr := pool.QueryRow(ctx, `
			SELECT COALESCE(a.partition_id, 0)
			FROM dune.encrypted_accounts e
			JOIN dune.player_state ps ON ps.account_id = e.id
			JOIN dune.actors a ON a.id = ps.player_pawn_id
			WHERE convert_from(e.encrypted_funcom_id, 'UTF8') = $1`, flsID).Scan(&partitionID); scanErr != nil || partitionID == 0 {
			_ = pool.QueryRow(ctx,
				`SELECT id FROM dune.world_partition WHERE blocked = false LIMIT 1`).Scan(&partitionID)
		}
		if _, execErr := pool.Exec(ctx, `
			SELECT dune.admin_move_offline_player_to_partition($1::text, $2::bigint, ROW($3::float8,$4::float8,$5::float8)::dune.Vector)`,
			flsID, partitionID, loc.X, loc.Y, loc.Z); execErr != nil {
			return msgMutate{err: fmt.Errorf("teleport: %w", execErr)}
		}
		return msgMutate{ok: fmt.Sprintf("Moved %s to %s", flsID, locationName)}
	}
}

// playerPosition is the live world position of a player's character.
type playerPosition struct {
	PartitionID int64   `json:"partition_id"`
	Map         string  `json:"map"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
}

// cmdGetPlayerPosition reads a player's current world position from the
// actors table. The transform column is a composite type holding a vector
// location plus a quaternion rotation; we only need the vector.
// playerID is the actor id (dune.actors.id) — matches playerInfo.ID.
func cmdGetPlayerPosition(pool *pgxpool.Pool, playerID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgPlayerPosition{err: fmt.Errorf("not connected")}
		}
		var pos playerPosition
		err := pool.QueryRow(context.Background(), `
			SELECT
				COALESCE(a.partition_id, 0),
				COALESCE(a.map, ''),
				((a.transform).location).x,
				((a.transform).location).y,
				((a.transform).location).z
			FROM dune.actors a
			WHERE a.id = $1`, playerID).Scan(&pos.PartitionID, &pos.Map, &pos.X, &pos.Y, &pos.Z)
		if err != nil {
			return msgPlayerPosition{err: fmt.Errorf("read position: %w", err)}
		}
		return msgPlayerPosition{pos: pos}
	}
}

// cmdTeleportPlayerToCoords moves an offline player to a specific
// (partition_id, x, y, z). For online players this should be skipped in
// favour of rmqTeleportTo, which has immediate effect.
func cmdTeleportPlayerToCoords(pool *pgxpool.Pool, flsID string, partitionID int64, x, y, z float64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()
		if partitionID == 0 {
			// Try the player's current partition first (most likely to be valid).
			_ = pool.QueryRow(ctx, `
				SELECT COALESCE(a.partition_id, 0)
				FROM dune.accounts ac
				JOIN dune.player_state ps ON ps.account_id = ac.id
				JOIN dune.actors a ON a.id = ps.player_pawn_id
				WHERE ac."user" = $1`, flsID).Scan(&partitionID)
		}
		if partitionID == 0 {
			// Fall back to any non-blocked partition (handles offline/no-actor state).
			_ = pool.QueryRow(ctx,
				`SELECT id FROM dune.world_partition WHERE blocked = false ORDER BY id LIMIT 1`,
			).Scan(&partitionID)
		}
		if partitionID == 0 {
			return msgMutate{err: fmt.Errorf("could not resolve a valid partition for teleport")}
		}
		if _, execErr := pool.Exec(ctx, `
			SELECT dune.admin_move_offline_player_to_partition($1::text, $2::bigint, ROW($3::float8,$4::float8,$5::float8)::dune.Vector)`,
			flsID, partitionID, x, y, z); execErr != nil {
			return msgMutate{err: fmt.Errorf("teleport: %w", execErr)}
		}
		return msgMutate{ok: fmt.Sprintf("Moved %s to (%.0f, %.0f, %.0f)", flsID, x, y, z)}
	}
}

// ── live map commands ─────────────────────────────────────────────────────────

// liveMapKeys is the allow-list of maps the Live Map supports (v1: open-world
// only). The value is the gameplay partition index for that map — not used by
// the read query (which filters by a.map) but kept here as the single source of
// truth for teleport routing (Phase 3) and to gate caller-supplied input.
var liveMapKeys = map[string]int64{
	"HaggaBasin": 1,
	"DeepDesert": 8,
}

// cmdFetchDistinctMaps returns all distinct non-empty map names from dune.actors,
// sorted alphabetically.
func cmdFetchDistinctMaps(ctx context.Context, db *pgxpool.Pool) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT DISTINCT COALESCE(map, '')
		FROM dune.actors
		WHERE map IS NOT NULL AND map != ''
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("fetch distinct maps: %w", err)
	}
	defer rows.Close()
	var maps []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("scan map: %w", err)
		}
		maps = append(maps, m)
	}
	if maps == nil {
		maps = []string{}
	}
	return maps, nil
}

// validateMapKey rejects any map the Live Map does not support, so caller input
// can never reach the query as an unexpected value.
func validateMapKey(key string) error {
	if key == "" {
		return fmt.Errorf("map required")
	}
	if _, ok := liveMapKeys[key]; !ok {
		return fmt.Errorf("unsupported map: %q", key)
	}
	return nil
}

// dimensionFilterSQL builds the optional dimension_index filter shared by
// cmdFetchMapMarkers/cmdFetchBaseMarkers (#274). A nil dimension means "all
// dimensions" — the pre-#274 behaviour, kept for backward compatibility — and
// returns no clause/args. A non-nil dimension (including the zero value, which
// is a real dimension, not "unset") returns a clause bound to $2, since $1 is
// always the map key in every caller. alias lets the same helper serve both
// the actors alias ("a") and the base-markers totem alias ("t").
//
// The comparison goes through COALESCE(dimension_index, 0) rather than a bare
// `= $2`, matching the COALESCE(..., 0) already used when the column is
// displayed (mapMarker.DimensionIndex) and by cmdFetchMapDimensions when
// listing options: a NULL dimension_index is treated as bucket 0 everywhere,
// consistently. A bare `dimension_index = $2` would silently exclude NULL rows
// even when $2 = 0, because SQL NULL = 0 is neither true nor false — those
// rows would still be labelled "dimension 0" on display and would have no
// dimension option in the selector to reach them, only reappearing under "all
// dimensions". The game server's own save_actors routine already does
// `coalesce(in_server_info.dimension_index, 0)` on write, so NULL should be
// rare in practice (legacy/edge-case rows only) — but the read path must not
// assume that holds.
func dimensionFilterSQL(alias string, dimension *int) (string, []any) {
	if dimension == nil {
		return "", nil
	}
	return fmt.Sprintf(" AND COALESCE(%s.dimension_index, 0) = $2", alias), []any{*dimension}
}

// cmdFetchMapMarkers returns every plottable entity (players + vehicles) on the
// given map, reading positions from dune.actors.transform. Bases are added in
// Phase 2b. The map key is validated, then passed as a bound parameter. dimension
// optionally narrows results to a single dimension_index (#274) — nil returns
// every dimension merged, matching pre-#274 behaviour.
func cmdFetchMapMarkers(ctx context.Context, pool *pgxpool.Pool, mapKey string, dimension *int) ([]mapMarker, error) {
	if err := validateMapKey(mapKey); err != nil {
		return nil, err
	}

	markers := []mapMarker{}

	dimClause, dimArgs := dimensionFilterSQL("a", dimension)

	// Players: position is the player's pawn actor transform.
	// fls_id must be accounts."user" (hex UUID) — that is what RMQ PlayerId and
	// isHexIDOnline both expect. encrypted_funcom_id is the display name and is
	// NOT valid for those uses.
	playerRows, err := pool.Query(ctx, `
		SELECT a.id,
		       COALESCE(NULLIF(ps.character_name, ''), 'Unknown') AS name,
		       COALESCE(ps.online_status::text, '') AS online_status,
		       COALESCE(a.partition_id, 0) AS partition_id,
		       COALESCE(a.dimension_index, 0) AS dimension_index,
		       COALESCE(ac."user", '') AS fls_id,
		       ((a.transform).location).x,
		       ((a.transform).location).y,
		       ((a.transform).location).z
		FROM dune.actors a
		JOIN dune.player_state ps ON ps.player_pawn_id = a.id
		LEFT JOIN dune.accounts ac ON ac.id = ps.account_id
		WHERE a.map = $1 AND a.transform IS NOT NULL`+dimClause,
		append([]any{mapKey}, dimArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("query player markers: %w", err)
	}
	defer playerRows.Close()
	for playerRows.Next() {
		m := mapMarker{Type: "player", Map: mapKey}
		if err := playerRows.Scan(&m.ID, &m.Name, &m.OnlineStatus, &m.PartitionID, &m.DimensionIndex, &m.FLSID, &m.X, &m.Y, &m.Z); err != nil {
			return nil, fmt.Errorf("scan player marker: %w", err)
		}
		markers = append(markers, m)
	}
	if err := playerRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate player markers: %w", err)
	}

	// Vehicles: dune.vehicles joined to its actor row for transform + class.
	vehicleRows, err := pool.Query(ctx, `
		SELECT a.id,
		       a.class,
		       COALESCE(a.partition_id, 0) AS partition_id,
		       COALESCE(a.dimension_index, 0) AS dimension_index,
		       ((a.transform).location).x,
		       ((a.transform).location).y,
		       ((a.transform).location).z
		FROM dune.vehicles v
		JOIN dune.actors a ON a.id = v.id
		WHERE a.map = $1 AND a.transform IS NOT NULL`+dimClause,
		append([]any{mapKey}, dimArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("query vehicle markers: %w", err)
	}
	defer vehicleRows.Close()
	for vehicleRows.Next() {
		m := mapMarker{Type: "vehicle", Map: mapKey}
		if err := vehicleRows.Scan(&m.ID, &m.Class, &m.PartitionID, &m.DimensionIndex, &m.X, &m.Y, &m.Z); err != nil {
			return nil, fmt.Errorf("scan vehicle marker: %w", err)
		}
		m.Class = shortClass(m.Class)
		m.Name = m.Class
		markers = append(markers, m)
	}
	if err := vehicleRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vehicle markers: %w", err)
	}

	bases, err := cmdFetchBaseMarkers(ctx, pool, mapKey, dimension)
	if err != nil {
		return nil, err
	}
	markers = append(markers, bases...)

	return markers, nil
}

// cmdFetchBaseMarkers returns base-totem map markers for the given map. Each
// building's canonical totem actor (lowest owner_entity_id) is joined to
// permission_actor for the display name, mirroring cmdListBases. dimension
// optionally narrows results to a single dimension_index (#274) — nil returns
// every dimension.
func cmdFetchBaseMarkers(ctx context.Context, pool *pgxpool.Pool, mapKey string, dimension *int) ([]mapMarker, error) {
	dimClause, dimArgs := dimensionFilterSQL("t", dimension)
	rows, err := pool.Query(ctx, `
		SELECT b.id,
		       COALESCE(NULLIF(pa.actor_name, ''), 'Base') AS name,
		       COALESCE(t.dimension_index, 0) AS dimension_index,
		       ((t.transform).location).x,
		       ((t.transform).location).y,
		       ((t.transform).location).z
		FROM dune.buildings b
		JOIN (
		    SELECT building_id, MIN(owner_entity_id) AS owner_entity_id
		    FROM dune.building_instances
		    GROUP BY building_id
		) first_inst ON first_inst.building_id = b.id
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = first_inst.owner_entity_id
		JOIN dune.actors t ON t.id = afe.actor_id AND t.class ILIKE '%Totem%'
		LEFT JOIN dune.permission_actor pa ON pa.actor_id = t.id
		WHERE t.map = $1 AND t.transform IS NOT NULL`+dimClause,
		append([]any{mapKey}, dimArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("query base markers: %w", err)
	}
	defer rows.Close()
	var out []mapMarker
	for rows.Next() {
		m := mapMarker{Type: "base", Map: mapKey}
		if err := rows.Scan(&m.ID, &m.Name, &m.DimensionIndex, &m.X, &m.Y, &m.Z); err != nil {
			return nil, fmt.Errorf("scan base marker: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate base markers: %w", err)
	}
	return out, nil
}

// cmdFetchMapDimensions returns the distinct dimension_index values present for
// the given map's actors, sorted ascending, so the frontend can populate a
// dimension selector (#274). Always returns a non-nil slice.
//
// Groups on COALESCE(dimension_index, 0) rather than filtering NULL rows out,
// so it agrees with dimensionFilterSQL and the marker queries' display
// COALESCE: a map with only NULL-dimension actors still reports dimension 0 as
// a selectable option, instead of omitting the only dimension those actors
// could ever be filtered into.
func cmdFetchMapDimensions(ctx context.Context, pool *pgxpool.Pool, mapKey string) ([]int, error) {
	if err := validateMapKey(mapKey); err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT COALESCE(dimension_index, 0)
		FROM dune.actors
		WHERE map = $1
		ORDER BY 1`, mapKey)
	if err != nil {
		return nil, fmt.Errorf("query map dimensions: %w", err)
	}
	defer rows.Close()
	dims := []int{}
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan dimension: %w", err)
		}
		dims = append(dims, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate map dimensions: %w", err)
	}
	return dims, nil
}

// ── storage container commands ────────────────────────────────────────────────

type storageContainerRow struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Class         string   `json:"class"`
	Map           string   `json:"map"`
	ItemCount     int64    `json:"item_count"`
	ItemTemplates []string `json:"item_templates"`
	ItemNames     []string `json:"item_names"`
	OwnerName     string   `json:"owner_name"`
}

type msgStorageContainers struct {
	rows []storageContainerRow
	err  error
}

func cmdListStorageContainers(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgStorageContainers{err: fmt.Errorf("not connected")}
	}
	// Drive from dune.placeables so we catch player-built containers regardless
	// of whether they've been promoted to an actor row yet (the game creates the
	// actor lazily on first interaction). building_type is the in-data identity
	// of the placeable kind; the six below cover the storage-container tiers,
	// noting that "Small Storage Container" registers as SpiceSilo_Placeable
	// despite sharing the type name with world POI silos — owner_entity_id
	// distinguishes player-built from world-spawned. Totem_Placeable /
	// Totem_Small_Placeable are the Advanced Sub-Fief Console / Sub-Fief
	// Console — Patch 1.2 gave sub-fiefs their own storage compartment, which
	// lives on the console's own inventory (same actor_id-keyed join as any
	// other container, confirmed against a live server) (#263).
	// User-given container names live on dune.permission_actor.actor_name.
	// Unnamed containers default to 'None' or '##<PlaceableType>_Placeable' —
	// filter both out so only real custom names surface.
	rows, err := pool.Query(context.Background(), `
		SELECT p.id,
		       COALESCE(MAX(CASE
		           WHEN pa.actor_name NOT LIKE '##%' AND pa.actor_name <> 'None'
		           THEN pa.actor_name
		       END), '') AS name,
		       p.building_type AS class,
		       COALESCE(a.map, '') AS map,
		       COUNT(DISTINCT i.id) AS item_count,
		       COALESCE(array_agg(DISTINCT i.template_id) FILTER (WHERE i.template_id IS NOT NULL), '{}') AS item_templates,
		       COALESCE(MAX(ps.character_name), MAX(convert_from(e.encrypted_funcom_id, 'UTF8')), '') AS owner_name
		FROM dune.placeables p
		LEFT JOIN dune.actors a            ON a.id = p.id
		LEFT JOIN dune.permission_actor pa ON pa.actor_id = p.id
		LEFT JOIN dune.inventories inv     ON inv.actor_id = p.id
		LEFT JOIN dune.items i             ON i.inventory_id = inv.id
		LEFT JOIN dune.actor_fgl_entities afe  ON afe.entity_id = p.owner_entity_id
		LEFT JOIN dune.permission_actor_rank par ON par.permission_actor_id = afe.actor_id
		LEFT JOIN dune.actors player_a          ON player_a.id = par.player_id
		LEFT JOIN dune.encrypted_accounts e     ON e.id = player_a.owner_account_id
		LEFT JOIN dune.player_state ps          ON ps.account_id = player_a.owner_account_id
		WHERE p.building_type IN (
		    'SpiceSilo_Placeable',
		    'GenericContainer_Placeable',
		    'StorageContainer_Placeable',
		    'MediumStorageContainer_Placeable',
		    'Totem_Placeable',
		    'Totem_Small_Placeable'
		  )
		  AND p.is_hologram = false
		  AND p.owner_entity_id IS NOT NULL
		  AND p.owner_entity_id != 0
		GROUP BY p.id, p.building_type, a.map
		ORDER BY p.id`)
	if err != nil {
		return msgStorageContainers{err: err}
	}
	defer rows.Close()
	var out []storageContainerRow
	for rows.Next() {
		var r storageContainerRow
		var templates []string
		if err := rows.Scan(&r.ID, &r.Name, &r.Class, &r.Map, &r.ItemCount, &templates, &r.OwnerName); err != nil {
			continue
		}
		if templates != nil {
			r.ItemTemplates = templates
		} else {
			r.ItemTemplates = []string{}
		}
		r.ItemNames = []string{}
		for _, t := range templates {
			if name := itemData.Names[strings.ToLower(t)]; name != "" {
				r.ItemNames = append(r.ItemNames, name)
			}
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return msgStorageContainers{err: rows.Err()}
	}
	return msgStorageContainers{rows: out}
}

type msgContainerInventory struct {
	rows []itemInfo
	err  error
}

func cmdGetContainerInventory(pool *pgxpool.Pool, actorID int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgContainerInventory{err: fmt.Errorf("not connected")}
		}
		rows, err := pool.Query(context.Background(), `
			SELECT i.id, i.template_id, i.stack_size, i.quality_level,
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'), 'N/A'),
			       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'), 'N/A')
			FROM dune.items i
			JOIN dune.inventories inv ON i.inventory_id = inv.id
			WHERE inv.actor_id = $1
			ORDER BY i.template_id`, actorID)
		if err != nil {
			return msgContainerInventory{err: err}
		}
		defer rows.Close()
		var items []itemInfo
		for rows.Next() {
			var it itemInfo
			if err := rows.Scan(&it.ID, &it.TemplateID, &it.StackSize, &it.Quality, &it.Durability, &it.MaxDurability); err != nil {
				continue
			}
			it.Name = itemData.Names[strings.ToLower(it.TemplateID)]
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return msgContainerInventory{err: err}
		}
		return msgContainerInventory{rows: items}
	}
}

func cmdGiveItemToContainer(pool *pgxpool.Pool, actorID int64, templateID string, qty, quality int64) Cmd {
	return func() Msg {
		if pool == nil {
			return msgMutate{err: fmt.Errorf("not connected")}
		}
		ctx := context.Background()

		// Find the container's inventory (any type).
		var invID int64
		var maxCount int
		var maxVol float32
		err := pool.QueryRow(ctx, `
			SELECT id, max_item_count, max_item_volume
			FROM dune.inventories
			WHERE actor_id = $1
			LIMIT 1`, actorID).Scan(&invID, &maxCount, &maxVol)
		if err != nil {
			return msgMutate{err: fmt.Errorf("find container inventory: %w", err)}
		}

		// Count current items.
		var currentCount int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM dune.items WHERE inventory_id = $1`, invID).Scan(&currentCount); err != nil {
			return msgMutate{err: fmt.Errorf("count items: %w", err)}
		}
		if maxCount > 0 && currentCount >= int64(maxCount) {
			return msgMutate{err: fmt.Errorf("container inventory full (%d/%d)", currentCount, maxCount)}
		}

		// Insert item with minimal valid stats matching game-generated items.
		_, err = pool.Exec(ctx, `
			INSERT INTO dune.items (inventory_id, template_id, stack_size, quality_level, position_index, stats)
			VALUES ($1, $2, $3, $4, $5, '{"FCustomizationStats":[[],{}],"FItemStackAndDurabilityStats":[[],{}]}')`,
			invID, templateID, qty, quality, currentCount)
		if err != nil {
			return msgMutate{err: fmt.Errorf("insert item: %w", err)}
		}
		return msgMutate{ok: fmt.Sprintf("Added %dx %s (quality %d) to container %d", qty, templateID, quality, actorID)}
	}
}

func cmdListBases(pool *pgxpool.Pool) Msg {
	if pool == nil {
		return msgBaseList{err: fmt.Errorf("not connected")}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT b.id,
		       COALESCE(pa.actor_name, '') AS name,
		       COALESCE(inst.cnt, 0) AS pieces,
		       COALESCE(plac.cnt, 0) AS placeables
		FROM dune.buildings b
		LEFT JOIN (
		    SELECT building_id, MIN(owner_entity_id) AS owner_entity_id, COUNT(*) AS cnt
		    FROM dune.building_instances
		    GROUP BY building_id
		) inst ON inst.building_id = b.id
		LEFT JOIN dune.actor_fgl_entities afe ON afe.entity_id = inst.owner_entity_id
		LEFT JOIN dune.actors t ON t.id = afe.actor_id AND t.class ILIKE '%Totem%'
		LEFT JOIN dune.permission_actor pa ON pa.actor_id = t.id
		LEFT JOIN (
		    SELECT bi.building_id, COUNT(*) AS cnt
		    FROM dune.building_instances bi
		    JOIN dune.placeables p ON p.owner_entity_id = bi.owner_entity_id
		    GROUP BY bi.building_id
		) plac ON plac.building_id = b.id
		ORDER BY b.id`)
	if err != nil {
		return msgBaseList{err: err}
	}
	defer rows.Close()
	var out []baseRow
	for rows.Next() {
		var r baseRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Pieces, &r.Placeables); err != nil {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return msgBaseList{err: err}
	}
	return msgBaseList{rows: out}
}

// ── player stats ─────────────────────────────────────────────────────────────

// cmdFetchOnlineAccountIDs returns the account IDs of all players currently
// marked Online in player_state. Used by the session poller.
func cmdFetchOnlineAccountIDs(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	rows, err := pool.Query(ctx, `SELECT account_id FROM dune.player_state WHERE online_status = 'Online' AND account_id <> $1`, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("fetch online account ids: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan account id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type playerPgStats struct {
	SolarisBal      int64      `json:"solaris_balance"`
	ScripBal        int64      `json:"scrip_balance"`
	SolarisEarned   int64      `json:"solaris_earned"`
	SolarisSpent    int64      `json:"solaris_spent"`
	POIsDiscovered  int        `json:"pois_discovered"`
	StoryMilestones int        `json:"story_milestones"`
	MaxFactionTier  int        `json:"max_faction_tier"`
	Faction         string     `json:"faction"`
	CharXP          int64      `json:"char_xp"`
	SkillPoints     int        `json:"skill_points"`
	LastSeen        *time.Time `json:"last_seen"`
}

// fetchItemFormSolari sums Solari held as stackable SolarisCoin item stacks:
// what the player is carrying, plus Solari sitting in containers/placeables
// they own, using the same ownership-chain join as cmdListStorageContainers.
// Top-level inventories only (no recursion into sub-containers nested inside
// those). This is distinct from the bank ledger (player_virtual_currency_balances)
// — dune_exchange_retrieve_solaris_from_item converts one into the other,
// confirming they're genuinely separate stores (#266). Shared by
// cmdFetchPlayerPgStats (the live "Solaris display") and fetchSolarisBalance
// (the periodic stat-snapshot collector, #297) so the two totals can never
// drift apart again.
func fetchItemFormSolari(ctx context.Context, pool *pgxpool.Pool, accountID int64) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("not connected")
	}
	var itemSolari int64
	itemRow := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(i.stack_size), 0)
		FROM dune.items i
		JOIN dune.inventories inv ON inv.id = i.inventory_id
		WHERE lower(i.template_id) = 'solariscoin'
		  AND (
		    inv.actor_id IN (SELECT player_pawn_id FROM dune.player_state WHERE account_id = $1)
		    OR inv.actor_id IN (
		      SELECT p.id
		      FROM dune.placeables p
		      JOIN dune.actor_fgl_entities afe    ON afe.entity_id = p.owner_entity_id
		      JOIN dune.permission_actor_rank par ON par.permission_actor_id = afe.actor_id
		      JOIN dune.actors player_a           ON player_a.id = par.player_id
		      WHERE player_a.owner_account_id = $1
		    )
		  )
	`, accountID)
	if err := itemRow.Scan(&itemSolari); err != nil {
		return 0, fmt.Errorf("fetch item-form solari for account %d: %w", accountID, err)
	}
	return itemSolari, nil
}

// cmdFetchPlayerPgStats gathers all Postgres-derived stats for a player.
//
// Economy uses two queries:
//  1. Current balances from player_virtual_currency_balances (always works).
//  2. Earned/spent totals from event_log, derived via a balance-match CTE:
//     the most recent solaris_balance in the event_log is matched against the
//     current balance to identify the player's hex FLS entity ID. This works
//     as long as the live balance equals the last logged balance.
func cmdFetchPlayerPgStats(ctx context.Context, pool *pgxpool.Pool, accountID int64) (playerPgStats, error) {
	var stats playerPgStats

	// Current currency balances via player_controller_id.
	rows, err := pool.Query(ctx, `
		SELECT pvc.currency_id, pvc.balance
		FROM dune.player_virtual_currency_balances pvc
		JOIN dune.player_state ps ON ps.player_controller_id = pvc.player_controller_id
		WHERE ps.account_id = $1
	`, accountID)
	if err != nil {
		return stats, fmt.Errorf("fetch currency for account %d: %w", accountID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int16
		var bal int64
		if err := rows.Scan(&cid, &bal); err != nil {
			return stats, fmt.Errorf("scan currency: %w", err)
		}
		switch cid {
		case 0:
			stats.SolarisBal = bal
		case 1:
			stats.ScripBal = bal
		}
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("iterate currency: %w", err)
	}

	// Solari is also representable as a stackable inventory item (SolarisCoin),
	// separate from the bank ledger above (dune_exchange_retrieve_solaris_from_item
	// converts one into the other, confirming they're genuinely distinct stores).
	// The bank-only total under-reported a player's wealth (#266) — add Solari
	// the player is carrying, plus Solari sitting in containers they own. Shared
	// with fetchSolarisBalance (the stat-snapshot collector, #297) via
	// fetchItemFormSolari so the two totals can never drift apart again.
	itemSolari, err := fetchItemFormSolari(ctx, pool, accountID)
	if err != nil {
		return stats, err
	}
	stats.SolarisBal += itemSolari

	// Earned/spent totals from event_log, joined directly via dune.accounts."user"
	// which stores the hex PlayFab entity ID used as event_log.meta->>'fls_id'.
	row := pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN COALESCE((el.meta->>'solaris_delta')::float, 0) > 0
			                  THEN (el.meta->>'solaris_delta')::float ELSE 0 END), 0)::bigint,
			COALESCE(SUM(CASE WHEN COALESCE((el.meta->>'solaris_delta')::float, 0) < 0
			                  THEN ABS((el.meta->>'solaris_delta')::float) ELSE 0 END), 0)::bigint
		FROM dune.event_log el
		JOIN dune.accounts ac ON ac."user" = el.meta->>'fls_id'
		WHERE ac.id = $1 AND el.meta->>'solaris_delta' IS NOT NULL
	`, accountID)
	if err := row.Scan(&stats.SolarisEarned, &stats.SolarisSpent); err != nil {
		return stats, fmt.Errorf("fetch solaris earned/spent for account %d: %w", accountID, err)
	}

	// Tag-derived stats: POIs discovered, story milestones, max faction tier.
	// player_tags migrated account_id -> character_id (issue #267); resolve the key.
	tagKeyCol, tagKeyVal, err := playerKeyFor(ctx, pool, "player_tags", accountID)
	if err != nil {
		return stats, fmt.Errorf("resolve tag-stats key for account %d: %w", accountID, err)
	}
	// #nosec G201 -- tagKeyCol is a fixed internal allowlist (character_id|account_id)
	row = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(*) FILTER (WHERE tag LIKE 'Exploration.POI.%%'),
			COUNT(*) FILTER (WHERE tag LIKE 'BigMoments.%%.Complete'),
			COALESCE(MAX(
				CASE WHEN tag ~ '^Faction\.[^.]+\.Tier[0-9]+$'
				     THEN CAST(SUBSTRING(tag FROM '[0-9]+$') AS INTEGER)
				     ELSE NULL END
			), 0)
		FROM dune.player_tags
		WHERE %s = $1
	`, tagKeyCol), tagKeyVal)
	if err := row.Scan(&stats.POIsDiscovered, &stats.StoryMilestones, &stats.MaxFactionTier); err != nil {
		return stats, fmt.Errorf("fetch tag stats for account %d: %w", accountID, err)
	}

	// Character XP and total skill points from FLevelComponent — joined via pawn_id.
	row = pool.QueryRow(ctx, `
		SELECT
			COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0),
			COALESCE((fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::int, 0)
		FROM dune.fgl_entities fe
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
		JOIN dune.player_state ps ON ps.player_pawn_id = afe.actor_id
		WHERE afe.slot_name = 'DuneCharacter' AND ps.account_id = $1
	`, accountID)
	if err := row.Scan(&stats.CharXP, &stats.SkillPoints); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("fetch char xp for account %d: %w", accountID, err)
	}

	// Last seen: most recent avatar activity timestamp.
	var lastSeen pgtype.Timestamptz
	row = pool.QueryRow(ctx, `SELECT last_avatar_activity FROM dune.player_state WHERE account_id = $1`, accountID)
	if err := row.Scan(&lastSeen); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("fetch last seen for account %d: %w", accountID, err)
	}
	if lastSeen.Valid {
		t := lastSeen.Time
		stats.LastSeen = &t
	}

	// Faction alignment (#117 review item 3) — see fetchAccountFaction.
	stats.Faction, err = fetchAccountFaction(ctx, pool, accountID)
	if err != nil {
		return stats, err
	}

	return stats, nil
}

// fetchAccountFaction resolves a player's faction name by account. Faction is
// stored on the PlayerController actor, NOT the PlayerCharacter, so it's resolved
// per-account (same rationale as factionByAccountJoin). Returns "" when the
// player has no faction row (Unaligned).
func fetchAccountFaction(ctx context.Context, pool *pgxpool.Pool, accountID int64) (string, error) {
	var faction string
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(f.name, '')
		FROM dune.factions f
		WHERE f.id = (
			SELECT pf.faction_id FROM dune.player_faction pf
			JOIN dune.actors fa ON fa.id = pf.actor_id
			WHERE fa.owner_account_id = $1
			LIMIT 1
		)`, accountID).Scan(&faction)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("fetch faction for account %d: %w", accountID, err)
	}
	return faction, nil
}

// solarisRaw is an intermediate scan target before cumulative sums are applied.
type solarisRaw struct {
	Time    string
	Balance int64
	Delta   int64
}

type solarisPoint struct {
	Time      string `json:"time"`
	Balance   int64  `json:"balance"`
	CumEarned int64  `json:"cum_earned"`
	CumSpent  int64  `json:"cum_spent"`
}

// accumulateSolarisPoints converts raw (time, balance, delta) rows into points
// with monotonically-increasing cumulative earned and spent totals.
func accumulateSolarisPoints(raws []solarisRaw) []solarisPoint {
	out := make([]solarisPoint, 0, len(raws))
	var cumEarned, cumSpent int64
	for _, r := range raws {
		if r.Delta > 0 {
			cumEarned += r.Delta
		} else if r.Delta < 0 {
			cumSpent += -r.Delta
		}
		out = append(out, solarisPoint{
			Time:      r.Time,
			Balance:   r.Balance,
			CumEarned: cumEarned,
			CumSpent:  cumSpent,
		})
	}
	return out
}

// cmdFetchSolarisHistory returns timestamped solaris balance snapshots for a
// player, joined directly via dune.accounts."user" = event_log.meta->>'fls_id'.
// Returns at most 500 points with cumulative earned/spent in ascending order.
func cmdFetchSolarisHistory(ctx context.Context, pool *pgxpool.Pool, accountID int64) ([]solarisPoint, error) {
	rows, err := pool.Query(ctx, `
		SELECT
			to_char(el.event_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			ROUND((el.meta->>'solaris_balance')::float)::bigint,
			COALESCE(ROUND((el.meta->>'solaris_delta')::float)::bigint, 0)
		FROM dune.event_log el
		JOIN dune.accounts ac ON ac."user" = el.meta->>'fls_id'
		WHERE ac.id = $1 AND el.meta->>'solaris_balance' IS NOT NULL
		ORDER BY el.event_time ASC
		LIMIT 500
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("fetch solaris history for account %d: %w", accountID, err)
	}
	defer rows.Close()

	var raws []solarisRaw
	for rows.Next() {
		var r solarisRaw
		if err := rows.Scan(&r.Time, &r.Balance, &r.Delta); err != nil {
			return nil, fmt.Errorf("scan solaris point: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate solaris history for account %d: %w", accountID, err)
	}
	out := accumulateSolarisPoints(raws)
	if len(out) == 0 {
		out = []solarisPoint{}
	}
	return out, nil
}

// cmdFetchPlayerSnapshot queries the current stat values for one player from
// Postgres. Returns a statSnapshot ready to write to SQLite.
func cmdFetchPlayerSnapshot(ctx context.Context, pool *pgxpool.Pool, accountID int64, snappedAt string) (statSnapshot, error) {
	snap := statSnapshot{AccountID: accountID, SnappedAt: snappedAt}

	// Character XP and skill points from FLevelComponent.
	row := pool.QueryRow(ctx, `
		SELECT
			(fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint,
			(fe.components->'FLevelComponent'->1->>'TotalSkillPoints')::int
		FROM dune.fgl_entities fe
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
		JOIN dune.player_state ps ON ps.player_pawn_id = afe.actor_id
		WHERE afe.slot_name = 'DuneCharacter' AND ps.account_id = $1
	`, accountID)
	var charXP int64
	var sp int
	if err := row.Scan(&charXP, &sp); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return snap, fmt.Errorf("fetch char xp snapshot for account %d: %w", accountID, err)
	} else if err == nil {
		snap.CharXP = &charXP
		snap.SkillPoints = &sp
	}

	// Intel points from TechKnowledgePlayerComponent on the pawn actor.
	row = pool.QueryRow(ctx, `
		SELECT (a.properties->'TechKnowledgePlayerComponent'->>'m_TechKnowledgePoints')::int
		FROM dune.actors a
		JOIN dune.player_state ps ON ps.player_pawn_id = a.id
		WHERE ps.account_id = $1 AND a.properties ? 'TechKnowledgePlayerComponent'
	`, accountID)
	var intel int
	if err := row.Scan(&intel); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return snap, fmt.Errorf("fetch intel snapshot for account %d: %w", accountID, err)
	} else if err == nil {
		snap.IntelPoints = &intel
	}

	// Spec track XPs — NULL for players not in specialization_tracks.
	rows, err := pool.Query(ctx, `
		SELECT track_type::text, xp_amount
		FROM dune.specialization_tracks st
		JOIN dune.player_state ps ON ps.player_controller_id = st.player_id
		WHERE ps.account_id = $1
	`, accountID)
	if err != nil {
		return snap, fmt.Errorf("fetch spec snapshot for account %d: %w", accountID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var track string
		var xp int
		if err := rows.Scan(&track, &xp); err != nil {
			return snap, fmt.Errorf("scan spec track: %w", err)
		}
		xpCopy := xp
		switch track {
		case "Combat":
			snap.CombatXP = &xpCopy
		case "Crafting":
			snap.CraftingXP = &xpCopy
		case "Gathering":
			snap.GatheringXP = &xpCopy
		case "Exploration":
			snap.ExplorationXP = &xpCopy
		case "Sabotage":
			snap.SabotageXP = &xpCopy
		}
	}
	if err := rows.Err(); err != nil {
		return snap, fmt.Errorf("iterate spec tracks for account %d: %w", accountID, err)
	}

	snap.SolarisBalance, err = fetchSolarisBalance(ctx, pool, accountID)
	if err != nil {
		return snap, err
	}
	return snap, nil
}

// fetchSolarisBalance returns the total Solari the stat-snapshot collector
// should record for a player: wallet balance plus item-form Solari (#297).
// The bank-only version of this query returned nil whenever a player had no
// wallet row, which silently dropped history points for anyone whose only
// Solari was carried as SolarisCoin items or sitting in an owned base stash
// — the Solari chart looked "stopped" rather than merely never-started. A
// player with no wallet row and no item Solari still gets a real 0 recorded,
// not a NULL, so the chart carries an unbroken series.
func fetchSolarisBalance(ctx context.Context, pool *pgxpool.Pool, accountID int64) (*int64, error) {
	if pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	var walletBal int64
	err := pool.QueryRow(ctx, `
		SELECT pvc.balance
		FROM dune.player_virtual_currency_balances pvc
		JOIN dune.player_state ps ON ps.player_controller_id = pvc.player_controller_id
		WHERE ps.account_id = $1 AND pvc.currency_id = 0
	`, accountID).Scan(&walletBal)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fetch solaris snapshot for account %d: %w", accountID, err)
	}
	// walletBal stays 0 on ErrNoRows — a player without a wallet row may
	// still carry Solari as items.

	itemSolari, err := fetchItemFormSolari(ctx, pool, accountID)
	if err != nil {
		return nil, err
	}

	total := walletBal + itemSolari
	return &total, nil
}

// cmdActorIDFromFlsID resolves the player pawn actor ID from their hex FLS ID
// (accounts."user"). Used by cmdRefillWaterOffline and similar offline writes
// that require an actor_id to locate inventories.
func cmdActorIDFromFlsID(ctx context.Context, pool *pgxpool.Pool, flsID string) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("not connected")
	}
	var actorID int64
	err := pool.QueryRow(ctx, `
		SELECT ps.player_pawn_id
		FROM dune.accounts ac
		JOIN dune.player_state ps ON ps.account_id = ac.id
		WHERE ac."user" = $1
		LIMIT 1`, flsID).Scan(&actorID)
	if err != nil {
		return 0, fmt.Errorf("resolve actor id for fls_id %s: %w", flsID, err)
	}
	return actorID, nil
}

// cmdRefillWaterOffline sets CurrentAmount = MaxAmount for every water-fillable
// item in the player's carried inventories (backpack, equipment, weapon slots,
// action wheel, bank). Uses the waterFillableTemplates list generated from
// DT_ItemTableFillables.json. For online players use rmqUpdateAllWaterFillables
// instead — this path takes effect on the player's next relog.
func cmdRefillWaterOffline(ctx context.Context, pool *pgxpool.Pool, actorID int64) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("not connected")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE dune.items i
		SET stats = jsonb_set(
			i.stats,
			'{FFillableItemStats,1,CurrentAmount}',
			(i.stats->'FFillableItemStats'->1->'MaxAmount')
		)
		FROM dune.inventories inv
		WHERE inv.actor_id = $1
		  AND inv.inventory_type = ANY($2::int[])
		  AND i.inventory_id = inv.id
		  AND lower(i.template_id) = ANY($3::text[])
		  AND i.stats ? 'FFillableItemStats'
		  AND (i.stats->'FFillableItemStats'->1->'MaxAmount') IS NOT NULL`,
		actorID, repairGearInventoryTypes, waterFillableTemplates)
	if err != nil {
		return 0, fmt.Errorf("refill water offline actor %d: %w", actorID, err)
	}
	return tag.RowsAffected(), nil
}

// cmdReadLegacyDiscordLinks reads the legacy Postgres dune.discord_links table
// (pre-multi-guild registrations) for one-time migration into the SQLite
// discord_user_links store. Returns an empty slice (no error) when the table
// doesn't exist, so the migration treats "no legacy table" as nothing to copy.
// Distinct (discord_user_id) is collapsed to one row per user (the new model is
// one global character per user), keeping the most-recently-registered row.
func cmdReadLegacyDiscordLinks(ctx context.Context, db *pgxpool.Pool) ([]legacyUserLink, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('dune.discord_links') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check discord_links table: %w", err)
	}
	if !exists {
		return nil, nil
	}
	rows, err := db.Query(ctx, `
		SELECT DISTINCT ON (discord_user_id)
		       discord_user_id, account_id, character_name, COALESCE(avatar_url, '')
		FROM dune.discord_links
		ORDER BY discord_user_id, registered_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("read legacy discord_links: %w", err)
	}
	defer rows.Close()
	var out []legacyUserLink
	for rows.Next() {
		var l legacyUserLink
		if err := rows.Scan(&l.discordUserID, &l.accountID, &l.characterName, &l.avatarURL); err != nil {
			return nil, fmt.Errorf("scan legacy discord_link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// cmdFetchPlayerCurrencyCtx returns all currency balances for a single player
// identified by their player_controller_id.
func cmdFetchPlayerCurrencyCtx(ctx context.Context, db *pgxpool.Pool, controllerID int64) ([]currencyRow, error) {
	rows, err := db.Query(ctx, `
		SELECT player_controller_id, currency_id, balance
		FROM dune.player_virtual_currency_balances
		WHERE player_controller_id = $1
		ORDER BY currency_id`, controllerID)
	if err != nil {
		return nil, fmt.Errorf("fetch currency for controller %d: %w", controllerID, err)
	}
	defer rows.Close()
	var out []currencyRow
	for rows.Next() {
		var r currencyRow
		if err := rows.Scan(&r.PlayerID, &r.CurrencyID, &r.Balance); err != nil {
			return nil, fmt.Errorf("scan currency row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// cmdFetchPlayerInventoryCtx returns the inventory items for a single player
// actor (actor_id = playerInfo.ID).
func cmdFetchPlayerInventoryCtx(ctx context.Context, db *pgxpool.Pool, actorID int64) ([]itemInfo, error) {
	rows, err := db.Query(ctx, `
		SELECT i.id, i.template_id, i.stack_size, i.quality_level,
		       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'CurrentDurability'), 'N/A'),
		       COALESCE((i.stats->'FItemStackAndDurabilityStats'->1->>'MaxDurability'), 'N/A')
		FROM dune.items i
		JOIN dune.inventories inv ON i.inventory_id = inv.id
		WHERE inv.actor_id = $1::bigint
		ORDER BY i.template_id`, actorID)
	if err != nil {
		return nil, fmt.Errorf("fetch inventory for actor %d: %w", actorID, err)
	}
	defer rows.Close()
	var items []itemInfo
	for rows.Next() {
		var it itemInfo
		if err := rows.Scan(&it.ID, &it.TemplateID, &it.StackSize, &it.Quality, &it.Durability, &it.MaxDurability); err != nil {
			return nil, fmt.Errorf("scan inventory row: %w", err)
		}
		it.Name = itemData.Names[strings.ToLower(it.TemplateID)]
		items = append(items, it)
	}
	return items, rows.Err()
}

// ── events engine DB helpers ──────────────────────────────────────────────────

// cmdFetchEventPlayers returns all online players (excluding the GM identity)
// with the IDs the events engine needs: AccountID, player_controller_id
// (ControllerID), player_pawn_id (ActorID), and character name.
func cmdFetchEventPlayers(ctx context.Context, pool *pgxpool.Pool) ([]eventPlayer, error) {
	rows, err := pool.Query(ctx, `
		SELECT ps.account_id,
		       COALESCE(ps.player_controller_id, 0),
		       ps.player_pawn_id,
		       COALESCE(ps.character_name, '')
		FROM dune.player_state ps
		WHERE ps.online_status = 'Online'
		  AND ps.player_pawn_id IS NOT NULL
		  AND ps.account_id <> $1`, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("fetch event players: %w", err)
	}
	defer rows.Close()

	out := make([]eventPlayer, 0)
	for rows.Next() {
		var p eventPlayer
		if err := rows.Scan(&p.AccountID, &p.ControllerID, &p.ActorID, &p.Name); err != nil {
			return nil, fmt.Errorf("scan event player: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// cmdFetchEventGrantTargets returns the player_controller_id and player_pawn_id
// for a single account, regardless of online status. Reward-grant retries must
// work for offline players, so — unlike cmdFetchEventPlayers — this query does
// NOT filter on online_status.
func cmdFetchEventGrantTargets(ctx context.Context, pool *pgxpool.Pool, accountID int64) (controllerID, actorID int64, err error) {
	row := pool.QueryRow(ctx, `
		SELECT COALESCE(ps.player_controller_id, 0),
		       COALESCE(ps.player_pawn_id, 0)
		FROM dune.player_state ps
		WHERE ps.account_id = $1`, accountID)
	if scanErr := row.Scan(&controllerID, &actorID); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return 0, 0, errNotFound
		}
		return 0, 0, fmt.Errorf("fetch event grant targets %d: %w", accountID, scanErr)
	}
	return controllerID, actorID, nil
}

// cmdFetchBattlepassGrantTargets returns the player_pawn_id for a single
// account regardless of online status. Battlepass auto-grant retries must work
// for offline players, so — unlike cmdFetchBattlepassPlayers' online flag — this
// query does NOT filter on online_status. The pawn is the actor that receives
// both intel (awardIntel) and item rewards (giveItem).
func cmdFetchBattlepassGrantTargets(ctx context.Context, pool *pgxpool.Pool, accountID int64) (pawnID int64, err error) {
	row := pool.QueryRow(ctx, `
		SELECT COALESCE(ps.player_pawn_id, 0)
		FROM dune.player_state ps
		WHERE ps.account_id = $1`, accountID)
	if scanErr := row.Scan(&pawnID); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("fetch battlepass grant targets %d: %w", accountID, scanErr)
	}
	if pawnID == 0 {
		return 0, errNotFound
	}
	return pawnID, nil
}

// cmdFetchOnlinePositions returns a map of account_id → position for all
// provided account IDs that are currently online and have a live actor.
func cmdFetchOnlinePositions(ctx context.Context, pool *pgxpool.Pool, accountIDs []int64) (map[int64]playerPosition, error) {
	if len(accountIDs) == 0 {
		return map[int64]playerPosition{}, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT ps.account_id,
		       COALESCE(a.partition_id, 0),
		       COALESCE(a.map, ''),
		       ((a.transform).location).x,
		       ((a.transform).location).y,
		       ((a.transform).location).z
		FROM dune.player_state ps
		JOIN dune.actors a ON a.id = ps.player_pawn_id
		WHERE ps.online_status = 'Online'
		  AND ps.player_pawn_id IS NOT NULL
		  AND ps.account_id = ANY($1)`, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch online positions: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]playerPosition)
	for rows.Next() {
		var accountID int64
		var pos playerPosition
		if err := rows.Scan(&accountID, &pos.PartitionID, &pos.Map, &pos.X, &pos.Y, &pos.Z); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		out[accountID] = pos
	}
	return out, rows.Err()
}

// cmdFetchCharacterLevel returns the character level for the given account,
// derived from FLevelComponent.TotalXPEarned → xpToLevel.
func cmdFetchCharacterLevel(ctx context.Context, pool *pgxpool.Pool, accountID int64) (int, error) {
	var charXP int64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0)
		FROM dune.fgl_entities fe
		JOIN dune.actor_fgl_entities afe ON afe.entity_id = fe.entity_id
		JOIN dune.player_state ps ON ps.player_pawn_id = afe.actor_id
		WHERE afe.slot_name = 'DuneCharacter' AND ps.account_id = $1`, accountID).Scan(&charXP)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("fetch character level for account %d: %w", accountID, err)
	}
	return xpToLevel(charXP), nil
}

// cmdFetchPlayerTagsForAccount is the injectable (ctx+pool) form of
// cmdGetPlayerTags, used by the events engine dependency injection. Keyed via
// playerKeyFor so it follows the account_id -> character_id migration (#267).
func cmdFetchPlayerTagsForAccount(ctx context.Context, pool *pgxpool.Pool, accountID int64) ([]string, error) {
	keyCol, keyVal, err := playerKeyFor(ctx, pool, "player_tags", accountID)
	if err != nil {
		return nil, err
	}
	// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
	rows, err := pool.Query(ctx,
		fmt.Sprintf(`SELECT tag FROM dune.player_tags WHERE %s = $1 ORDER BY tag`, keyCol), keyVal)
	if err != nil {
		return nil, fmt.Errorf("fetch player tags for account %d: %w", accountID, err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan player tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// cmdFetchBattlepassPlayers returns one row per character the battlepass
// engine tracks: account, pawn actor, name, online flag, and level (derived
// from the same FLevelComponent XP source as cmdFetchCharacterLevel).
func cmdFetchBattlepassPlayers(ctx context.Context, pool *pgxpool.Pool) ([]battlepassPlayer, error) {
	rows, err := pool.Query(ctx, `
		SELECT ps.account_id,
		       ps.player_pawn_id,
		       COALESCE(ps.character_name, ''),
		       (ps.online_status::text = 'Online'),
		       COALESCE((fe.components->'FLevelComponent'->1->>'TotalXPEarned')::bigint, 0)
		FROM dune.player_state ps
		LEFT JOIN dune.actor_fgl_entities afe
		       ON afe.actor_id = ps.player_pawn_id AND afe.slot_name = 'DuneCharacter'
		LEFT JOIN dune.fgl_entities fe ON fe.entity_id = afe.entity_id
		WHERE ps.player_pawn_id IS NOT NULL
		  AND ps.account_id <> $1`, gmIdentityAccountID)
	if err != nil {
		return nil, fmt.Errorf("fetch battlepass players: %w", err)
	}
	defer rows.Close()

	out := make([]battlepassPlayer, 0)
	for rows.Next() {
		var p battlepassPlayer
		var xp int64
		if err := rows.Scan(&p.AccountID, &p.PawnID, &p.Name, &p.Online, &xp); err != nil {
			return nil, fmt.Errorf("scan battlepass player: %w", err)
		}
		p.Level = xpToLevel(xp)
		out = append(out, p)
	}
	return out, rows.Err()
}

// cmdFetchCompletedJourneyNodeIDs returns the IDs of every completed journey
// story node for the account. Used by the battlepass engine, which polls
// slower than the journey cache TTL and reads completion state only. Keyed via
// playerKeyFor so it follows the account_id -> character_id migration (#267).
func cmdFetchCompletedJourneyNodeIDs(ctx context.Context, pool *pgxpool.Pool, accountID int64) ([]string, error) {
	keyCol, keyVal, err := playerKeyFor(ctx, pool, "journey_story_node", accountID)
	if err != nil {
		return nil, err
	}
	// #nosec G201 -- keyCol is a fixed internal allowlist (character_id|account_id)
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT story_node_id FROM dune.journey_story_node
		WHERE %s = $1 AND complete_condition_state = 'true'::jsonb`, keyCol), keyVal)
	if err != nil {
		return nil, fmt.Errorf("fetch completed journey nodes for account %d: %w", accountID, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan completed journey node: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// cmdGiveItemCtx is the injectable form used by the events engine.
func cmdGiveItemCtx(_ context.Context, pool *pgxpool.Pool, actorID int64, template string, qty, quality int64) error {
	msg := runGiveItem(pool, actorID, template, qty, quality)
	if m, ok := msg.(msgMutate); ok && m.err != nil {
		return m.err
	}
	return nil
}

// cmdAwardXPCtx is the injectable form used by the events engine.
func cmdAwardXPCtx(_ context.Context, pool *pgxpool.Pool, playerID int64, track string, amount int32) error {
	msg := cmdAwardXP(pool, playerID, track, amount)()
	if m, ok := msg.(msgMutate); ok && m.err != nil {
		return m.err
	}
	return nil
}
