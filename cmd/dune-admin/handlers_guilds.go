package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// @Summary List all guilds with member count + faction name
// @Tags guilds
// @Produce json
// @Success 200 {array} guildSummary
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds [get]
func handleListGuilds(w http.ResponseWriter, r *http.Request) {
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	guilds, err := cmdFetchGuilds(r.Context(), globalDB)
	if err != nil {
		log.Printf("handleListGuilds: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, guilds)
}

// @Summary Get one guild with its members and pending invites
// @Tags guilds
// @Produce json
// @Param id path int true "Guild ID"
// @Success 200 {object} guildDetail
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds/{id} [get]
func handleGetGuild(w http.ResponseWriter, r *http.Request) {
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid guild id"), http.StatusBadRequest)
		return
	}
	detail, err := cmdFetchGuildDetail(r.Context(), globalDB, id)
	if err != nil {
		if errors.Is(err, errGuildNotFound) {
			jsonErr(w, fmt.Errorf("guild not found"), http.StatusNotFound)
			return
		}
		log.Printf("handleGetGuild: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, detail)
}
