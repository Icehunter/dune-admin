package main

import (
	"encoding/json"
	"testing"
	"time"
)

// TestBuildWhisperBody_GoldenShape pins the exact whisper wire body against the
// live-confirmed shape in dune-rmq-protocol/chat-and-courier.md (whisper publish,
// 2026-05-28). This golden assertion is the regression guard for the four bugs in
// the previous best-guess implementation:
//
//  1. fields carry the m_ prefix (m_ChannelType, m_FuncomIdFrom, ...)
//  2. m_Message uses {m_UnlocalizedMessage, m_LocalizedMessage{...}} (NOT {Body})
//  3. m_SubChannelId (recipient funcom id) is present
//  4. enum/channel strings are fully qualified for the whisper channel
//
// user_id / routing key are AMQP-envelope concerns, asserted at the publish layer.
func TestBuildWhisperBody_GoldenShape(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 31, 19, 49, 58, 0, time.UTC)
	body, err := buildWhisperBody(
		"11111111-1111-1111-1111-111111111111", // msg id (caller-generated GUID)
		"Server#0001",                          // sender (GM) funcom id
		"Tester#1234",                          // recipient funcom id
		"Tester",                               // recipient character name
		"Hello from the server",                // visible whisper text
		ts,
	)
	if err != nil {
		t.Fatalf("buildWhisperBody: %v", err)
	}

	// Outer courier envelope for the whisper channel: capitalized "Content" key,
	// fully-qualified "Type". (Map chat uses a different outer shape — kept apart.)
	var outer struct {
		Content string `json:"Content"`
		Type    string `json:"Type"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("unmarshal outer envelope: %v", err)
	}
	if outer.Type != "ECourierMessageType::TextChat" {
		t.Fatalf("outer Type = %q, want ECourierMessageType::TextChat", outer.Type)
	}

	const wantInner = `{"m_Id":"11111111-1111-1111-1111-111111111111",` +
		`"m_ChannelType":"ETextChatChannelType::Whispers",` +
		`"m_SubChannelId":"Tester#1234",` +
		`"m_bUseSpoofedUserName":false,` +
		`"m_SpoofedUserNameFrom":{"m_Id":"","m_DisplayName":""},` +
		`"m_FuncomIdFrom":"Server#0001",` +
		`"m_UserNameTo":"Tester",` +
		`"m_Message":{"m_UnlocalizedMessage":"Hello from the server",` +
		`"m_LocalizedMessage":{"m_TableId":"","m_Key":"","m_FormatArgs":[]}},` +
		`"m_TimeStamp":"2026-05-31T19:49:58Z",` +
		`"m_OriginLocation":{"X":0,"Y":0,"Z":0},` +
		`"m_HasSeenMessage":false}`

	if outer.Content != wantInner {
		t.Fatalf("inner FChatMessageData mismatch:\n got: %s\nwant: %s", outer.Content, wantInner)
	}
}

// TestBuildWhisperBody_EscapesMessage guards the stringified-inner escaping: the
// inner FChatMessageData is carried as a JSON string inside the outer envelope, so
// quotes/newlines in operator text must survive a round trip without corrupting
// the body. A naive string concat would break here.
func TestBuildWhisperBody_EscapesMessage(t *testing.T) {
	t.Parallel()

	msg := "line1\nsay \"hi\"\tend"
	body, err := buildWhisperBody("id", "Server#0001", "Tester#1234", "Tester", msg, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("buildWhisperBody: %v", err)
	}

	var outer struct {
		Content string `json:"Content"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("unmarshal outer: %v", err)
	}
	var inner struct {
		Message struct {
			Unlocalized string `json:"m_UnlocalizedMessage"`
		} `json:"m_Message"`
	}
	if err := json.Unmarshal([]byte(outer.Content), &inner); err != nil {
		t.Fatalf("unmarshal inner content string: %v", err)
	}
	if inner.Message.Unlocalized != msg {
		t.Fatalf("message did not round-trip:\n got: %q\nwant: %q", inner.Message.Unlocalized, msg)
	}
}
