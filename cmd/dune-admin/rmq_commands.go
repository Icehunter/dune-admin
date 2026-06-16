package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// buildCourierPublishExpr builds the rabbitmqctl eval Erlang expression that
// publishes a chat/courier message. Publishing via eval (rather than an AMQP
// client) is what lets us set the P_basic user_id to an arbitrary sender FLS id —
// the broker would otherwise force user_id to match the authenticated connection,
// and a synthetic "fls" sender is silently dropped by the game.
//
// P_basic record order: content_type, content_encoding, headers, delivery_mode,
// priority, correlation_id, reply_to, expiration, message_id, timestamp, type,
// user_id, app_id, cluster_id. We set type to the notification type NAME (e.g.
// "text_chat"), user_id to the sender's hex FLS id, and app_id to fls_backend.
func buildCourierPublishExpr(exchange, routingKey, bodyB64, msgID, amqpType, userID string) string {
	return fmt.Sprintf(
		`Body = base64:decode(<<"%s">>),`+
			`XName = rabbit_misc:r(<<"/">>, exchange, <<"%s">>),`+
			`X = rabbit_exchange:lookup_or_die(XName),`+
			`MsgId = <<"%s">>,`+
			`P = {list_to_atom("P_basic"), <<"Content">>, undefined, [], undefined,`+
			` undefined, undefined, undefined, undefined, MsgId, undefined,`+
			` <<"%s">>, <<"%s">>, <<"fls_backend">>, undefined},`+
			`Content = rabbit_basic:build_content(P, Body),`+
			`{ok, Msg} = rabbit_basic:message(XName, <<"%s">>, Content),`+
			`rabbit_queue_type:publish_at_most_once(X, Msg).`,
		bodyB64, exchange, msgID, amqpType, userID, routingKey)
}

// publishCourierMessageAs publishes a chat/courier message to the game broker as
// a specific sender identity. exchange + routingKey select the chat channel
// (e.g. chat.whispers + recipient funcom id); amqpType is the notification type
// name ("text_chat"); userID is the sender's hex FLS id (the GM/Server persona).
// body is the already-serialized FCourierMessageContent envelope.
func publishCourierMessageAs(exchange, routingKey string, body []byte, amqpType, userID string) error {
	if globalControl == nil || globalExecutor == nil {
		return fmt.Errorf("control plane not connected")
	}
	bodyB64 := base64.StdEncoding.EncodeToString(body)
	msgID := fmt.Sprintf("dune-admin-chat-%d", time.Now().UnixMilli())
	expr := buildCourierPublishExpr(exchange, routingKey, bodyB64, msgID, amqpType, userID)
	out, err := globalControl.EvalOnGameBroker(context.Background(), globalExecutor, expr)
	if err != nil {
		return fmt.Errorf("publish courier message: %w (output: %s)", err, out)
	}
	return nil
}

// rmqSendWhisper sends a private chat message ("whisper") to one player, shown in
// the target's Whispers tab. The whisper is attributed to the seeded GM/Server
// persona: senderFuncomID is that identity's funcom id (m_FuncomIdFrom) and
// senderHexID is its hex FLS id (the AMQP user_id the game resolves the sender
// name from). recipientFuncomID is the target's funcom id (m_SubChannelId and the
// whisper routing key); recipientName is the target's character name
// (m_UserNameTo). The exact wire shape is pinned by buildWhisperBody.
func rmqSendWhisper(senderFuncomID, senderHexID, recipientFuncomID, recipientName, message string) error {
	body, err := buildWhisperBody(newCourierMessageID(), senderFuncomID, recipientFuncomID, recipientName, message, time.Now())
	if err != nil {
		return err
	}
	return publishCourierMessageAs("chat.whispers", recipientFuncomID, body, "text_chat", senderHexID)
}

// rmqSendMapChat publishes a message to the map chat channel for mapName/dimension,
// shown in the regional chat tab for all players subscribed to that channel.
// Exchange: chat.map, routing key: {mapName}.{dimension} (e.g. "HaggaBasin.0").
// senderFuncomID is the GM/bridge persona's chat id (m_FuncomIdFrom); senderHexID
// is the AMQP user_id — must be a real DB-backed FLS id (live-confirmed requirement).
func rmqSendMapChat(mapName string, dimension int, senderFuncomID, senderHexID, message string) error {
	body, err := buildMapChatBody(newCourierMessageID(), senderFuncomID, message, time.Now())
	if err != nil {
		return err
	}
	routingKey := fmt.Sprintf("%s.%d", mapName, dimension)
	return publishCourierMessageAs("chat.map", routingKey, body, "text_chat", senderHexID)
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

// flsIDFromActorIDPool resolves the accounts."user" hex Funcom UUID for an actor
// (player pawn) using the provided pool. This is the PlayerId format expected by
// RMQ server commands.
func flsIDFromActorIDPool(ctx context.Context, pool *pgxpool.Pool, actorID int64) (string, error) {
	if pool == nil {
		return "", fmt.Errorf("not connected")
	}
	var flsID string
	err := pool.QueryRow(ctx, `
		SELECT ac."user"
		FROM dune.accounts ac
		JOIN dune.actors a ON a.owner_account_id = ac.id
		WHERE a.id = $1`, actorID).Scan(&flsID)
	if err != nil {
		return "", fmt.Errorf("resolve fls id for actor %d: %w", actorID, err)
	}
	return flsID, nil
}

// flsIDFromActorID resolves via the global pool (legacy; use flsIDFromActorIDPool for multi-server).
func flsIDFromActorID(ctx context.Context, actorID int64) (string, error) {
	return flsIDFromActorIDPool(ctx, globalDB, actorID)
}

// playerIDDebugPool returns all relevant player ID forms for a given actor ID
// using the provided pool. Used by the debug endpoint.
func playerIDDebugPool(ctx context.Context, pool *pgxpool.Pool, actorID int64) (map[string]string, error) {
	if pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	var displayName, hexID string
	err := pool.QueryRow(ctx, `
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

// playerIDDebug resolves via the global pool (legacy; use playerIDDebugPool for multi-server).
func playerIDDebug(ctx context.Context, actorID int64) (map[string]string, error) {
	return playerIDDebugPool(ctx, globalDB, actorID)
}

// isHexIDOnlinePool returns true if the player identified by their hex Funcom UUID
// (accounts."user") currently has a non-Offline online_status, using the given pool.
func isHexIDOnlinePool(ctx context.Context, pool *pgxpool.Pool, hexID string) bool {
	if pool == nil {
		return false
	}
	var status string
	err := pool.QueryRow(ctx, `
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

// isHexIDOnline resolves via the global pool (legacy; use isHexIDOnlinePool for multi-server).
func isHexIDOnline(ctx context.Context, hexID string) bool {
	return isHexIDOnlinePool(ctx, globalDB, hexID)
}

// displayNameFromHexIDPool resolves the encrypted_funcom_id display name
// (e.g. "Icehunter#55381") from the hex Funcom UUID in accounts."user",
// using the given pool.
func displayNameFromHexIDPool(ctx context.Context, pool *pgxpool.Pool, hexID string) (string, error) {
	if pool == nil {
		return "", fmt.Errorf("not connected")
	}
	var name string
	err := pool.QueryRow(ctx, `
		SELECT convert_from(e.encrypted_funcom_id, 'UTF8')
		FROM dune.accounts ac
		JOIN dune.encrypted_accounts e ON e.id = ac.id
		WHERE ac."user" = $1`, hexID).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("resolve display name for %s: %w", hexID, err)
	}
	return name, nil
}

// displayNameFromHexID resolves via the global pool (legacy; use displayNameFromHexIDPool for multi-server).
func displayNameFromHexID(ctx context.Context, hexID string) (string, error) {
	return displayNameFromHexIDPool(ctx, globalDB, hexID)
}
