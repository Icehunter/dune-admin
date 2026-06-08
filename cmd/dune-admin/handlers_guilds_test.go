package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuildMemberDisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		charName string
		actorID  int64
		want     string
	}{
		{"resolved name passes through", "Paul Atreides", 123, "Paul Atreides"},
		{"empty name falls back to actor id", "", 456, "Actor 456"},
		{"whitespace-only name falls back", "   ", 789, "Actor 789"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := guildMemberDisplayName(tt.charName, tt.actorID); got != tt.want {
				t.Fatalf("guildMemberDisplayName(%q, %d) = %q, want %q", tt.charName, tt.actorID, got, tt.want)
			}
		})
	}
}

func TestHandleListGuilds_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/guilds", nil)
	rr := httptest.NewRecorder()
	handleListGuilds(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleGetGuild_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/guilds/42", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	handleGetGuild(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

// Mirrors the project convention (see handlers_stats_test.go): the globalDB
// nil-guard is checked before the id parse, so a bad id with no DB returns 503,
// not 400.
func TestHandleGetGuild_InvalidID_DBNilFirst(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/guilds/not-a-number", nil)
	req.SetPathValue("id", "not-a-number")
	rr := httptest.NewRecorder()
	handleGetGuild(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 (db nil checked before id parse), got %d", rr.Code)
	}
}

func TestGuildRoleSetProc(t *testing.T) {
	t.Parallel()
	// Setting a member to admin (100) must route through promote_guild_member
	// (which transfers the single admin slot); any lower role uses
	// demote_guild_member (which guards against demoting the current admin).
	cases := map[int16]string{
		guildRoleAdmin:  "promote_guild_member",
		guildRoleMember: "demote_guild_member",
		75:              "demote_guild_member",
	}
	for role, want := range cases {
		if got := guildRoleSetProc(role); got != want {
			t.Errorf("guildRoleSetProc(%d) = %q, want %q", role, got, want)
		}
	}
}

func TestHandleUpdateGuild_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/guilds/1", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	handleUpdateGuild(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleSetGuildMemberRole_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/guilds/1/members/2/role", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("pid", "2")
	rr := httptest.NewRecorder()
	handleSetGuildMemberRole(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}
