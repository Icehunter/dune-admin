package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleGetPlayers_NilDB verifies the handler returns 503 when globalDB is nil.
func TestHandleGetPlayers_NilDB(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)
	handleGetPlayers(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

// TestPlayerInfoDiscordField verifies that playerInfo serialises discord_user_id.
// This guards against accidentally removing the field or its JSON tag.
func TestPlayerInfoDiscordField(t *testing.T) {
	p := playerInfo{
		ID:            1,
		Name:          "Narisa",
		DiscordUserID: "123456789",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["discord_user_id"] != "123456789" {
		t.Errorf("discord_user_id not found in JSON: %s", b)
	}
}

// TestPlayerInfoDiscordFieldEmpty verifies that an unlinked player emits an
// empty string for discord_user_id (so the frontend can treat "" as unlinked).
func TestPlayerInfoDiscordFieldEmpty(t *testing.T) {
	p := playerInfo{ID: 2, Name: "Leto"}
	b, _ := json.Marshal(p)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["discord_user_id"]; !ok {
		t.Errorf("discord_user_id key missing from JSON: %s", b)
	}
	if m["discord_user_id"] != "" {
		t.Errorf("expected empty string for unlinked player, got %v", m["discord_user_id"])
	}
}
