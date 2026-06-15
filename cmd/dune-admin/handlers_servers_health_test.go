package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// healthFakeControl scripts GetStatus for assembleServerHealth tests.
type healthFakeControl struct {
	stubControlPlane
	status *BattlegroupStatus
	err    error
}

func (c *healthFakeControl) GetStatus(context.Context, Executor) (*BattlegroupStatus, error) {
	return c.status, c.err
}

// countingHealthControl counts GetStatus calls so the cache test can prove the
// expensive assembly is not recomputed on a cache hit.
type countingHealthControl struct {
	stubControlPlane
	calls  *atomic.Int32
	status *BattlegroupStatus
}

func (c *countingHealthControl) GetStatus(context.Context, Executor) (*BattlegroupStatus, error) {
	c.calls.Add(1)
	return c.status, nil
}

func TestHandleServersHealth_CachesAssembly(t *testing.T) {
	origCache := globalHealthCache
	c, err := newRistrettoCache[serverHealth]("test-health", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	globalHealthCache = c
	t.Cleanup(func() { globalHealthCache = origCache })

	var calls atomic.Int32
	ctrl := &countingHealthControl{calls: &calls, status: &BattlegroupStatus{Phase: "Running", Database: "Connected"}}
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "One", Cfg: ServerConfig{ID: 1, Control: "amp"}, Control: ctrl})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	call := func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/health", nil)
		rr := httptest.NewRecorder()
		handleServersHealth(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
		}
	}

	call()
	call()
	if calls.Load() != 1 {
		t.Errorf("GetStatus called %d times, want 1 (second call must hit cache)", calls.Load())
	}

	invalidateServerHealth("1")
	call()
	if calls.Load() != 2 {
		t.Errorf("after invalidation GetStatus called %d times, want 2 (cache busted)", calls.Load())
	}
}

func TestAssembleServerHealth_RunningWithRows(t *testing.T) {
	sc := &ServerContext{
		ID:   "1",
		Name: "One",
		Cfg:  ServerConfig{ID: 1, Name: "One", Control: "amp"},
		Control: &healthFakeControl{status: &BattlegroupStatus{
			Phase:    "Running",
			Database: "Connected",
			Servers: []ServerRow{
				{Ready: true, Players: 5, AgeSeconds: 100},
				{Ready: true, Players: 7, AgeSeconds: 250},
			},
		}},
	}
	h := assembleServerHealth(context.Background(), sc)

	if h.ID != 1 || h.Name != "One" || h.Control != "amp" {
		t.Errorf("identity wrong: %+v", h)
	}
	if !h.Running {
		t.Error("Running should be true for phase Running")
	}
	if h.UptimeSeconds != 250 {
		t.Errorf("UptimeSeconds = %d, want 250 (max row age)", h.UptimeSeconds)
	}
	// No DB pool → players fall back to summing control rows (5+7).
	if h.PlayersOnline != 12 {
		t.Errorf("PlayersOnline = %d, want 12 (fallback to control rows)", h.PlayersOnline)
	}
	if h.DBConnected {
		t.Error("DBConnected should be false when sc.DB is nil")
	}
}

func TestAssembleServerHealth_ControlError(t *testing.T) {
	sc := &ServerContext{
		ID:      "2",
		Name:    "Two",
		Cfg:     ServerConfig{ID: 2, Control: "local"},
		Control: &healthFakeControl{err: context.DeadlineExceeded},
	}
	h := assembleServerHealth(context.Background(), sc)
	if h.Running {
		t.Error("Running should be false on control error")
	}
	if h.Error == "" {
		t.Error("Error should be populated on control failure")
	}
}

func TestAssembleServerHealth_NilControl(t *testing.T) {
	sc := &ServerContext{ID: "3", Name: "Three", Cfg: ServerConfig{ID: 3, Control: "amp"}}
	h := assembleServerHealth(context.Background(), sc)
	if h.Running {
		t.Error("Running should be false with no control plane")
	}
	if h.ID != 3 {
		t.Errorf("ID = %d, want 3", h.ID)
	}
}

func TestHandleServersHealth_ReturnsArray(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "A", Cfg: ServerConfig{ID: 1, Control: "local"}})
	reg.Register(&ServerContext{ID: "2", Name: "B", Cfg: ServerConfig{ID: 2, Control: "amp"}})
	_ = reg.SetActive("1")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/health", nil)
	rr := httptest.NewRecorder()
	handleServersHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var out []serverHealth
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].ID != 1 || !out[0].Active {
		t.Errorf("first entry should be active server 1, got %+v", out[0])
	}
	if out[1].ID != 2 || out[1].Active {
		t.Errorf("second entry should be non-active 2, got %+v", out[1])
	}
}
