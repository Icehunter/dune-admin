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

// ── courier (chat) publish ────────────────────────────────────────────────────

// publishCourierMessage publishes a chat / courier message to the game broker.
// Unlike publishServerCommand (which goes to exchange=heartbeats,
// routingKey=notifications via the ServerCommand AuthToken-protected envelope),
// courier messages are routed by exchange + routingKey directly — exchange is
// chat-channel-specific (chat.whispers, chat.faction.{id}, ...) and routing key
// varies by channel (target FLS for whispers, "" for faction broadcasts, etc.).
//
// EXPERIMENTAL — Adain's chat-and-courier.md documents the wire format from
// IDA/DWARF analysis but explicitly notes "live external publish recipe still
// not pinned." This is the first attempt at sending one of these as an external
// operator. If the game ignores the message or the broker rejects it, see
// chat-and-courier.md and iterate the basic_properties or body shape.
//
// body should be the JSON-serialized FCourierMessageContent — caller's
// responsibility. typeStr is the AMQP basic_properties `type` value, set to
// "12" for text_chat per the FNotificationsSystemMessage type-byte mapping.
func publishCourierMessage(exchange, routingKey string, body []byte, typeStr string) error {
	bodyB64 := base64.StdEncoding.EncodeToString(body)
	msgID := fmt.Sprintf("dune-admin-chat-%d", time.Now().UnixMilli())

	// AMQP basic_properties P_basic record order:
	//   content_type, content_encoding, headers, delivery_mode, priority,
	//   correlation_id, reply_to, expiration, message_id, timestamp, type,
	//   user_id, app_id, cluster_id
	// We set type to the courier message-type byte string ("12") and reuse
	// the fls / fls_backend user/app identity that the game expects.
	erlang := fmt.Sprintf(
		`Body = base64:decode(<<"%s">>),`+
			`XName = rabbit_misc:r(<<"/">>, exchange, <<"%s">>),`+
			`X = rabbit_exchange:lookup_or_die(XName),`+
			`MsgId = <<"%s">>,`+
			`P = {list_to_atom("P_basic"), <<"Content">>, undefined, [], undefined,`+
			` undefined, undefined, undefined, undefined, MsgId, undefined,`+
			` <<"%s">>, <<"fls">>, <<"fls_backend">>, undefined},`+
			`Content = rabbit_basic:build_content(P, Body),`+
			`{ok, Msg} = rabbit_basic:message(XName, <<"%s">>, Content),`+
			`rabbit_queue_type:publish_at_most_once(X, Msg).`,
		bodyB64, exchange, msgID, typeStr, routingKey)

	if globalControl == nil || globalExecutor == nil {
		return fmt.Errorf("control plane not connected")
	}
	out, err := globalControl.EvalOnGameBroker(context.Background(), globalExecutor, erlang)
	if err != nil {
		return fmt.Errorf("publish courier message: %w (output: %s)", err, out)
	}
	return nil
}

// rmqSendWhisper sends a private chat message ("whisper") to one player.
// The target sees it in their whispers chat tab; only they receive it.
//
// senderName is the display name shown on the whisper. impersonatedFlsID is
// optional — if non-empty, the message appears to come from that FLS player
// (admin-only feature, useful for "GM" personas). Leave empty for an unsigned
// admin whisper.
func rmqSendWhisper(targetFlsID, targetName, senderName, message, impersonatedFlsID string) error {
	// FChatMessageData per chat-and-courier.md. JSON field names match the C++
	// member names with the m_ prefix stripped (broadcast struct uses the same
	// convention).
	chatMsg := map[string]any{
		"Id":                  fmt.Sprintf("%d", time.Now().UnixNano()),
		"ChannelType":         "ETextChatChannelType::Whispers",
		"FuncomIdFrom":        impersonatedFlsID,
		"UserNameTo":          targetName,
		"Message":             map[string]any{"Body": message},
		"TimeStamp":           time.Now().UTC().Format(time.RFC3339),
		"bUseSpoofedUserName": senderName != "",
		"SpoofedUserNameFrom": map[string]any{"AuthorName": senderName},
	}
	chatJSON, err := json.Marshal(chatMsg)
	if err != nil {
		return fmt.Errorf("marshal chat message: %w", err)
	}

	// FCourierMessageContent: outer wrapper with stringified inner Content +
	// Type discriminator.
	envelope := map[string]any{
		"Content": string(chatJSON),
		"Type":    "ECourierMessageType::TextChat",
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal courier envelope: %w", err)
	}

	// Whisper routing: exchange=chat.whispers, routing key=target's FLS id.
	return publishCourierMessage("chat.whispers", targetFlsID, envelopeJSON, "12")
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
		"ShutdownType":       shutdownType,
		"ShouldCancel":       shouldCancel,
		"ShutdownTimestamp":  timestamp,
		"BroadcastFrequency": frequency,
		"ShutdownDuration":   duration,
		"DateTimestamp":      timestamp,
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
