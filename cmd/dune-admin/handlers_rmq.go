package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// All handlers in this file publish RabbitMQ server commands.
// They are fire-and-forget — the game server applies the command and logs the
// result. The HTTP response indicates whether the command was sent, not whether
// the game server executed it successfully.

// @Summary Send kick command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/kick [post]
// POST /api/v1/players/kick
func handleRMQKickPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID string `json:"fls_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), 400)
		return
	}
	if err := rmqKickPlayer(req.FlsID); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("kick command sent for %s", req.FlsID)})
}

// fillWaterParams bundles the injectable dependencies for processFillWater so
// the online/offline branching can be unit-tested without a live DB or broker.
type fillWaterParams struct {
	flsID        string
	waterAmount  int
	isOnline     func(flsID string) bool
	sendRMQ      func(flsID string, waterAmount int) error
	resolveActor func(flsID string) (int64, error)
	refillDB     func(actorID int64) (int64, error)
}

// processFillWater fills water containers for the given player. Online players
// receive an immediate RMQ command; offline players get a DB write that takes
// effect on their next relog. The zero water-amount defaults to 1 000 000.
func processFillWater(p fillWaterParams) error {
	if p.waterAmount <= 0 {
		p.waterAmount = 1000000
	}
	if p.isOnline(p.flsID) {
		return p.sendRMQ(p.flsID, p.waterAmount)
	}
	actorID, err := p.resolveActor(p.flsID)
	if err != nil {
		return fmt.Errorf("resolve player: %w", err)
	}
	_, err = p.refillDB(actorID)
	return err
}

// @Summary Send fill-water command via RabbitMQ (player must be online)
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID and optional water amount"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 422 {object} map[string]string "Player must be online"
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/fill-water [post]
// POST /api/v1/players/fill-water
func handleRMQFillWater(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID       string `json:"fls_id"`
		WaterAmount int    `json:"water_amount"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	pool := dbFromCtx(r)
	err := processFillWater(fillWaterParams{
		flsID:       req.FlsID,
		waterAmount: req.WaterAmount,
		isOnline:    func(id string) bool { return isHexIDOnline(ctx, id) },
		sendRMQ:     func(id string, amt int) error { return rmqUpdateAllWaterFillables(id, amt) },
		resolveActor: func(id string) (int64, error) {
			return cmdActorIDFromFlsID(ctx, pool, id)
		},
		refillDB: func(actorID int64) (int64, error) {
			return cmdRefillWaterOffline(ctx, pool, actorID)
		},
	})
	if err != nil {
		componentLog("rmq").Error().Err(err).Msg("fill water failed")
		jsonErr(w, fmt.Errorf("fill water failed: %w", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("fill water command sent for %s", req.FlsID)})
}

// @Summary Send set-skill-points command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID and skill points value"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/set-skill-points [post]
// POST /api/v1/players/set-skill-points
func handleRMQSetSkillPoints(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID       string `json:"fls_id"`
		SkillPoints int    `json:"skill_points"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), 400)
		return
	}
	if err := rmqSkillsSetUnspentSkillPoints(req.FlsID, req.SkillPoints); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("set skill points %d sent for %s", req.SkillPoints, req.FlsID)})
}

// @Summary Send server-wide broadcast message via RabbitMQ
// @Tags broadcast
// @Accept json
// @Produce json
// @Param body body object true "Localized texts and optional duration in seconds"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/broadcast [post]
// POST /api/v1/broadcast
func handleRMQBroadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DurationSec int             `json:"duration_sec"`
		Texts       []localizedText `json:"texts"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if len(req.Texts) == 0 {
		jsonErr(w, fmt.Errorf("texts required"), 400)
		return
	}
	if req.DurationSec <= 0 {
		req.DurationSec = 30
	}
	if err := rmqServiceBroadcastGeneric(req.DurationSec, req.Texts); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": "broadcast sent"})
}

// @Summary Send shutdown broadcast command via RabbitMQ
// @Tags broadcast
// @Accept json
// @Produce json
// @Param body body object true "Shutdown type, delay, frequency, duration, and cancel flag"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/broadcast/shutdown [post]
// POST /api/v1/broadcast/shutdown
func handleRMQBroadcastShutdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ShutdownType string `json:"shutdown_type"` // "Restart", "Maintenance", or "Update"
		DelayMinutes int    `json:"delay_minutes"`
		Frequency    int    `json:"frequency"`
		Duration     int    `json:"duration"`
		Cancel       bool   `json:"cancel"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.ShutdownType == "" {
		req.ShutdownType = "Restart"
	}
	// A cancel supersedes any pending action; clear it before re-broadcasting.
	if req.Cancel {
		cancelBroadcastShutdownExec()
	}
	ts := time.Now().Add(time.Duration(req.DelayMinutes) * time.Minute).Unix()
	if err := rmqServiceBroadcastShutdown(req.ShutdownType, ts, req.Frequency, req.Duration, req.Cancel); err != nil {
		jsonErr(w, err, 500)
		return
	}
	// The broadcast above only shows the in-game countdown. Arm the actual
	// control-plane action so the server is genuinely restarted/stopped when the
	// countdown elapses (#205 — previously nothing happened in game).
	if !req.Cancel {
		delay := time.Duration(req.DelayMinutes) * time.Minute
		scheduleBroadcastShutdownExec(delay, shutdownVerb(req.ShutdownType), controlFromCtx(r), executorFromCtx(r))
	}
	action := "shutdown broadcast sent"
	if req.Cancel {
		action = "shutdown broadcast cancelled"
	}
	jsonOK(w, map[string]string{"ok": action})
}

// pending broadcast-shutdown control-plane action. A new broadcast or a cancel
// supersedes whatever was previously armed. shutdownExecAt is the Unix time the
// action will fire (0 when nothing is armed) — exposed via /status so the UI can
// rehydrate the pending state and Cancel button after a browser refresh.
var (
	shutdownExecMu    sync.Mutex
	shutdownExecTimer *time.Timer
	shutdownExecAt    int64
)

// shutdownVerb maps a broadcast shutdown type to the control-plane lifecycle
// command. "Restart" cycles the server; maintenance/update windows stop it.
func shutdownVerb(shutdownType string) string {
	if strings.EqualFold(shutdownType, "Restart") {
		return "restart"
	}
	return "stop"
}

// scheduleBroadcastShutdownExec arms a one-shot timer to run the control-plane
// action after delay, cancelling any previously-armed action first. ctrl and
// exec are captured at call time so the goroutine uses the right server.
func scheduleBroadcastShutdownExec(delay time.Duration, verb string, ctrl ControlPlane, exec Executor) {
	shutdownExecMu.Lock()
	defer shutdownExecMu.Unlock()
	if shutdownExecTimer != nil {
		shutdownExecTimer.Stop()
	}
	shutdownExecAt = time.Now().Add(delay).Unix()
	shutdownExecTimer = time.AfterFunc(delay, func() {
		shutdownExecMu.Lock()
		shutdownExecTimer = nil
		shutdownExecAt = 0
		shutdownExecMu.Unlock()
		fireBroadcastShutdown(context.Background(), verb, ctrl, exec)
	})
}

// cancelBroadcastShutdownExec stops any armed control-plane action.
func cancelBroadcastShutdownExec() {
	shutdownExecMu.Lock()
	defer shutdownExecMu.Unlock()
	if shutdownExecTimer != nil {
		shutdownExecTimer.Stop()
		shutdownExecTimer = nil
	}
	shutdownExecAt = 0
}

// pendingBroadcastShutdown reports whether a broadcast-shutdown action is armed
// and, if so, the Unix time it fires. Read by /status for UI rehydration.
func pendingBroadcastShutdown() (at int64, pending bool) {
	shutdownExecMu.Lock()
	defer shutdownExecMu.Unlock()
	return shutdownExecAt, shutdownExecAt > 0
}

// fireBroadcastShutdown runs the lifecycle command against the control plane.
// Mirrors fireScheduledRestart: a no-op (logged) when nothing is connected.
func fireBroadcastShutdown(ctx context.Context, verb string, ctrl ControlPlane, exec Executor) {
	if ctrl == nil || exec == nil {
		componentLog("rmq").Warn().Str("verb", verb).Msg("broadcast-shutdown: control plane not connected; skipped")
		return
	}
	if _, err := ctrl.ExecCommand(ctx, exec, verb); err != nil {
		componentLog("rmq").Error().Err(err).Str("verb", verb).Msg("broadcast-shutdown failed")
	}
}

// @Summary Send cheat script command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID and script name"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/cheat-script [post]
// POST /api/v1/players/cheat-script
func handleRMQCheatScript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID      string `json:"fls_id"`
		ScriptName string `json:"script_name"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" || req.ScriptName == "" {
		jsonErr(w, fmt.Errorf("fls_id and script_name required"), 400)
		return
	}
	if err := rmqCheatScript(req.FlsID, req.ScriptName); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("cheat script %q sent for %s", req.ScriptName, req.FlsID)})
}

// @Summary Send give-item command via RabbitMQ to an online player
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Actor ID, item template, quantity, and durability"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/give-item-live [post]
// POST /api/v1/players/give-item-live
// Give item to an ONLINE player via RMQ. Pre-checks weight/slot limits via DB.
// Accepts actor_id (player pawn ID), resolves to FLS ID automatically.
func handleRMQGiveItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   int64   `json:"player_id"` // actor (pawn) ID
		Template   string  `json:"template"`
		Qty        int     `json:"qty"`
		Durability float64 `json:"durability"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.PlayerID == 0 || req.Template == "" {
		jsonErr(w, fmt.Errorf("player_id and template required"), 400)
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Durability <= 0 {
		req.Durability = 1.0
	}

	ctx := context.Background()

	// Check weight/slot capacity before sending to avoid bypassing limits.
	if err := checkInventoryCapacity(ctx, req.PlayerID, req.Template, int64(req.Qty)); err != nil {
		jsonErr(w, err, 400)
		return
	}

	flsID, err := flsIDFromActorID(ctx, req.PlayerID)
	if err != nil {
		jsonErr(w, fmt.Errorf("resolve player: %w", err), 404)
		return
	}

	if err := rmqAddItemToInventory(flsID, req.Template, req.Qty, req.Durability); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{
		"ok":   fmt.Sprintf("sent %d × %s to online player %d via server command", req.Qty, req.Template, req.PlayerID),
		"path": "rmq",
	})
}

// @Summary Send clean-inventory command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/clean-inventory [post]
// POST /api/v1/players/clean-inventory
func handleRMQCleanInventory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID string `json:"fls_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), 400)
		return
	}
	if err := rmqCleanPlayerInventory(req.FlsID); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("clean inventory command sent for %s", req.FlsID)})
}

// @Summary Send reset-progression command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/reset-progression [post]
// POST /api/v1/players/reset-progression
func handleRMQResetProgression(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID string `json:"fls_id"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" {
		jsonErr(w, fmt.Errorf("fls_id required"), 400)
		return
	}
	if err := rmqResetProgression(req.FlsID); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("reset progression command sent for %s", req.FlsID)})
}

// @Summary Send set-skill-module command via RabbitMQ
// @Tags players
// @Accept json
// @Produce json
// @Param body body object true "Player FLS ID, module name, and level"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/players/set-skill-module [post]
// POST /api/v1/players/set-skill-module
func handleRMQSetSkillModule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID  string `json:"fls_id"`
		Module string `json:"module"`
		Level  int    `json:"level"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" || req.Module == "" {
		jsonErr(w, fmt.Errorf("fls_id and module required"), 400)
		return
	}
	if err := rmqSkillsSetModuleLevel(req.FlsID, req.Module, req.Level); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("set module %s level %d sent for %s", req.Module, req.Level, req.FlsID)})
}

// @Summary Send vehicle spawn command via RabbitMQ
// @Tags vehicles
// @Accept json
// @Produce json
// @Param body body object true "FLS ID, class name, coordinates, rotation, template, persistence, and faction"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/vehicles/spawn [post]
// POST /api/v1/vehicles/spawn
func handleRMQSpawnVehicle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FlsID        string  `json:"fls_id"`
		ClassName    string  `json:"class_name"`
		X            float64 `json:"x"`
		Y            float64 `json:"y"`
		Z            float64 `json:"z"`
		Rotation     float64 `json:"rotation"`
		TemplateName string  `json:"template_name"`
		Persistent   bool    `json:"persistent"`
		Faction      string  `json:"faction"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.FlsID == "" || req.ClassName == "" {
		jsonErr(w, fmt.Errorf("fls_id and class_name required"), 400)
		return
	}
	if err := rmqSpawnVehicleAt(req.FlsID, req.ClassName, req.X, req.Y, req.Z, req.Rotation, req.TemplateName, req.Persistent, req.Faction); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("spawn %s command sent for %s", req.ClassName, req.FlsID)})
}

// whisperDeps are the injected dependencies for processWhisper so the
// orchestration can be unit-tested without a live DB or broker.
type whisperDeps struct {
	getGM        func(context.Context) (gmIdentity, error)
	resolveRecip func(ctx context.Context, accountID int64) (funcomID, charName string, err error)
	send         func(senderFuncomID, senderHexID, recipientFuncomID, recipientName, message string) error
}

// processWhisper resolves the GM/Server sender and the recipient identities, then
// sends the whisper. The seeded GM persona is the sender (its funcom id and hex
// FLS id); the recipient is looked up by account id. Returns the underlying error
// so the handler can map errGMNotProvisioned to 503.
func processWhisper(ctx context.Context, accountID int64, message string, d whisperDeps) error {
	gm, err := d.getGM(ctx)
	if err != nil {
		return err
	}
	recipientFuncomID, recipientName, err := d.resolveRecip(ctx, accountID)
	if err != nil {
		return err
	}
	return d.send(gm.FuncomID, gm.HexID, recipientFuncomID, recipientName, message)
}

// @Summary Send a whisper to a player from the GM/Server persona
// @Tags chat
// @Accept json
// @Produce json
// @Param body body object true "Recipient account id and message"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/chat/whisper [post]
// POST /api/v1/chat/whisper
//
// Sends a private chat message to one player, shown in their Whispers tab and
// attributed to the seeded GM/Server persona. The exact wire shape is pinned by
// buildWhisperBody against the live-confirmed protocol.
func handleRMQWhisper(w http.ResponseWriter, r *http.Request) {
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		AccountID int64  `json:"account_id"`
		Message   string `json:"message"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.AccountID == 0 || req.Message == "" {
		jsonErr(w, fmt.Errorf("account_id and message required"), 400)
		return
	}

	err := processWhisper(r.Context(), req.AccountID, req.Message, whisperDeps{
		getGM:        cmdGetGMIdentity,
		resolveRecip: cmdResolveRecipientChatIdentity,
		send:         rmqSendWhisper,
	})
	if errors.Is(err, errGMNotProvisioned) {
		jsonErr(w, err, http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		componentLog("rmq").Error().Err(err).Msg("failed to send whisper")
		jsonErr(w, fmt.Errorf("failed to send whisper"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": fmt.Sprintf("whisper sent to account %d", req.AccountID)})
}

// @Summary Resolve actor ID to both ID forms and render a sample RMQ envelope
// @Tags players
// @Produce json
// @Param id path int true "Actor (pawn) ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/players/{id}/player-ids [get]
// Returns both ID forms for an actor so you can verify which PlayerId the
// game server would receive. Also renders a sample AddItemToInventory envelope
// (without sending it) so the exact message format can be confirmed.
func handlePlayerIDDebug(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	actorID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid actor id %q", idStr), 400)
		return
	}

	ctx := context.Background()
	ids, err := playerIDDebug(ctx, actorID)
	if err != nil {
		jsonErr(w, err, 404)
		return
	}

	// Build a sample envelope to show what the game server receives.
	inner, _ := json.Marshal(map[string]any{
		"ServerCommand": "AddItemToInventory",
		"PlayerId":      ids["hex_id"],
		"ItemName":      "<item_template>",
		"Quantity":      1,
		"Durability":    1.0,
	})
	outer := map[string]any{
		"Version":        2,
		"AuthToken":      serverCmdAuthToken,
		"MessageContent": string(inner),
	}

	jsonOK(w, map[string]any{
		"actor_id":        actorID,
		"display_name":    ids["display_name"],
		"hex_id":          ids["hex_id"],
		"player_id_field": ids["hex_id"],
		"auth_token":      serverCmdAuthToken,
		"publish_method":  "rabbitmqctl eval (user_id=fls)",
		"sample_envelope": outer,
	})
}
