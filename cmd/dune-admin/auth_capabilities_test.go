package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAPIRegistersCapability(t *testing.T) {
	t.Run("registers route and records capability", func(t *testing.T) {
		mux := http.NewServeMux()
		handleAPI(mux, "GET /api/v1/test-cap-route", capPlayersRead, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r := httptest.NewRequest(http.MethodGet, "/api/v1/test-cap-route", nil)
		cap, ok := capabilityForRequest(mux, r)
		if !ok {
			t.Fatal("capability not found for registered route")
		}
		if cap != capPlayersRead {
			t.Errorf("capability = %q, want %q", cap, capPlayersRead)
		}
	})

	t.Run("panics on empty capability", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic for empty capability")
			}
		}()
		mux := http.NewServeMux()
		handleAPI(mux, "GET /api/v1/bad-route", "", func(w http.ResponseWriter, r *http.Request) {})
	})

	t.Run("panics on conflicting re-registration", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic for conflicting capability")
			}
		}()
		mux := http.NewServeMux()
		handleAPI(mux, "GET /api/v1/conflict-route", capPlayersRead, func(w http.ResponseWriter, r *http.Request) {})
		mux2 := http.NewServeMux()
		handleAPI(mux2, "GET /api/v1/conflict-route", capPlayersWrite, func(w http.ResponseWriter, r *http.Request) {})
	})
}

func TestCapabilityForRequest(t *testing.T) {
	mux := buildMux()

	tests := []struct {
		method string
		path   string
		want   capability
	}{
		{"GET", "/api/v1/players", capPlayersRead},
		{"GET", "/api/v1/players/123/inventory", capPlayersRead},
		{"POST", "/api/v1/players/give-items", capPlayersWrite},
		{"POST", "/api/v1/players/kick", capPlayersWrite},
		{"DELETE", "/api/v1/players/item/9", capPlayersWrite},
		{"POST", "/api/v1/players/5/backup", capBackupsManage},
		{"GET", "/api/v1/players/5/backups", capBackupsRead},
		{"POST", "/api/v1/character-backups/5/restore", capBackupsManage},
		{"GET", "/api/v1/character-backups/5/download", capBackupsRead},
		{"DELETE", "/api/v1/character-backups/5", capBackupsManage},
		{"GET", "/api/v1/blueprints/7/export", capDataExport},
		{"POST", "/api/v1/blueprints/import", capWorldWrite},
		{"GET", "/api/v1/storage", capWorldRead},
		{"GET", "/api/v1/status", capServerRead},
		{"POST", "/api/v1/update/apply", capServerControl},
		{"GET", "/api/v1/config", capConfigRead},
		{"POST", "/api/v1/config", capConfigWrite},
		{"GET", "/api/v1/market/listings", capMarketRead},
		{"GET", "/api/v1/market-bot/status", capMarketBotRead},
		{"POST", "/api/v1/market-bot/exec", capMarketBotManage},
		{"GET", "/api/v1/events", capEventsRead},
		{"PUT", "/api/v1/events/config", capEventsManage},
		{"GET", "/api/v1/welcome-package/grants", capWelcomeRead},
		{"PUT", "/api/v1/welcome-package/config", capWelcomeManage},
		{"GET", "/api/v1/battlepass/tiers", capBattlepassTrack},
		{"GET", "/api/v1/battlepass/pending", capBattlepassRead},
		{"GET", "/api/v1/battlepass/export", capDataExport},
		{"POST", "/api/v1/battlepass/import", capBattlepassManage},
		{"POST", "/api/v1/broadcast", capBroadcastSend},
		{"GET", "/api/v1/scheduled-restarts", capRestartsRead},
		{"PUT", "/api/v1/scheduled-restarts", capRestartsManage},
		{"GET", "/api/v1/logs/stream", capLogsRead},
		{"GET", "/api/v1/database/tables", capDatabaseRead},
		{"POST", "/api/v1/database/sql", capDatabaseQuery},
		{"GET", "/api/v1/db-backups", capBackupsRead},
		{"POST", "/api/v1/db-backups/restore", capBackupsManage},
		{"GET", "/api/v1/db-backups/restore/status", capBackupsRead},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			got, ok := capabilityForRequest(mux, r)
			if !ok {
				t.Fatalf("no capability resolved for %s %s", tt.method, tt.path)
			}
			if got != tt.want {
				t.Errorf("capability = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEveryAPIRouteHasCapability(t *testing.T) {
	// buildMux registers everything through handleAPI, which panics on a
	// missing capability — so reaching this point proves the invariant for
	// registered routes. Verify the table is populated and consistent.
	buildMux()
	if len(routeCapabilities) < 100 {
		t.Errorf("routeCapabilities has %d entries — expected the full API surface (>100)", len(routeCapabilities))
	}
	for pattern, cap := range routeCapabilities {
		if cap == "" {
			t.Errorf("route %q has empty capability", pattern)
		}
		if !strings.Contains(pattern, "/api/v1/") {
			t.Errorf("route %q registered via handleAPI but is not an /api/v1/ route", pattern)
		}
		if strings.Contains(pattern, "/api/v1/auth/") {
			t.Errorf("auth route %q must not be capability-gated", pattern)
		}
	}
	if _, ok := allCapabilities[capPlayersRead]; !ok {
		t.Error("allCapabilities missing players:read")
	}
	for _, cap := range routeCapabilities {
		if _, ok := allCapabilities[cap]; !ok {
			t.Errorf("capability %q used in route table but missing from allCapabilities", cap)
		}
	}
}

func TestDefaultSeedCaps(t *testing.T) {
	seed := defaultSeedCaps()
	for _, name := range seed {
		if _, ok := allCapabilities[capability(name)]; !ok {
			t.Errorf("seed capability %q not in allCapabilities", name)
		}
	}
	// The seed must be a read-only baseline — no write/manage/control caps.
	for _, banned := range []string{
		string(capPlayersWrite), string(capServerControl), string(capConfigWrite),
		string(capBackupsManage), string(capAuthManage), string(capDatabaseQuery),
	} {
		for _, name := range seed {
			if name == banned {
				t.Errorf("seed must not include privileged capability %q", banned)
			}
		}
	}
}
