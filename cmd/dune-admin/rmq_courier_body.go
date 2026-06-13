package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// newCourierMessageID returns a fresh GUID string for a chat message m_Id. The
// game treats m_Id as a per-message identifier; uniqueness is what matters.
func newCourierMessageID() string {
	return uuid.NewString()
}

// Courier (chat) message body builders. The wire shapes here are pinned against
// the live-confirmed publishes documented in the dune-rmq-protocol research
// (chat-and-courier.md). Each chat channel has its OWN serialized shape — field
// casing, enum qualification, and even the outer envelope key differ between the
// whisper and map channels — so the builders are deliberately NOT shared. Written
// from scratch against the findings; never copy the research repo's content.

// ── shared inner sub-structs ────────────────────────────────────────────────

// localizedMessageData is FLocalizedMessageData. Empty for direct operator text.
type localizedMessageData struct {
	TableID    string   `json:"m_TableId"`
	Key        string   `json:"m_Key"`
	FormatArgs []string `json:"m_FormatArgs"`
}

// localizableMessage is FLocalizableMessageData. m_UnlocalizedMessage carries the
// plain operator text — live-confirmed as the field the client actually renders.
type localizableMessage struct {
	Unlocalized string               `json:"m_UnlocalizedMessage"`
	Localized   localizedMessageData `json:"m_LocalizedMessage"`
}

// vec3 is the FVector origin location. Operator chat uses the zero vector.
type vec3 struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
	Z float64 `json:"Z"`
}

// newLocalizableMessage builds the message payload with a non-nil empty
// FormatArgs so it serializes as [] (a nil slice would emit null).
func newLocalizableMessage(text string) localizableMessage {
	return localizableMessage{
		Unlocalized: text,
		Localized:   localizedMessageData{FormatArgs: []string{}},
	}
}

// ── whisper (chat.whispers) ─────────────────────────────────────────────────

// whisperSpoofedAuthor is FMessageAuthor for the whisper channel: {m_Id, m_DisplayName}.
// (Map chat uses a different spoofed-author shape.)
type whisperSpoofedAuthor struct {
	ID          string `json:"m_Id"`
	DisplayName string `json:"m_DisplayName"`
}

// whisperChatData is FChatMessageData for the Whispers channel. Field order
// mirrors the live-confirmed body for readability; the game parses by name.
type whisperChatData struct {
	ID              string               `json:"m_Id"`
	ChannelType     string               `json:"m_ChannelType"`
	SubChannelID    string               `json:"m_SubChannelId"`
	UseSpoofedName  bool                 `json:"m_bUseSpoofedUserName"`
	SpoofedNameFrom whisperSpoofedAuthor `json:"m_SpoofedUserNameFrom"`
	FuncomIDFrom    string               `json:"m_FuncomIdFrom"`
	UserNameTo      string               `json:"m_UserNameTo"`
	Message         localizableMessage   `json:"m_Message"`
	TimeStamp       string               `json:"m_TimeStamp"`
	OriginLocation  vec3                 `json:"m_OriginLocation"`
	HasSeenMessage  bool                 `json:"m_HasSeenMessage"`
}

// courierEnvelope is FCourierMessageContent — the outer body the game parses for
// the whisper channel: a stringified inner payload plus a Type discriminator.
type courierEnvelope struct {
	Content string `json:"Content"`
	Type    string `json:"Type"`
}

// ── map chat (chat.map) ─────────────────────────────────────────────────────

// mapChatSpoofedAuthor is the spoofed-author shape for map chat. It differs from
// the whisper channel's FMessageAuthor: map chat uses {m_TableId, m_Key,
// m_UnlocalizedName} (live-captured 2026-05-31) vs whisper's {m_Id, m_DisplayName}.
type mapChatSpoofedAuthor struct {
	TableID         string `json:"m_TableId"`
	Key             string `json:"m_Key"`
	UnlocalizedName string `json:"m_UnlocalizedName"`
}

// mapChatData is FChatMessageData for the Map channel. Field order matches the
// live-captured body (dune-rmq-protocol/chat-and-courier.md, 2026-05-31).
// Notable differences from whisperChatData:
//   - No m_SubChannelId (whisper uses it for per-recipient routing).
//   - m_UserNameTo is always empty (map chat is channel-broadcast, not direct).
//   - m_ChannelType uses short form "Map" (not "ETextChatChannelType::Map").
//   - Timestamp field is m_Timestamp (lowercase t) in "2006.01.02-15.04.05" format.
type mapChatData struct {
	ID             string               `json:"m_Id"`
	ChannelType    string               `json:"m_ChannelType"`
	UseSpoofedName bool                 `json:"m_bUseSpoofedUserName"`
	SpoofedFrom    mapChatSpoofedAuthor `json:"m_SpoofedUserNameFrom"`
	FuncomIDFrom   string               `json:"m_FuncomIdFrom"`
	UserNameTo     string               `json:"m_UserNameTo"`
	Message        localizableMessage   `json:"m_Message"`
	Timestamp      string               `json:"m_Timestamp"`
	OriginLocation vec3                 `json:"m_OriginLocation"`
	HasSeenMessage bool                 `json:"m_HasSeenMessage"`
}

// mapChatEnvelope is the outer FCourierMessageContent for the map chat channel.
// The envelope key is lowercase "content" (differs from whisper's "Content"), and
// Type uses the short form "TextChat" (not "ECourierMessageType::TextChat").
// Both differences are live-confirmed in dune-rmq-protocol/chat-and-courier.md.
type mapChatEnvelope struct {
	Content string `json:"content"`
	Type    string `json:"Type"`
}

// buildMapChatBody serializes a map-channel chat message to the live-confirmed
// wire body (dune-rmq-protocol/chat-and-courier.md, 2026-05-31). senderFuncomID
// is the GM/bridge persona's chat id (m_FuncomIdFrom); the AMQP user_id and
// routing key are applied at the publish layer by rmqSendMapChat.
func buildMapChatBody(msgID, senderFuncomID, message string, ts time.Time) ([]byte, error) {
	inner := mapChatData{
		ID:           msgID,
		ChannelType:  "Map",
		FuncomIDFrom: senderFuncomID,
		Message:      newLocalizableMessage(message),
		Timestamp:    ts.UTC().Format("2006.01.02-15.04.05"),
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("marshal map chat data: %w", err)
	}
	body, err := json.Marshal(mapChatEnvelope{
		Content: string(innerJSON),
		Type:    "TextChat",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal map chat envelope: %w", err)
	}
	return body, nil
}

// buildWhisperBody serializes a private (Whispers channel) chat message to the
// exact live-confirmed wire body. The inner FChatMessageData is marshalled, then
// embedded as a STRING inside the FCourierMessageContent envelope — the game
// expects Content to be stringified JSON, not a nested object.
//
// senderFuncomID is the GM/Server persona's funcom id (m_FuncomIdFrom);
// recipientFuncomID is the target's funcom id (m_SubChannelId + AMQP routing key);
// recipientCharName is the target's character name (m_UserNameTo). The AMQP
// user_id (sender hex FLS id) and routing key are applied at the publish layer.
func buildWhisperBody(msgID, senderFuncomID, recipientFuncomID, recipientCharName, message string, ts time.Time) ([]byte, error) {
	inner := whisperChatData{
		ID:             msgID,
		ChannelType:    "ETextChatChannelType::Whispers",
		SubChannelID:   recipientFuncomID,
		UseSpoofedName: false,
		FuncomIDFrom:   senderFuncomID,
		UserNameTo:     recipientCharName,
		Message:        newLocalizableMessage(message),
		TimeStamp:      ts.UTC().Format(time.RFC3339),
		HasSeenMessage: false,
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("marshal whisper chat data: %w", err)
	}
	body, err := json.Marshal(courierEnvelope{
		Content: string(innerJSON),
		Type:    "ECourierMessageType::TextChat",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal whisper courier envelope: %w", err)
	}
	return body, nil
}
