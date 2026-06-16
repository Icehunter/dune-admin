package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// discordGuildsStore persists the Discord model in the unified SQLite store
// across three tables:
//
//   - discord_guilds: per-guild AUTH only (the three capability-role CSVs).
//   - discord_servers: ONE guild per game server (server_id PK) + that server's
//     own announce + status channels in that guild.
//   - discord_user_links: ONE character per (Discord user, server).
//
// FK ON DELETE CASCADE: deleting a server removes its discord_servers row + its
// discord_user_links rows. discord_guilds is auth-only; deleting a guild does
// NOT cascade discord_servers (a server's link references servers(id), not the
// guild) — guild deletion is roles-only.
type discordGuildsStore struct{ db *sql.DB }

const discordGuildsSchema = `
CREATE TABLE IF NOT EXISTS discord_guilds (
	guild_id      TEXT PRIMARY KEY,
	roles_viewer  TEXT NOT NULL DEFAULT '',
	roles_economy TEXT NOT NULL DEFAULT '',
	roles_admin   TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL DEFAULT '',
	updated_at    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS discord_servers (
	server_id               INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
	guild_id                TEXT    NOT NULL,
	announce_channel_id     TEXT    NOT NULL DEFAULT '',
	status_channel_id       TEXT    NOT NULL DEFAULT '',
	status_enabled          INTEGER NOT NULL DEFAULT 0,
	status_interval_seconds INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS discord_user_links (
	discord_user_id TEXT    NOT NULL,
	server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	account_id      INTEGER NOT NULL,
	character_name  TEXT    NOT NULL,
	avatar_url      TEXT    NOT NULL DEFAULT '',
	registered_at   TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (discord_user_id, server_id)
);`

// initDiscordGuildsSchema creates the three Discord tables + their indexes. Must
// run AFTER initServersSchema: discord_servers and discord_user_links FK-reference
// servers. Idempotent.
func initDiscordGuildsSchema(db *sql.DB) error {
	if _, err := db.Exec(discordGuildsSchema); err != nil {
		return fmt.Errorf("init discord_guilds schema: %w", err)
	}
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_discord_servers_guild ON discord_servers(guild_id)`,
		`CREATE INDEX IF NOT EXISTS idx_discord_user_links_server ON discord_user_links(server_id)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("init discord index: %w", err)
		}
	}
	return nil
}

func newDiscordGuildsStore(db *sql.DB) *discordGuildsStore { return &discordGuildsStore{db: db} }

// globalDiscordGuildsStore is the process-wide guild + server-link + user-link
// store, wired at startup like globalServersStore. Nil when the unified store
// failed to open.
var globalDiscordGuildsStore *discordGuildsStore

// ── discord_guilds (roles) ─────────────────────────────────────────────────────

func scanDiscordGuild(s interface{ Scan(...any) error }) (discordGuild, error) {
	var g discordGuild
	if err := s.Scan(&g.GuildID, &g.RolesViewer, &g.RolesEconomy, &g.RolesAdmin); err != nil {
		return discordGuild{}, err
	}
	return g, nil
}

const discordGuildCols = `guild_id, roles_viewer, roles_economy, roles_admin`

// listGuilds returns every guild's roles config in stable order.
func (s *discordGuildsStore) listGuilds() ([]discordGuild, error) {
	rows, err := s.db.Query(`SELECT ` + discordGuildCols + ` FROM discord_guilds ORDER BY guild_id`)
	if err != nil {
		return nil, fmt.Errorf("list discord guilds: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []discordGuild
	for rows.Next() {
		g, err := scanDiscordGuild(rows)
		if err != nil {
			return nil, fmt.Errorf("scan discord guild: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// getGuild returns the roles config for guildID. ok=false when none exists.
func (s *discordGuildsStore) getGuild(guildID string) (discordGuild, bool, error) {
	row := s.db.QueryRow(`SELECT `+discordGuildCols+` FROM discord_guilds WHERE guild_id = ?`, guildID)
	g, err := scanDiscordGuild(row)
	if errors.Is(err, sql.ErrNoRows) {
		return discordGuild{}, false, nil
	}
	if err != nil {
		return discordGuild{}, false, fmt.Errorf("get discord guild %q: %w", guildID, err)
	}
	return g, true, nil
}

// upsertGuild inserts or updates a guild's roles config, preserving created_at.
func (s *discordGuildsStore) upsertGuild(g discordGuild) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO discord_guilds (guild_id, roles_viewer, roles_economy, roles_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(guild_id) DO UPDATE SET
			roles_viewer  = excluded.roles_viewer,
			roles_economy = excluded.roles_economy,
			roles_admin   = excluded.roles_admin,
			updated_at    = excluded.updated_at`,
		g.GuildID, g.RolesViewer, g.RolesEconomy, g.RolesAdmin, now, now)
	if err != nil {
		return fmt.Errorf("upsert discord guild %q: %w", g.GuildID, err)
	}
	return nil
}

// deleteGuild removes a guild's roles row. Its server links are NOT cascaded
// (they reference servers(id), not the guild); callers that want to unlink a
// server use deleteServerLink.
func (s *discordGuildsStore) deleteGuild(guildID string) error {
	if _, err := s.db.Exec(`DELETE FROM discord_guilds WHERE guild_id = ?`, guildID); err != nil {
		return fmt.Errorf("delete discord guild %q: %w", guildID, err)
	}
	return nil
}

// ── discord_servers (one guild per server) ─────────────────────────────────────

const discordServerCols = `server_id, guild_id, announce_channel_id, status_channel_id,
	status_enabled, status_interval_seconds`

func scanServerLink(s interface{ Scan(...any) error }) (discordServerLink, error) {
	var link discordServerLink
	var statusEnabled int
	if err := s.Scan(&link.ServerID, &link.GuildID, &link.AnnounceChannelID, &link.StatusChannelID,
		&statusEnabled, &link.StatusIntervalSeconds); err != nil {
		return discordServerLink{}, err
	}
	link.StatusEnabled = statusEnabled != 0
	return link, nil
}

// getServerLink returns the guild + channel link for one server. ok=false when
// the server is not linked to any guild.
func (s *discordGuildsStore) getServerLink(serverID int) (discordServerLink, bool, error) {
	row := s.db.QueryRow(`SELECT `+discordServerCols+` FROM discord_servers WHERE server_id = ?`, serverID)
	link, err := scanServerLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return discordServerLink{}, false, nil
	}
	if err != nil {
		return discordServerLink{}, false, fmt.Errorf("get server link %d: %w", serverID, err)
	}
	return link, true, nil
}

// serverForChannel resolves the server (and its guild) that owns channelID,
// matching either its announce OR status channel. ok=false when no server claims
// the channel (the command was invoked outside any server's channels).
func (s *discordGuildsStore) serverForChannel(channelID string) (serverID int, guildID string, ok bool, err error) {
	if channelID == "" {
		return 0, "", false, nil
	}
	row := s.db.QueryRow(
		`SELECT server_id, guild_id FROM discord_servers
		 WHERE announce_channel_id = ? OR status_channel_id = ?
		 LIMIT 1`, channelID, channelID)
	scanErr := row.Scan(&serverID, &guildID)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if scanErr != nil {
		return 0, "", false, fmt.Errorf("server for channel %q: %w", channelID, scanErr)
	}
	return serverID, guildID, true, nil
}

// listServerLinks returns every server→guild link in server order.
func (s *discordGuildsStore) listServerLinks() ([]discordServerLink, error) {
	rows, err := s.db.Query(`SELECT ` + discordServerCols + ` FROM discord_servers ORDER BY server_id`)
	if err != nil {
		return nil, fmt.Errorf("list server links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []discordServerLink
	for rows.Next() {
		link, err := scanServerLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan server link: %w", err)
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// serversForGuild returns the server ids linked to a guild (a guild can hold
// many servers).
func (s *discordGuildsStore) serversForGuild(guildID string) ([]int, error) {
	rows, err := s.db.Query(`SELECT server_id FROM discord_servers WHERE guild_id = ? ORDER BY server_id`, guildID)
	if err != nil {
		return nil, fmt.Errorf("servers for guild %q: %w", guildID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan server id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// upsertServerLink inserts or updates the link for one server (server_id PK → a
// server maps to exactly one guild; re-upsert reassigns it).
func (s *discordGuildsStore) upsertServerLink(link discordServerLink) error {
	statusEnabled := 0
	if link.StatusEnabled {
		statusEnabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO discord_servers
			(server_id, guild_id, announce_channel_id, status_channel_id, status_enabled, status_interval_seconds)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET
			guild_id                = excluded.guild_id,
			announce_channel_id     = excluded.announce_channel_id,
			status_channel_id       = excluded.status_channel_id,
			status_enabled          = excluded.status_enabled,
			status_interval_seconds = excluded.status_interval_seconds`,
		link.ServerID, link.GuildID, link.AnnounceChannelID, link.StatusChannelID,
		statusEnabled, link.StatusIntervalSeconds)
	if err != nil {
		return fmt.Errorf("upsert server link %d→%q: %w", link.ServerID, link.GuildID, err)
	}
	return nil
}

// deleteServerLink unlinks one server from its guild.
func (s *discordGuildsStore) deleteServerLink(serverID int) error {
	if _, err := s.db.Exec(`DELETE FROM discord_servers WHERE server_id = ?`, serverID); err != nil {
		return fmt.Errorf("delete server link %d: %w", serverID, err)
	}
	return nil
}

// ── discord_user_links (one character per (user, server)) ──────────────────────

// upsertUserLink writes the link for (discordUserID, serverID), replacing any
// prior character that user registered ON THAT SERVER. The same user can hold a
// separate character per server.
func (s *discordGuildsStore) upsertUserLink(discordUserID string, serverID int, accountID int64, charName, avatarURL string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO discord_user_links
			(discord_user_id, server_id, account_id, character_name, avatar_url, registered_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(discord_user_id, server_id) DO UPDATE SET
			account_id     = excluded.account_id,
			character_name = excluded.character_name,
			avatar_url     = excluded.avatar_url,
			registered_at  = excluded.registered_at`,
		discordUserID, serverID, accountID, charName, avatarURL, now)
	if err != nil {
		return fmt.Errorf("upsert user link %q/%d: %w", discordUserID, serverID, err)
	}
	return nil
}

// getUserLink returns the user's character on serverID. ok=false when the user
// has not registered a character on that server.
func (s *discordGuildsStore) getUserLink(discordUserID string, serverID int) (accountID int64, charName string, ok bool, err error) {
	row := s.db.QueryRow(
		`SELECT account_id, character_name FROM discord_user_links WHERE discord_user_id = ? AND server_id = ?`,
		discordUserID, serverID)
	scanErr := row.Scan(&accountID, &charName)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if scanErr != nil {
		return 0, "", false, fmt.Errorf("get user link %q/%d: %w", discordUserID, serverID, scanErr)
	}
	return accountID, charName, true, nil
}

// deleteUserLink removes the user's character on serverID. ok=true when a row was
// deleted.
func (s *discordGuildsStore) deleteUserLink(discordUserID string, serverID int) (bool, error) {
	res, err := s.db.Exec(
		`DELETE FROM discord_user_links WHERE discord_user_id = ? AND server_id = ?`, discordUserID, serverID)
	if err != nil {
		return false, fmt.Errorf("delete user link %q/%d: %w", discordUserID, serverID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete user link rows %q/%d: %w", discordUserID, serverID, err)
	}
	return n > 0, nil
}

// userLinkInfo carries the Discord enrichment for a player row.
type userLinkInfo struct {
	discordUserID string
	avatar        string
}

// userLinksForServer returns account_id → Discord info for every link on
// serverID, used to enrich the per-server player list in Go.
func (s *discordGuildsStore) userLinksForServer(serverID int) (map[int64]userLinkInfo, error) {
	rows, err := s.db.Query(
		`SELECT account_id, discord_user_id, avatar_url FROM discord_user_links WHERE server_id = ?`,
		serverID)
	if err != nil {
		return nil, fmt.Errorf("user links for server %d: %w", serverID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]userLinkInfo)
	for rows.Next() {
		var accountID int64
		var info userLinkInfo
		if err := rows.Scan(&accountID, &info.discordUserID, &info.avatar); err != nil {
			return nil, fmt.Errorf("scan user link: %w", err)
		}
		out[accountID] = info
	}
	return out, rows.Err()
}
