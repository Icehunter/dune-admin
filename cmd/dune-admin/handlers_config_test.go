package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// handleGetConfig must mask per-server secrets in Servers[] — not just the
// top-level flat fields — so plaintext passwords never reach the client.
func TestHandleGetConfig_MasksServerSecrets(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	cfg := appConfig{Servers: []ServerConfig{
		{ID: 1, Name: "One", DBPass: "plaintext-pw", BrokerPass: "bpw", AmpAPIPass: "amp-pw"},
	}}
	origCfg := loadedConfig
	loadedConfig = cfg
	defer func() { loadedConfig = origCfg }()
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rr := httptest.NewRecorder()
	handleGetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var got appConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("Servers len = %d, want 1", len(got.Servers))
	}
	for name, v := range map[string]string{
		"DBPass":     got.Servers[0].DBPass,
		"BrokerPass": got.Servers[0].BrokerPass,
		"AmpAPIPass": got.Servers[0].AmpAPIPass,
	} {
		if v != masked {
			t.Errorf("Servers[0].%s = %q, want masked (no plaintext leak)", name, v)
		}
	}
}

// A scope=global save on a server-less install must NOT synthesize a "default"
// server (no gap-fill defaults, no connectAll) — it only persists global fields.
func TestHandleSaveConfig_GlobalScopeNoPhantomServer(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	reg := newServerRegistry(nil)
	origReg := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = origReg }()

	origCfg := loadedConfig
	loadedConfig = appConfig{} // no servers, no flat connection
	defer func() { loadedConfig = origCfg }()

	body, _ := json.Marshal(appConfig{ListenAddr: ":9090"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config?scope=global", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSaveConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(globalRegistry.All()) != 0 {
		t.Errorf("global save created %d server(s); want 0 (no phantom default)", len(globalRegistry.All()))
	}
	if loadedConfig.Control != "" || loadedConfig.DBHost != "" {
		t.Errorf("global save gap-filled connection fields: control=%q db_host=%q", loadedConfig.Control, loadedConfig.DBHost)
	}
	if loadedConfig.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", loadedConfig.ListenAddr)
	}
}

// A global-settings save in a multi-server install must NOT wipe Servers[],
// tear down the active server's live connection, or register a spurious
// "default" server. Per-server config is owned by the /servers endpoints.
func TestHandleSaveConfig_MultiServerPreservesServersAndConnections(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	reg := newServerRegistry(nil)
	s1 := &ServerContext{ID: "1", Name: "One", StoreScope: "1"}
	reg.Register(s1)
	reg.Register(&ServerContext{ID: "2", Name: "Two", StoreScope: "2"})
	_ = reg.SetActive("1")
	origReg := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = origReg }()

	origCfg := loadedConfig
	loadedConfig = appConfig{
		Servers: []ServerConfig{{ID: 1, Name: "One"}, {ID: 2, Name: "Two"}},
	}
	defer func() { loadedConfig = origCfg }()

	// Global-only payload (omits Servers[]) toggling a global field.
	body, _ := json.Marshal(appConfig{ListenAddr: ":9090"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSaveConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(loadedConfig.Servers) != 2 {
		t.Fatalf("Servers len = %d after global save, want 2 (preserved)", len(loadedConfig.Servers))
	}
	if globalRegistry.Get("1") != s1 {
		t.Error("active server context was replaced/torn down by a global-settings save")
	}
	if globalRegistry.Get("default") != nil {
		t.Error("global save registered a spurious 'default' server in multi-server mode")
	}
	if loadedConfig.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090 (global field applied)", loadedConfig.ListenAddr)
	}
}

// TestApplyMarketBotConfig_RemoteProxy verifies applyMarketBotConfig now only
// manages the remote-bot proxy (the embedded bots are per-server). Setting a URL
// installs the proxy; clearing it removes the proxy.
func TestApplyMarketBotConfig_RemoteProxy(t *testing.T) {
	orig := remoteBotProxy
	t.Cleanup(func() { remoteBotProxy = orig })

	remoteBotProxy = nil
	applyMarketBotConfig(appConfig{MarketBotRemoteURL: "http://bot.example:9000", MarketBotRemoteToken: "tok"})
	if remoteBotProxy == nil {
		t.Fatal("remoteBotProxy should be set when MarketBotRemoteURL is provided")
	}

	applyMarketBotConfig(appConfig{MarketBotRemoteURL: ""})
	if remoteBotProxy != nil {
		t.Error("remoteBotProxy should be cleared when MarketBotRemoteURL is empty")
	}
}

// TestApplyConfig_SetsBrokerCredentials verifies that applyConfig copies broker
// credentials into the package-level globals so hot-apply works without restart.
func TestApplyConfig_SetsBrokerCredentials(t *testing.T) {
	// Not parallel: mutates package-level globals.
	origUser := brokerUser
	origPass := brokerPass
	origLoaded := loadedConfig
	t.Cleanup(func() {
		brokerUser = origUser
		brokerPass = origPass
		loadedConfig = origLoaded
	})

	cfg := appConfig{
		BrokerUser:      "cap_user",
		BrokerPass:      "cap_pass",
		BrokerJWTSecret: "jwt_secret",
	}
	applyConfig(cfg)

	if brokerUser != "cap_user" {
		t.Errorf("brokerUser = %q, want cap_user", brokerUser)
	}
	if brokerPass != "cap_pass" {
		t.Errorf("brokerPass = %q, want cap_pass", brokerPass)
	}
	// BrokerJWTSecret is read from loadedConfig in buildCaptureJWT; confirm it is set there.
	if loadedConfig.BrokerJWTSecret != "jwt_secret" {
		t.Errorf("loadedConfig.BrokerJWTSecret = %q, want jwt_secret", loadedConfig.BrokerJWTSecret)
	}
}

// PreserveMaskedDBPass exercises the preserveMaskedSecrets function for the
// DBPass field specifically. Not parallel because subtests mutate loadedConfig.
func TestPreserveMaskedDBPass(t *testing.T) {
	t.Run("keeps explicit password", func(t *testing.T) {
		cfg := appConfig{DBPass: "new-pass"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			t.Fatalf("readFile should not be called for explicit password")
			return nil, nil
		}, "/tmp/unused")
		if cfg.DBPass != "new-pass" {
			t.Fatalf("expected explicit password to stay unchanged, got %q", cfg.DBPass)
		}
	})

	t.Run("uses existing config password from file", func(t *testing.T) {
		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return []byte("db_pass: stored-pass\n"), nil
		}, "/tmp/config.yaml")
		if cfg.DBPass != "stored-pass" {
			t.Fatalf("expected stored password from config file, got %q", cfg.DBPass)
		}
	})

	t.Run("falls back to loadedConfig when file missing", func(t *testing.T) {
		orig := loadedConfig
		loadedConfig = appConfig{DBPass: "in-memory-pass"}
		t.Cleanup(func() { loadedConfig = orig })

		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return nil, errors.New("no file")
		}, "/tmp/missing.yaml")
		if cfg.DBPass != "in-memory-pass" {
			t.Fatalf("expected in-memory fallback password, got %q", cfg.DBPass)
		}
	})
}

// TestHandleDiscover_NilExecutor verifies 503 when no executor is connected.
func TestHandleDiscover_NilExecutor(t *testing.T) {
	prevE := globalExecutor
	globalExecutor = nil
	t.Cleanup(func() { globalExecutor = prevE })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	rr := httptest.NewRecorder()
	handleDiscover(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

// TestHandleDiscover_CtxExecutorOverridesGlobal verifies that an executor stashed
// in the request context prevents the 503 guard when globalExecutor is nil.
func TestHandleDiscover_CtxExecutorOverridesGlobal(t *testing.T) {
	prevE := globalExecutor
	globalExecutor = nil
	t.Cleanup(func() { globalExecutor = prevE })

	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: "s1", Executor: exec}
	reg.Register(sc)

	inner := http.HandlerFunc(handleDiscover)
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusServiceUnavailable {
		t.Error("ctx executor should prevent the 503 guard when globalExecutor is nil")
	}
}
