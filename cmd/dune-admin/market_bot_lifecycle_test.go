package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dune-admin/internal/marketbot"
)

func TestServerMarketBotEnabled(t *testing.T) {
	tests := []struct {
		name string
		in   *bool
		want bool
	}{
		{"nil is off (opt-in)", nil, false},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverMarketBotEnabled(ServerConfig{MarketBotEnabled: tt.in})
			if got != tt.want {
				t.Errorf("serverMarketBotEnabled(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSuffixPath(t *testing.T) {
	tests := []struct {
		path, id, want string
	}{
		{"/data/market-bot-cache.db", "s1", "/data/market-bot-cache-s1.db"},
		{"/data/state.json", "default", "/data/state-default.json"},
		{"/data/noext", "s2", "/data/noext-s2"},
	}
	for _, tt := range tests {
		if got := suffixPath(tt.path, tt.id); got != tt.want {
			t.Errorf("suffixPath(%q,%q) = %q, want %q", tt.path, tt.id, got, tt.want)
		}
	}
}

func TestServerBotPaths_DistinctPerServer(t *testing.T) {
	gcfg := appConfig{MarketBotCacheDB: "/d/cache.db", MarketBotState: "/d/state.json"}
	c1, s1 := serverBotPaths(gcfg, "s1")
	c2, s2 := serverBotPaths(gcfg, "s2")
	if c1 == c2 {
		t.Errorf("cache paths must differ per server: %q == %q", c1, c2)
	}
	if s1 == s2 {
		t.Errorf("state paths must differ per server: %q == %q", s1, s2)
	}
	if c1 != "/d/cache-s1.db" {
		t.Errorf("cache path = %q, want /d/cache-s1.db", c1)
	}
}

// Enabled toggle off → no bot, not configured.
func TestStartServerMarketBot_DisabledNoop(t *testing.T) {
	sc := &ServerContext{ID: "1", Cfg: ServerConfig{ID: 1}} // MarketBotEnabled nil
	startServerMarketBot(sc, appConfig{})
	if sc.Bot != nil {
		t.Error("bot should not start when disabled")
	}
	if sc.BotConfigured {
		t.Error("BotConfigured should be false when toggle is off")
	}
}

// Enabled but no DB → do NOT attempt a connection (the fresh-env case), but mark
// configured so status can report "configured, not running".
func TestStartServerMarketBot_EnabledNoDB(t *testing.T) {
	sc := &ServerContext{ID: "1", Cfg: ServerConfig{ID: 1, MarketBotEnabled: boolPtr(true)}}
	startServerMarketBot(sc, appConfig{})
	if sc.Bot != nil {
		t.Error("bot must not start without a DB connection (fresh env)")
	}
	if !sc.BotConfigured {
		t.Error("BotConfigured should be true when toggle is on, even without DB")
	}
}

func TestStopServerMarketBot(t *testing.T) {
	cancelled := false
	sc := &ServerContext{
		ID:        "s1",
		Bot:       &marketbot.Instance{},
		BotCancel: func() { cancelled = true },
	}
	stopServerMarketBot(sc)
	if !cancelled {
		t.Error("stop should call BotCancel")
	}
	if sc.Bot != nil || sc.BotCancel != nil {
		t.Error("stop should clear Bot and BotCancel")
	}
	// Safe on a nil/empty context.
	stopServerMarketBot(nil)
	stopServerMarketBot(&ServerContext{})
}

func TestActiveBot_PrefersCtxServerThenActive(t *testing.T) {
	ctxBot := &marketbot.Instance{}
	activeBotInst := &marketbot.Instance{}

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "active", Bot: activeBotInst})
	_ = reg.SetActive("active")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	// No ctx server → falls back to active server's bot.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market-bot/status", nil)
	if got := activeBot(req); got != activeBotInst {
		t.Error("activeBot should fall back to the active server's bot")
	}

	// Ctx server present → its bot wins.
	scCtx := &ServerContext{ID: "ctx", Bot: ctxBot}
	req2 := req.WithContext(context.WithValue(req.Context(), serverContextKey, scCtx))
	if got := activeBot(req2); got != ctxBot {
		t.Error("activeBot should prefer the request context server's bot")
	}
}

func TestLegacyServerFromFlat_MigratesMarketBotEnabled(t *testing.T) {
	orig := loadedConfig
	t.Cleanup(func() { loadedConfig = orig })
	loadedConfig = appConfig{MarketBotEnabled: boolPtr(true)}

	sc := legacyServerFromFlat(loadedConfig)
	if sc.MarketBotEnabled == nil || !*sc.MarketBotEnabled {
		t.Error("legacy default server should inherit the global market_bot_enabled toggle")
	}
}
