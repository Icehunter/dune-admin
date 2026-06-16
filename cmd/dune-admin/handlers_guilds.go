package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errEmptyGuildName = errors.New("guild name must not be empty")

// @Summary Create a new NPC guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param body body map[string]interface{} true "Guild definition"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds [post]
func handleCreateGuild(w http.ResponseWriter, r *http.Request) {
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		FactionID   int    `json:"faction_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, fmt.Errorf("invalid json"), http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		jsonErr(w, errEmptyGuildName, http.StatusBadRequest)
		return
	}
	id, err := cmdCreateGuild(r.Context(), globalDB, req.Name, req.Description, req.FactionID)
	if err != nil {
		log.Printf("handleCreateGuild: %v", err)
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"guild_id": id})
}

// @Summary List all guilds with member count + faction name
// @Tags guilds
// @Produce json
// @Success 200 {array} guildSummary
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds [get]
func handleListGuilds(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	guilds, err := cmdFetchGuilds(r.Context(), db)
	if err != nil {
		componentLog("handlers").Error().Err(err).Msg("fetch guilds failed")
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
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid guild id"), http.StatusBadRequest)
		return
	}
	detail, err := cmdFetchGuildDetail(r.Context(), db, id)
	if err != nil {
		if errors.Is(err, errGuildNotFound) {
			jsonErr(w, fmt.Errorf("guild not found"), http.StatusNotFound)
			return
		}
		componentLog("handlers").Error().Int64("guild_id", id).Err(err).Msg("fetch guild detail failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, detail)
}

// applyGuildUpdate applies the provided (optional) name/description edits. Returns
// sentinel errors (errEmptyGuildName / errGuildNameTaken / errGuildNotFound) that
// the handler maps to HTTP statuses.
func applyGuildUpdate(r *http.Request, db *pgxpool.Pool, id int64, name, desc *string) error {
	if desc != nil {
		if err := cmdEditGuildDescription(r.Context(), db, id, *desc); err != nil {
			return err
		}
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errEmptyGuildName
		}
		if err := cmdEditGuildName(r.Context(), db, id, n); err != nil {
			return err
		}
	}
	return nil
}

func writeGuildUpdateErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errEmptyGuildName):
		jsonErr(w, fmt.Errorf("guild name must not be empty"), http.StatusBadRequest)
	case errors.Is(err, errGuildNameTaken):
		jsonErr(w, fmt.Errorf("guild name already taken"), http.StatusConflict)
	case errors.Is(err, errGuildNotFound):
		jsonErr(w, fmt.Errorf("guild not found"), http.StatusNotFound)
	default:
		componentLog("handlers").Error().Err(err).Msg("update guild failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
	}
}

// @Summary Edit a guild's name and/or description
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path int true "Guild ID"
// @Success 200 {object} guildDetail
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds/{id} [patch]
func handleUpdateGuild(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid guild id"), http.StatusBadRequest)
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.Name == nil && body.Description == nil {
		jsonErr(w, fmt.Errorf("nothing to update"), http.StatusBadRequest)
		return
	}
	if err := applyGuildUpdate(r, db, id, body.Name, body.Description); err != nil {
		writeGuildUpdateErr(w, err)
		return
	}
	detail, err := cmdFetchGuildDetail(r.Context(), db, id)
	if err != nil {
		writeGuildUpdateErr(w, err)
		return
	}
	jsonOK(w, detail)
}

// @Summary Set a guild member's role (50 = member, 100 = admin)
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path int true "Guild ID"
// @Param pid path int true "Member player (actor) ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/guilds/{id}/members/{pid}/role [put]
func handleSetGuildMemberRole(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid guild id"), http.StatusBadRequest)
		return
	}
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid player id"), http.StatusBadRequest)
		return
	}
	var body struct {
		Role int16 `json:"role"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if body.Role != guildRoleMember && body.Role != guildRoleAdmin {
		jsonErr(w, fmt.Errorf("role must be %d (member) or %d (admin)", guildRoleMember, guildRoleAdmin), http.StatusBadRequest)
		return
	}
	if err := cmdSetGuildMemberRole(r.Context(), db, id, pid, body.Role); err != nil {
		// The game procs raise on invalid transitions (e.g. demoting the sitting
		// admin). Surface a hint; log the detail.
		componentLog("handlers").Warn().Int64("guild_id", id).Int64("player_id", pid).Err(err).Msg("set guild member role rejected")
		jsonErr(w, fmt.Errorf("role change rejected — to change the admin, promote another member to admin first"), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]string{"ok": "role updated"})
}
