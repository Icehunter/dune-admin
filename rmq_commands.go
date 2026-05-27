package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// serverCmdAuthToken is the static AuthToken the game server validates on
// incoming server command envelopes. Extracted from send-dune-broadcast.
const serverCmdAuthToken = "Nu6VmPWUMvdPMeB7qErr"

// ── core publish ──────────────────────────────────────────────────────────────

// publishServerCommand sends a server command to the game via rabbitmqctl eval
// executed inside the mq-game broker pod. This mirrors send-dune-broadcast:
// the Erlang P_basic tuple sets user_id="fls", which AMQP connections cannot
// do (the broker validates UserId against the authenticated username).
func publishServerCommand(fields map[string]any) error {
	inner, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshal server command: %w", err)
	}

	outer, err := json.Marshal(map[string]any{
		"Version":        2,
		"AuthToken":      serverCmdAuthToken,
		"MessageContent": string(inner),
	})
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Pass the envelope as base64 to avoid any Erlang string-escaping issues.
	outerB64 := base64.StdEncoding.EncodeToString(outer)
	msgID := fmt.Sprintf("dune-admin-cmd-%d", time.Now().UnixMilli())

	erlang := fmt.Sprintf(
		`Outer = base64:decode(<<"%s">>),`+
			`XName = rabbit_misc:r(<<"/">>, exchange, <<"heartbeats">>),`+
			`X = rabbit_exchange:lookup_or_die(XName),`+
			`MsgId = <<"%s">>,`+
			`P = {list_to_atom("P_basic"), <<"Content">>, undefined, [], undefined,`+
			` undefined, undefined, undefined, undefined, MsgId, undefined,`+
			` undefined, <<"fls">>, <<"fls_backend">>, undefined},`+
			`Content = rabbit_basic:build_content(P, Outer),`+
			`{ok, Msg} = rabbit_basic:message(XName, <<"notifications">>, Content),`+
			`rabbit_queue_type:publish_at_most_once(X, Msg).`,
		outerB64, msgID)

	if globalControl == nil || globalExecutor == nil {
		return fmt.Errorf("control plane not connected")
	}

	_, err = globalControl.EvalOnGameBroker(context.Background(), globalExecutor, erlang)
	if err != nil {
		return fmt.Errorf("publish server command: %w", err)
	}
	return nil
}

// ── typed wrappers ────────────────────────────────────────────────────────────

func rmqAddItemToInventory(flsID, itemName string, qty int, durability float64) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "AddItemToInventory",
		"PlayerId":      flsID,
		"ItemName":      itemName,
		"Quantity":      qty,
		"Durability":    durability,
	})
}

func rmqKickPlayer(flsID string) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "KickPlayer",
		"PlayerId":      flsID,
	})
}

func rmqUpdateAllWaterFillables(flsID string, waterAmount int) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "UpdateAllWaterFillables",
		"PlayerId":      flsID,
		"WaterAmount":   waterAmount,
	})
}

func rmqAwardXP(flsID, category string, experience int) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "AwardXP",
		"PlayerId":      flsID,
		"Category":      category,
		"Experience":    experience,
	})
}

func rmqSkillsSetUnspentSkillPoints(flsID string, skillPoints int) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "SkillsSetUnspentSkillPoints",
		"PlayerId":      flsID,
		"SkillPoints":   skillPoints,
	})
}

type localizedText struct {
	Key   string `json:"Key"`
	Title string `json:"Title"`
	Body  string `json:"Body"`
}

func rmqServiceBroadcastGeneric(durationSec int, texts []localizedText) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "ServiceBroadcast",
		"BroadcastType": "Generic",
		"BroadcastPayload": map[string]any{
			"BroadcastDuration": durationSec,
			"LocalizedText":     texts,
		},
	})
}

func rmqServiceBroadcastShutdown(shutdownType string, timestamp int64, frequency, duration int, shouldCancel bool) error {
	payload := map[string]any{
		"ShutdownType":      shutdownType,
		"ShouldCancel":      shouldCancel,
		"ShutdownTimestamp": timestamp,
		"BroadcastFrequency": frequency,
		"ShutdownDuration":  duration,
		"DateTimestamp":     timestamp,
	}
	return publishServerCommand(map[string]any{
		"ServerCommand":    "ServiceBroadcast",
		"BroadcastType":    "ServerShutdown",
		"BroadcastPayload": payload,
	})
}

func rmqCheatScript(flsID, scriptName string) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "CheatScript",
		"PlayerId":      flsID,
		"ScriptName":    scriptName,
	})
}

func rmqCleanPlayerInventory(flsID string) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "CleanPlayerInventory",
		"PlayerId":      flsID,
	})
}

func rmqResetProgression(flsID string) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "ResetProgression",
		"PlayerId":      flsID,
	})
}

func rmqTeleportTo(flsID string, x, y, z float64) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "TeleportTo",
		"PlayerId":      flsID,
		"X":             x,
		"Y":             y,
		"Z":             z,
	})
}

// rmqTeleportToExact uses the engine's exact-location teleport path (no snap
// to nearest safe ground). Used for teleport-to-player, where the admin wants
// the source player to land precisely on top of the target rather than
// somewhere "safe" nearby. Per Adain's protocol docs.
func rmqTeleportToExact(flsID string, x, y, z float64) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "TeleportToExact",
		"PlayerId":      flsID,
		"X":             x,
		"Y":             y,
		"Z":             z,
	})
}

func rmqSkillsSetModuleLevel(flsID, module string, level int) error {
	return publishServerCommand(map[string]any{
		"ServerCommand": "SkillsSetModuleLevel",
		"PlayerId":      flsID,
		"Module":        module,
		"Level":         level,
	})
}

func rmqSpawnVehicleAt(flsID, className string, x, y, z, rotation float64, templateName string, persistent bool, faction string) error {
	fields := map[string]any{
		"ServerCommand": "SpawnVehicleAt",
		"PlayerId":      flsID,
		"ClassName":     className,
		"X":             x,
		"Y":             y,
		"Z":             z,
	}
	if rotation != 0 {
		fields["Rotation"] = rotation
	}
	if templateName != "" {
		fields["TemplateName"] = templateName
	}
	persistVal := 0.0
	if persistent {
		persistVal = 1.0
	}
	fields["Persistent"] = persistVal
	if faction != "" {
		fields["Faction"] = faction
	}
	return publishServerCommand(fields)
}


// ── player ID resolution ──────────────────────────────────────────────────────

// flsIDFromActorID resolves the accounts."user" hex Funcom UUID for an actor
// (player pawn). This is the PlayerId format expected by RMQ server commands.
func flsIDFromActorID(ctx context.Context, actorID int64) (string, error) {
	if globalDB == nil {
		return "", fmt.Errorf("not connected")
	}
	var flsID string
	err := globalDB.QueryRow(ctx, `
		SELECT ac."user"
		FROM dune.accounts ac
		JOIN dune.actors a ON a.owner_account_id = ac.id
		WHERE a.id = $1`, actorID).Scan(&flsID)
	if err != nil {
		return "", fmt.Errorf("resolve fls id for actor %d: %w", actorID, err)
	}
	return flsID, nil
}


// containerOwnerInfo holds the resolved owner details for a storage container.
type containerOwnerInfo struct {
	FlsID     string // accounts."user" hex Funcom UUID — used as PlayerId in RMQ commands
	AccountID int64  // accounts.id — used to check online status
}

// ownerFromContainerID resolves the FLS hex ID and account ID for the player
// who owns a storage container actor. Chain: placeables.owner_entity_id →
// actor_fgl_entities → actors.owner_account_id → accounts.
func ownerFromContainerID(ctx context.Context, containerID int64) (containerOwnerInfo, error) {
	if globalDB == nil {
		return containerOwnerInfo{}, fmt.Errorf("not connected")
	}
	var info containerOwnerInfo
	err := globalDB.QueryRow(ctx, `
		SELECT ac."user", ac.id
		FROM dune.placeables p
		JOIN dune.actor_fgl_entities afe  ON afe.entity_id = p.owner_entity_id
		JOIN dune.permission_actor_rank par ON par.permission_actor_id = afe.actor_id
		JOIN dune.actors player_a           ON player_a.id = par.player_id
		JOIN dune.accounts ac               ON ac.id = player_a.owner_account_id
		WHERE p.id = $1
		LIMIT 1`, containerID).Scan(&info.FlsID, &info.AccountID)
	if err != nil {
		return containerOwnerInfo{}, fmt.Errorf("resolve container owner %d: %w", containerID, err)
	}
	return info, nil
}

// isAccountOnline returns true if the account currently has a non-Offline
// player_state row. Uses account_id directly, suitable for storage container
// owner checks where the pawn actor ID is not readily available.
func isAccountOnline(ctx context.Context, accountID int64) bool {
	if globalDB == nil {
		return false
	}
	var status string
	err := globalDB.QueryRow(ctx, `
		SELECT COALESCE(online_status::text, 'Offline')
		FROM dune.player_state
		WHERE account_id = $1
		LIMIT 1`, accountID).Scan(&status)
	if err != nil {
		return false
	}
	return status != "Offline"
}

// playerIDDebug returns all relevant player ID forms for a given actor ID.
// Used by the debug endpoint to verify which ID format the game server expects.
func playerIDDebug(ctx context.Context, actorID int64) (map[string]string, error) {
	if globalDB == nil {
		return nil, fmt.Errorf("not connected")
	}
	var displayName, hexID string
	err := globalDB.QueryRow(ctx, `
		SELECT convert_from(e.encrypted_funcom_id, 'UTF8'), COALESCE(ac."user", '')
		FROM dune.encrypted_accounts e
		JOIN dune.actors a ON a.owner_account_id = e.id
		LEFT JOIN dune.accounts ac ON ac.id = e.id
		WHERE a.id = $1`, actorID).Scan(&displayName, &hexID)
	if err != nil {
		return nil, fmt.Errorf("lookup actor %d: %w", actorID, err)
	}
	return map[string]string{
		"display_name": displayName,
		"hex_id":       hexID,
	}, nil
}

// isHexIDOnline returns true if the player identified by their hex Funcom UUID
// (accounts."user") currently has a non-Offline online_status.
func isHexIDOnline(ctx context.Context, hexID string) bool {
	if globalDB == nil {
		return false
	}
	var status string
	err := globalDB.QueryRow(ctx, `
		SELECT COALESCE(ps.online_status::text, 'Offline')
		FROM dune.accounts ac
		JOIN dune.player_state ps ON ps.account_id = ac.id
		WHERE ac."user" = $1
		LIMIT 1`, hexID).Scan(&status)
	if err != nil {
		return false
	}
	return status != "Offline"
}

// displayNameFromHexID resolves the encrypted_funcom_id display name
// (e.g. "Icehunter#55381") from the hex Funcom UUID in accounts."user".
// Used by DB paths that identify players by display name.
func displayNameFromHexID(ctx context.Context, hexID string) (string, error) {
	if globalDB == nil {
		return "", fmt.Errorf("not connected")
	}
	var name string
	err := globalDB.QueryRow(ctx, `
		SELECT convert_from(e.encrypted_funcom_id, 'UTF8')
		FROM dune.accounts ac
		JOIN dune.encrypted_accounts e ON e.id = ac.id
		WHERE ac."user" = $1`, hexID).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("resolve display name for %s: %w", hexID, err)
	}
	return name, nil
}
