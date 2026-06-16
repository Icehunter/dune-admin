package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

// discordMemberRow is the JSON shape returned by the member search endpoint,
// used by the settings UI to pick owner user IDs.
type discordMemberRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
}

// memberSearchFn matches discordgo.Session.GuildMembersSearch for injection.
type memberSearchFn func(guildID, query string, limit int) ([]*discordgo.Member, error)

// handleSearchDiscordMembers searches guild members by username/nickname
// prefix via plain bot-token REST — no gateway connection required, so it
// works even while the event bot is still starting up.
//
// @Summary Search Discord guild members by name prefix
// @Tags discord
// @Produce json
// @Param q query string true "Name prefix to search"
// @Success 200 {array} discordMemberRow
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/discord/members/search [get]
func handleSearchDiscordMembers(w http.ResponseWriter, r *http.Request) {
	cfg := loadedConfig
	if cfg.DiscordBotToken == "" || cfg.DiscordGuildID == "" {
		jsonErr(w, errors.New("discord bot token + guild id are not configured"), http.StatusServiceUnavailable)
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonErr(w, errors.New("query parameter q is required"), http.StatusBadRequest)
		return
	}
	sess, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		jsonErr(w, errors.New("discord session init failed"), http.StatusInternalServerError)
		return
	}
	handleSearchDiscordMembersInner(w, cfg.DiscordGuildID, query, func(gID, q string, limit int) ([]*discordgo.Member, error) {
		return sess.GuildMembersSearch(gID, q, limit)
	})
}

func handleSearchDiscordMembersInner(w http.ResponseWriter, guildID, query string, search memberSearchFn) {
	members, err := search(guildID, query, 10)
	if err != nil {
		componentLog("discord").Error().Err(err).Msg("handleSearchDiscordMembers failed")
		jsonErr(w, fmt.Errorf("member search failed"), http.StatusInternalServerError)
		return
	}
	rows := make([]discordMemberRow, 0, len(members))
	for _, m := range members {
		if m.User == nil {
			continue
		}
		row := discordMemberRow{ID: m.User.ID, Name: m.Nick, Username: m.User.Username}
		if row.Name == "" {
			row.Name = m.User.Username
		}
		if m.User.Avatar != "" {
			row.Avatar = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=32", m.User.ID, m.User.Avatar)
		}
		rows = append(rows, row)
	}
	jsonOK(w, rows)
}
