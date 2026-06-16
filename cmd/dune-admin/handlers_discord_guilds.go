package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// handlers_discord_guilds.go exposes CRUD for the Discord model:
//
//   - discord_guilds: per-guild AUTH (roles only). GET/POST/DELETE under
//     /api/v1/discord/guilds.
//   - discord_servers: each server links to exactly one guild + its channels.
//     GET/PUT/DELETE under /api/v1/discord/servers.
//
// Each write re-applies the live bot's command registration + status loops via
// applyDiscordGuilds so changes take effect without a bot restart.

// ── discord_guilds (roles) ─────────────────────────────────────────────────────

// handleListDiscordGuilds returns every configured guild with its roles.
func handleListDiscordGuilds(w http.ResponseWriter, _ *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	guilds, err := globalDiscordGuildsStore.listGuilds()
	if err != nil {
		componentLog("discord").Error().Err(err).Msg("list discord guilds failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	if guilds == nil {
		guilds = []discordGuild{}
	}
	jsonOK(w, guilds)
}

// handleUpsertDiscordGuild creates or updates a guild's roles.
func handleUpsertDiscordGuild(w http.ResponseWriter, r *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	var g discordGuild
	if err := decode(r, &g); err != nil {
		jsonErr(w, fmt.Errorf("invalid body: %w", err), http.StatusBadRequest)
		return
	}
	g.GuildID = strings.TrimSpace(g.GuildID)
	if g.GuildID == "" {
		jsonErr(w, fmt.Errorf("guild_id is required"), http.StatusBadRequest)
		return
	}
	if err := globalDiscordGuildsStore.upsertGuild(g); err != nil {
		componentLog("discord").Error().Str("guild_id", g.GuildID).Err(err).Msg("upsert discord guild failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	applyDiscordGuildsAsync()
	jsonOK(w, g)
}

// handleDeleteDiscordGuild removes a guild's roles row and de-registers its
// commands on the live session. Server links referencing the guild are left
// intact (they reference servers, not the guild).
func handleDeleteDiscordGuild(w http.ResponseWriter, r *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	guildID := strings.TrimSpace(r.PathValue("guildID"))
	if guildID == "" {
		jsonErr(w, fmt.Errorf("invalid guild id"), http.StatusBadRequest)
		return
	}
	if err := globalDiscordGuildsStore.deleteGuild(guildID); err != nil {
		componentLog("discord").Error().Str("guild_id", guildID).Err(err).Msg("delete discord guild failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	applyDiscordGuildsAsync(guildID)
	jsonOK(w, map[string]bool{"deleted": true})
}

// ── discord_servers (one guild per server) ─────────────────────────────────────

// handleListDiscordServers returns every server→guild link with its channels.
func handleListDiscordServers(w http.ResponseWriter, _ *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	links, err := globalDiscordGuildsStore.listServerLinks()
	if err != nil {
		componentLog("discord").Error().Err(err).Msg("list discord servers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	if links == nil {
		links = []discordServerLink{}
	}
	jsonOK(w, links)
}

// handleUpsertDiscordServer sets a server's link: its guild + channels + status
// tuning. The path server id is authoritative; the body's server_id is ignored.
func handleUpsertDiscordServer(w http.ResponseWriter, r *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	serverID, err := strconv.Atoi(strings.TrimSpace(r.PathValue("serverID")))
	if err != nil || serverID <= 0 {
		jsonErr(w, fmt.Errorf("invalid server id"), http.StatusBadRequest)
		return
	}
	var link discordServerLink
	if err := decode(r, &link); err != nil {
		jsonErr(w, fmt.Errorf("invalid body: %w", err), http.StatusBadRequest)
		return
	}
	link.ServerID = serverID
	link.GuildID = strings.TrimSpace(link.GuildID)
	if link.GuildID == "" {
		jsonErr(w, fmt.Errorf("guild_id is required"), http.StatusBadRequest)
		return
	}
	if err := globalDiscordGuildsStore.upsertServerLink(link); err != nil {
		componentLog("discord").Error().Int("server_id", serverID).Str("guild_id", link.GuildID).Err(err).Msg("upsert discord server failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	applyDiscordGuildsAsync()
	jsonOK(w, link)
}

// handleDeleteDiscordServer unlinks a server from its guild.
func handleDeleteDiscordServer(w http.ResponseWriter, r *http.Request) {
	if globalDiscordGuildsStore == nil {
		jsonErr(w, errors.New("store not available"), http.StatusServiceUnavailable)
		return
	}
	serverID, err := strconv.Atoi(strings.TrimSpace(r.PathValue("serverID")))
	if err != nil || serverID <= 0 {
		jsonErr(w, fmt.Errorf("invalid server id"), http.StatusBadRequest)
		return
	}
	if err := globalDiscordGuildsStore.deleteServerLink(serverID); err != nil {
		componentLog("discord").Error().Int("server_id", serverID).Err(err).Msg("delete discord server failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	applyDiscordGuildsAsync()
	jsonOK(w, map[string]bool{"deleted": true})
}
