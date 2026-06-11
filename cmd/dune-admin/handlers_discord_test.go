package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeTestDiscordCfg() discordConfig {
	return discordConfig{
		GuildID:           "guild123",
		RolesViewer:       []string{"viewer-role"},
		RolesEconomy:      []string{"econ-role"},
		RolesAdmin:        []string{"admin-role"},
		AnnounceChannelID: "chan456",
	}
}

// ── TestAuthorizeDiscord ──────────────────────────────────────────────────────

func TestAuthorizeDiscord(t *testing.T) {
	cfg := makeTestDiscordCfg()

	tests := []struct {
		name     string
		guildID  string
		member   discordMember
		required discordTier
		want     bool
	}{
		{
			name:     "wrong guild rejected",
			guildID:  "other-guild",
			member:   discordMember{UserID: "u1", Roles: []string{"admin-role"}},
			required: tierViewer,
			want:     false,
		},
		{
			name:     "guild owner always allowed for admin command",
			guildID:  "guild123",
			member:   discordMember{UserID: "owner", Roles: nil, IsAdministrator: true},
			required: tierAdmin,
			want:     true,
		},
		{
			name:     "guild owner allowed for viewer command",
			guildID:  "guild123",
			member:   discordMember{UserID: "owner", IsAdministrator: true},
			required: tierViewer,
			want:     true,
		},
		{
			name:     "admin role satisfies admin required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"admin-role"}},
			required: tierAdmin,
			want:     true,
		},
		{
			name:     "admin role satisfies economy required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"admin-role"}},
			required: tierEconomy,
			want:     true,
		},
		{
			name:     "admin role satisfies viewer required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"admin-role"}},
			required: tierViewer,
			want:     true,
		},
		{
			name:     "economy role satisfies economy required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"econ-role"}},
			required: tierEconomy,
			want:     true,
		},
		{
			name:     "economy role satisfies viewer required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"econ-role"}},
			required: tierViewer,
			want:     true,
		},
		{
			name:     "economy role rejected for admin required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"econ-role"}},
			required: tierAdmin,
			want:     false,
		},
		{
			name:     "viewer role satisfies viewer required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"viewer-role"}},
			required: tierViewer,
			want:     true,
		},
		{
			name:     "viewer role rejected for economy required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"viewer-role"}},
			required: tierEconomy,
			want:     false,
		},
		{
			name:     "viewer role rejected for admin required",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"viewer-role"}},
			required: tierAdmin,
			want:     false,
		},
		{
			name:     "no mapped roles rejected",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: []string{"some-random-role"}},
			required: tierViewer,
			want:     false,
		},
		{
			name:     "no roles at all rejected",
			guildID:  "guild123",
			member:   discordMember{UserID: "u1", Roles: nil},
			required: tierViewer,
			want:     false,
		},
		{
			name:    "mixed roles: highest tier wins",
			guildID: "guild123",
			member: discordMember{
				UserID: "u1",
				Roles:  []string{"viewer-role", "econ-role"},
			},
			required: tierEconomy,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authorizeDiscord(tt.guildID, tt.member, tt.required, cfg)
			if got != tt.want {
				t.Errorf("authorizeDiscord() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── TestCommandTier ───────────────────────────────────────────────────────────

func TestCommandTier(t *testing.T) {
	tests := []struct {
		cmd  string
		want discordTier
	}{
		{"status", tierViewer},
		{"lookup", tierViewer},
		{"register", tierViewer},
		{"unregister", tierViewer},
		{"mystats", tierViewer},
		{"mybalance", tierViewer},
		{"myinventory", tierViewer},
		{"give-currency", tierEconomy},
		{"unknown-cmd", tierAdmin}, // fail-safe: unknown commands require highest tier
		{"", tierAdmin},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := commandTier(tt.cmd)
			if got != tt.want {
				t.Errorf("commandTier(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// ── TestDispatchDiscordCommand ────────────────────────────────────────────────

func TestDispatchDiscordCommand(t *testing.T) {
	cfg := makeTestDiscordCfg()
	adminMember := discordMember{UserID: "admin1", Roles: []string{"admin-role"}}
	econMember := discordMember{UserID: "econ1", Roles: []string{"econ-role"}}
	viewerMember := discordMember{UserID: "view1", Roles: []string{"viewer-role"}}
	anonMember := discordMember{UserID: "anon1", Roles: nil}

	statusErr := errors.New("db down")
	lookupErr := errors.New("lookup failed")
	giveErr := errors.New("give failed")

	samplePlayers := []playerInfo{
		{ID: 10, AccountID: 20, Name: "Narisa", OnlineStatus: "Online"},
	}
	multiPlayers := []playerInfo{
		{ID: 10, AccountID: 20, Name: "Narisa"},
		{ID: 11, AccountID: 21, Name: "Narisa"},
	}

	makeDeps := func(
		statusResult string, statusErr error,
		lookupResult []playerInfo, lookupErr error,
		giveResult int64, giveErr error,
	) discordDeps {
		return discordDeps{
			status: func(_ context.Context) (string, error) {
				return statusResult, statusErr
			},
			lookupPlayer: func(_ context.Context, _ string) ([]playerInfo, error) {
				return lookupResult, lookupErr
			},
			giveCurrency: func(_ context.Context, _ int64, _ int64) (int64, error) {
				return giveResult, giveErr
			},
		}
	}

	tests := []struct {
		name            string
		interaction     discordInteraction
		deps            discordDeps
		wantEphemeral   bool
		wantContains    string // substring that must appear in reply
		wantNotContains string // substring that must NOT appear (optional)
	}{
		// ── authorization ────────────────────────────────────────────────────
		{
			name: "unauthorized member rejected with ephemeral",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  anonMember,
				Command: "status",
			},
			deps:          makeDeps("7 online", nil, nil, nil, 0, nil),
			wantEphemeral: true,
			wantContains:  "not authorized",
		},
		{
			name: "wrong guild rejected",
			interaction: discordInteraction{
				GuildID: "wrong-guild",
				Member:  adminMember,
				Command: "status",
			},
			deps:          makeDeps("7 online", nil, nil, nil, 0, nil),
			wantEphemeral: true,
			wantContains:  "not authorized",
		},
		{
			name: "viewer role rejected from give-currency (economy required)",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa", "amount": int64(100)},
			},
			deps:          makeDeps("", nil, samplePlayers, nil, 500, nil),
			wantEphemeral: true,
			wantContains:  "not authorized",
		},
		// ── unknown command ───────────────────────────────────────────────────
		{
			name: "unknown command returns ephemeral error",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  discordMember{UserID: "owner", IsAdministrator: true},
				Command: "nonexistent",
			},
			deps:          makeDeps("", nil, nil, nil, 0, nil),
			wantEphemeral: true,
			wantContains:  "unknown command",
		},
		// ── /status ───────────────────────────────────────────────────────────
		{
			name: "status happy path",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "status",
			},
			deps:         makeDeps("7 online / 42 total", nil, nil, nil, 0, nil),
			wantContains: "7 online",
		},
		{
			name: "status dep error returns error message",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "status",
			},
			deps:         makeDeps("", statusErr, nil, nil, 0, nil),
			wantContains: "error",
		},
		// ── /lookup ───────────────────────────────────────────────────────────
		{
			name: "lookup returns player info",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "lookup",
				Options: map[string]any{"name": "Narisa"},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 0, nil),
			wantContains: "Narisa",
		},
		{
			name: "lookup no results",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "lookup",
				Options: map[string]any{"name": "Ghost"},
			},
			deps:         makeDeps("", nil, nil, nil, 0, nil),
			wantContains: "no player found",
		},
		{
			name: "lookup multiple matches reports all",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "lookup",
				Options: map[string]any{"name": "Narisa"},
			},
			deps:         makeDeps("", nil, multiPlayers, nil, 0, nil),
			wantContains: "2 match",
		},
		{
			name: "lookup dep error returns error message",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "lookup",
				Options: map[string]any{"name": "Narisa"},
			},
			deps:         makeDeps("", nil, nil, lookupErr, 0, nil),
			wantContains: "error",
		},
		{
			name: "lookup missing name option",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  viewerMember,
				Command: "lookup",
				Options: map[string]any{},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 0, nil),
			wantContains: "name",
		},
		// ── /give-currency ────────────────────────────────────────────────────
		{
			name: "give-currency happy path reports new balance",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa", "amount": int64(250)},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 750, nil),
			wantContains: "750",
		},
		{
			name: "give-currency player not found",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Ghost", "amount": int64(100)},
			},
			deps:         makeDeps("", nil, nil, nil, 0, nil),
			wantContains: "no player found",
		},
		{
			name: "give-currency ambiguous name rejected",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa", "amount": int64(100)},
			},
			deps:         makeDeps("", nil, multiPlayers, nil, 0, nil),
			wantContains: "ambiguous",
		},
		{
			name: "give-currency lookup dep error",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa", "amount": int64(100)},
			},
			deps:         makeDeps("", nil, nil, lookupErr, 0, nil),
			wantContains: "error",
		},
		{
			name: "give-currency grant dep error",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa", "amount": int64(100)},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 0, giveErr),
			wantContains: "error",
		},
		{
			name: "give-currency missing name option",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"amount": int64(100)},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 0, nil),
			wantContains: "name",
		},
		{
			name: "give-currency missing amount option",
			interaction: discordInteraction{
				GuildID: "guild123",
				Member:  econMember,
				Command: "give-currency",
				Options: map[string]any{"name": "Narisa"},
			},
			deps:         makeDeps("", nil, samplePlayers, nil, 0, nil),
			wantContains: "amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got := dispatchDiscordCommand(ctx, tt.interaction, cfg, tt.deps)

			if tt.wantEphemeral && !got.Ephemeral {
				t.Errorf("expected ephemeral reply, got non-ephemeral: %q", got.Content)
			}

			if tt.wantContains != "" {
				// case-insensitive substring check
				lower := toLower(got.Content)
				if !contains(lower, toLower(tt.wantContains)) {
					t.Errorf("reply %q does not contain %q", got.Content, tt.wantContains)
				}
			}

			if tt.wantNotContains != "" {
				lower := toLower(got.Content)
				if contains(lower, toLower(tt.wantNotContains)) {
					t.Errorf("reply %q unexpectedly contains %q", got.Content, tt.wantNotContains)
				}
			}
		})
	}
}

// ── TestDispatchRegisterUnregister ───────────────────────────────────────────

func TestDispatchRegisterUnregister(t *testing.T) {
	cfg := makeTestDiscordCfg()
	viewer := discordMember{UserID: "u1", Roles: []string{"viewer-role"}}
	player := playerInfo{ID: 99, AccountID: 200, Name: "Narisa"}
	regErr := errors.New("db error")

	tests := []struct {
		name          string
		interaction   discordInteraction
		deps          discordDeps
		wantEphemeral bool
		wantContains  string
	}{
		// ── /register ────────────────────────────────────────────────────────
		{
			name: "register: happy path",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "register",
				Options: map[string]any{"name": "Narisa"}},
			deps: discordDeps{
				lookupPlayer: func(_ context.Context, _ string) ([]playerInfo, error) { return []playerInfo{player}, nil },
				registerLink: func(_ context.Context, _ string, _ int64, _, _ string) error { return nil },
			},
			wantEphemeral: true,
			wantContains:  "Narisa",
		},
		{
			name: "register: character not found",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "register",
				Options: map[string]any{"name": "Ghost"}},
			deps: discordDeps{
				lookupPlayer: func(_ context.Context, _ string) ([]playerInfo, error) { return nil, nil },
			},
			wantEphemeral: true,
			wantContains:  "No character",
		},
		{
			name: "register: ambiguous name",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "register",
				Options: map[string]any{"name": "Nar"}},
			deps: discordDeps{
				lookupPlayer: func(_ context.Context, _ string) ([]playerInfo, error) {
					return []playerInfo{player, {ID: 100, Name: "Narco"}}, nil
				},
			},
			wantEphemeral: true,
			wantContains:  "Multiple",
		},
		{
			name: "register: missing name option",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "register",
				Options: map[string]any{}},
			deps:          discordDeps{},
			wantEphemeral: true,
			wantContains:  "character name",
		},
		{
			name: "register: db error on store",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "register",
				Options: map[string]any{"name": "Narisa"}},
			deps: discordDeps{
				lookupPlayer: func(_ context.Context, _ string) ([]playerInfo, error) { return []playerInfo{player}, nil },
				registerLink: func(_ context.Context, _ string, _ int64, _, _ string) error { return regErr },
			},
			wantEphemeral: true,
			wantContains:  "failed",
		},
		// ── /unregister ──────────────────────────────────────────────────────
		{
			name:        "unregister: happy path",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "unregister"},
			deps: discordDeps{
				deleteLink: func(_ context.Context, _ string) (bool, error) { return true, nil },
			},
			wantEphemeral: true,
			wantContains:  "Unregistered",
		},
		{
			name:        "unregister: not registered",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "unregister"},
			deps: discordDeps{
				deleteLink: func(_ context.Context, _ string) (bool, error) { return false, nil },
			},
			wantEphemeral: true,
			wantContains:  "not registered",
		},
		{
			name:        "unregister: db error",
			interaction: discordInteraction{GuildID: "guild123", Member: viewer, Command: "unregister"},
			deps: discordDeps{
				deleteLink: func(_ context.Context, _ string) (bool, error) { return false, regErr },
			},
			wantEphemeral: true,
			wantContains:  "failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispatchDiscordCommand(context.Background(), tt.interaction, cfg, tt.deps)
			if got.Ephemeral != tt.wantEphemeral {
				t.Errorf("ephemeral = %v, want %v", got.Ephemeral, tt.wantEphemeral)
			}
			if !contains(toLower(got.Content), toLower(tt.wantContains)) {
				t.Errorf("content %q missing %q", got.Content, tt.wantContains)
			}
		})
	}
}

// ── TestDispatchSelfService ───────────────────────────────────────────────────

func TestDispatchSelfService(t *testing.T) {
	cfg := makeTestDiscordCfg()
	viewer := discordMember{UserID: "u1", Roles: []string{"viewer-role"}}
	player := playerInfo{ID: 99, AccountID: 200, Name: "Narisa", Map: "Hagga Basin"}
	currency := []currencyRow{{PlayerID: 99, CurrencyID: 0, Balance: 5000}}
	inv := []itemInfo{{TemplateID: "SpiceFiber", Name: "Spice Fiber", StackSize: 10}}

	notRegisteredDeps := discordDeps{
		getLink: func(_ context.Context, _ string) (int64, string, error) { return 0, "", nil },
	}
	registeredDeps := discordDeps{
		getLink:        func(_ context.Context, _ string) (int64, string, error) { return 200, "Narisa", nil },
		lookupPlayer:   func(_ context.Context, _ string) ([]playerInfo, error) { return []playerInfo{player}, nil },
		fetchCurrency:  func(_ context.Context, _ int64) ([]currencyRow, error) { return currency, nil },
		fetchInventory: func(_ context.Context, _ int64) ([]itemInfo, error) { return inv, nil },
	}

	tests := []struct {
		name         string
		cmd          string
		deps         discordDeps
		wantContains string
	}{
		{"mystats: not registered", "mystats", notRegisteredDeps, "register"},
		{"mystats: happy path", "mystats", registeredDeps, "Narisa"},
		{"mybalance: not registered", "mybalance", notRegisteredDeps, "register"},
		{"mybalance: happy path", "mybalance", registeredDeps, "5000"},
		{"myinventory: not registered", "myinventory", notRegisteredDeps, "register"},
		{"myinventory: happy path", "myinventory", registeredDeps, "Spice Fiber"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := discordInteraction{GuildID: "guild123", Member: viewer, Command: tt.cmd}
			got := dispatchDiscordCommand(context.Background(), i, cfg, tt.deps)
			if !contains(toLower(got.Content), toLower(tt.wantContains)) {
				t.Errorf("content %q missing %q", got.Content, tt.wantContains)
			}
		})
	}
}

// ── TestSplitRoleIDs ──────────────────────────────────────────────────────────

func TestSplitRoleIDs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"abc", []string{"abc"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{",,,", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitRoleIDs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitRoleIDs(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("splitRoleIDs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ── TestCmdListDiscordRoles ───────────────────────────────────────────────────

func TestCmdListDiscordRoles(t *testing.T) {
	stubRoles := []*discordgo.Role{
		{ID: "111", Name: "@everyone"},
		{ID: "222", Name: "Admin"},
		{ID: "333", Name: "Viewer"},
	}
	stubFetch := func(_ string) ([]*discordgo.Role, error) { return stubRoles, nil }

	tests := []struct {
		name       string
		fetchRoles func(string) ([]*discordgo.Role, error)
		wantLen    int
		wantErr    bool
	}{
		{
			name:       "filters @everyone, returns the rest",
			fetchRoles: stubFetch,
			wantLen:    2,
		},
		{
			name: "empty guild → empty slice",
			fetchRoles: func(_ string) ([]*discordgo.Role, error) {
				return []*discordgo.Role{{ID: "000", Name: "@everyone"}}, nil
			},
			wantLen: 0,
		},
		{
			name: "fetch error propagates",
			fetchRoles: func(_ string) ([]*discordgo.Role, error) {
				return nil, errors.New("discord api error")
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cmdListDiscordRoles("guild123", tt.fetchRoles)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			for _, r := range got {
				if r.Name == "@everyone" {
					t.Errorf("@everyone must be filtered out")
				}
			}
		})
	}
}

// ── TestHandleGetDiscordRoles ─────────────────────────────────────────────────

func TestHandleGetDiscordRoles(t *testing.T) {
	twoRoles := []discordRoleRow{{ID: "222", Name: "Admin"}, {ID: "333", Name: "Viewer"}}

	tests := []struct {
		name       string
		fetch      roleFetcherFn
		wantStatus int
		wantCount  int
	}{
		{
			name: "returns roles",
			fetch: func(_ string) ([]discordRoleRow, error) {
				return twoRoles, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "bot not connected → 503",
			fetch: func(_ string) ([]discordRoleRow, error) {
				return nil, errDiscordNotConnected
			},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "fetch error → 500",
			fetch: func(_ string) ([]discordRoleRow, error) {
				return nil, errors.New("discord api error")
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "empty role list returns empty array",
			fetch: func(_ string) ([]discordRoleRow, error) {
				return []discordRoleRow{}, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handleGetDiscordRolesInner(rr, "guild123", tt.fetch)
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				var got []discordRoleRow
				if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(got) != tt.wantCount {
					t.Errorf("count = %d, want %d", len(got), tt.wantCount)
				}
			}
		})
	}
}

// ── test helpers ──────────────────────────────────────────────────────────────

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := range s {
		if i+len(sub) > len(s) {
			break
		}
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
