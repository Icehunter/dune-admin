package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// handlers_character_backups.go exposes the native-transfer character backup
// & restore feature (cmdCaptureCharacterBackup / cmdRestoreCharacterBackup,
// db.go) over HTTP: create a backup, list a player's backups, restore one,
// download its raw transfer JSON, or delete it.

// characterBackupsStoreForCtx returns the character-backups store scoped to
// the request's server, mirroring battlepassStoreForCtx. Returns nil when the
// store is unavailable.
func characterBackupsStoreForCtx(r *http.Request) *characterBackupsStore {
	if globalCharacterBackupsStore == nil {
		return nil
	}
	return globalCharacterBackupsStore.withScope(storeScopeFromCtx(r))
}

// @Summary Create a full character backup via the native transfer export
// @Tags players
// @Accept json
// @Produce json
// @Param id path int true "Account ID"
// @Param body body object true "character_name, reason"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/{id}/backup [post]
func handleBackupCharacter(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	var req struct {
		CharacterName string `json:"character_name"`
		Reason        string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual backup"
	}
	store := characterBackupsStoreForCtx(r)
	if err := cmdCaptureCharacterBackup(r.Context(), dbFromCtx(r), store, accountID, req.CharacterName, "manual", reason); err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("capture character backup failed")
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "backup created"})
}

// @Summary List a player's character backups
// @Tags players
// @Produce json
// @Param id path int true "Account ID"
// @Success 200 {array} characterBackup
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/players/{id}/backups [get]
func handleListCharacterBackups(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	store := characterBackupsStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("character backups store not available"), http.StatusServiceUnavailable)
		return
	}
	backups, err := store.listForAccount(accountID)
	if err != nil {
		componentLog("handlers").Error().Int64("account_id", accountID).Err(err).Msg("list character backups failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, backups)
}

// @Summary Restore a character from a backup (DESTRUCTIVE — full replace)
// @Tags players
// @Accept json
// @Produce json
// @Param id path int true "Backup ID"
// @Param body body object true "confirm"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/character-backups/{id}/restore [post]
func handleRestoreCharacterBackup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if !req.Confirm {
		jsonErr(w, fmt.Errorf("restore requires confirm=true"), http.StatusBadRequest)
		return
	}
	store := characterBackupsStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("character backups store not available"), http.StatusServiceUnavailable)
		return
	}
	newID, err := cmdRestoreCharacterBackup(r.Context(), dbFromCtx(r), store, id)
	if err != nil {
		componentLog("handlers").Error().Int64("backup_id", id).Err(err).Msg("restore character backup failed")
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": "character restored", "player_controller_id": newID})
}

// @Summary Download a character backup's raw transfer JSON
// @Tags players
// @Produce octet-stream
// @Param id path int true "Backup ID"
// @Success 200 {string} string "character-backup-{id}.json attachment"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/character-backups/{id}/download [get]
func handleDownloadCharacterBackup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	store := characterBackupsStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("character backups store not available"), http.StatusServiceUnavailable)
		return
	}
	b, err := store.get(id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			jsonErr(w, fmt.Errorf("backup not found"), http.StatusNotFound)
			return
		}
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	data, err := os.ReadFile(b.FilePath) // #nosec G304 -- path comes from a stored backup record written by writeCharacterBackupFile, never request input directly
	if err != nil {
		jsonErr(w, fmt.Errorf("backup file not found"), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="character-backup-%d.json"`, id))
	_, _ = w.Write(data)
}

// @Summary Delete a character backup (record + file)
// @Tags players
// @Produce json
// @Param id path int true "Backup ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/character-backups/{id} [delete]
func handleDeleteCharacterBackup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	store := characterBackupsStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("character backups store not available"), http.StatusServiceUnavailable)
		return
	}
	b, err := store.get(id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			jsonErr(w, fmt.Errorf("backup not found"), http.StatusNotFound)
			return
		}
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	if err := store.delete(id); err != nil {
		componentLog("handlers").Error().Int64("backup_id", id).Err(err).Msg("delete character backup failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	if rmErr := os.Remove(b.FilePath); rmErr != nil && !os.IsNotExist(rmErr) {
		componentLog("handlers").Warn().Str("path", b.FilePath).Err(rmErr).Msg("failed to remove character backup file")
	}
	jsonOK(w, map[string]string{"ok": "backup deleted"})
}
