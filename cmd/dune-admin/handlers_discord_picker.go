package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// discordGuildOption is one selectable guild the bot belongs to. The UI uses it
// to offer a "pick a guild" dropdown instead of pasting raw snowflake ids.
type discordGuildOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// discordChannelOption is one postable text channel in a guild, used to populate
// the searchable announce/status channel pickers.
type discordChannelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userGuildsFetchFn matches discordgo.Session.UserGuilds (narrowed) for tests.
type userGuildsFetchFn func() ([]*discordgo.UserGuild, error)

// guildChannelsFetchFn matches discordgo.Session.GuildChannels for tests.
type guildChannelsFetchFn func(guildID string) ([]*discordgo.Channel, error)

// cmdListDiscordGuilds maps the bot's guild membership to id/name options.
func cmdListDiscordGuilds(fetch userGuildsFetchFn) ([]discordGuildOption, error) {
	raw, err := fetch()
	if err != nil {
		return nil, fmt.Errorf("fetch bot guilds: %w", err)
	}
	out := make([]discordGuildOption, 0, len(raw))
	for _, g := range raw {
		out = append(out, discordGuildOption{ID: g.ID, Name: g.Name})
	}
	return out, nil
}

// cmdListDiscordChannels lists a guild's postable channels (text + announcement),
// dropping voice/category/etc. so the picker only offers valid message targets.
func cmdListDiscordChannels(guildID string, fetch guildChannelsFetchFn) ([]discordChannelOption, error) {
	raw, err := fetch(guildID)
	if err != nil {
		return nil, fmt.Errorf("fetch guild channels: %w", err)
	}
	out := make([]discordChannelOption, 0, len(raw))
	for _, c := range raw {
		if c.Type != discordgo.ChannelTypeGuildText && c.Type != discordgo.ChannelTypeGuildNews {
			continue
		}
		out = append(out, discordChannelOption{ID: c.ID, Name: c.Name})
	}
	return out, nil
}

// bestDiscordSession returns the live gateway session when the bot is running,
// otherwise a REST-only session built from the configured bot token. The second
// value is the configured default guild id (may be empty). A nil session means
// neither a running bot nor a usable token is available.
func bestDiscordSession() (*discordgo.Session, string) {
	if sess, guildID := getDiscordState(); sess != nil {
		return sess, guildID
	}
	cfg := loadedConfig
	if cfg.DiscordBotToken == "" {
		return nil, ""
	}
	sess, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		return nil, ""
	}
	return sess, cfg.DiscordGuildID
}

// handleGetDiscordAvailableGuilds returns the guilds the bot is a member of so
// the UI can offer a name-labelled guild dropdown (id + human name).
func handleGetDiscordAvailableGuilds(w http.ResponseWriter, _ *http.Request) {
	sess, _ := bestDiscordSession()
	if sess == nil {
		jsonErr(w, errDiscordNotConnected, http.StatusServiceUnavailable)
		return
	}
	guilds, err := cmdListDiscordGuilds(func() ([]*discordgo.UserGuild, error) {
		// 200 is the Discord max per page; one page covers any realistic bot.
		return sess.UserGuilds(200, "", "", false)
	})
	if err != nil {
		componentLog("discord").Error().Err(err).Msg("handleGetDiscordAvailableGuilds failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, guilds)
}

// handleGetDiscordChannels returns a guild's postable channels for the announce/
// status channel pickers. The guild is taken from ?guild=<id>, falling back to
// the configured default guild.
func handleGetDiscordChannels(w http.ResponseWriter, r *http.Request) {
	sess, defaultGuild := bestDiscordSession()
	if sess == nil {
		jsonErr(w, errDiscordNotConnected, http.StatusServiceUnavailable)
		return
	}
	guildID := strings.TrimSpace(r.URL.Query().Get("guild"))
	if guildID == "" {
		guildID = defaultGuild
	}
	if guildID == "" {
		jsonErr(w, fmt.Errorf("guild is required"), http.StatusBadRequest)
		return
	}
	channels, err := cmdListDiscordChannels(guildID, func(id string) ([]*discordgo.Channel, error) {
		return sess.GuildChannels(id)
	})
	if err != nil {
		componentLog("discord").Error().Err(err).Msg("handleGetDiscordChannels failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, channels)
}
