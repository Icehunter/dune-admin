package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBattlepassStore_WithScope verifies that withScope returns a view bound to
// a new server_id that shares the underlying handle, and that writes through the
// view are invisible to a differently-scoped view over the same DB.
func TestBattlepassStore_WithScope(t *testing.T) {
	db := openSharedScopeDB(t)
	base := newBattlepassStore(db, scopeA)

	scoped := base.withScope(scopeB)
	if scoped.serverID != scopeB {
		t.Errorf("withScope serverID = %d, want %d", scoped.serverID, scopeB)
	}
	if scoped.db != base.db {
		t.Error("withScope must share the underlying *sql.DB handle")
	}

	if err := scoped.recordClaim("level:5", 42, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("scoped.recordClaim: %v", err)
	}

	// The scopeA base must not see scopeB's claim.
	baseKeys, err := base.claimedKeys(42)
	if err != nil {
		t.Fatalf("base.claimedKeys: %v", err)
	}
	if len(baseKeys) != 0 {
		t.Errorf("scopeA should not see scopeB claims, got %v", baseKeys)
	}

	// The scopeB view must see its own claim.
	scopedKeys, err := scoped.claimedKeys(42)
	if err != nil {
		t.Fatalf("scoped.claimedKeys: %v", err)
	}
	if _, ok := scopedKeys["level:5"]; !ok {
		t.Errorf("scopeB scope should see its own claim, got %v", scopedKeys)
	}
}

// TestBattlepassStoreForCtx verifies the handler store resolver: nil when no
// global store, "default" scope without a server context, and the server's
// StoreScope when one is attached to the request.
func TestBattlepassStoreForCtx(t *testing.T) {
	db := openSharedScopeDB(t)
	prev := globalBattlepassStore
	t.Cleanup(func() { globalBattlepassStore = prev })

	r := httptest.NewRequest(http.MethodGet, "/", nil)

	globalBattlepassStore = nil
	if got := battlepassStoreForCtx(r); got != nil {
		t.Errorf("battlepassStoreForCtx with nil global = %v, want nil", got)
	}

	globalBattlepassStore = newBattlepassStore(db, defaultServerID)
	got := battlepassStoreForCtx(r)
	if got == nil || got.serverID != defaultServerID {
		t.Errorf("no-context scope = %v, want serverID=%d", got, defaultServerID)
	}

	rc := r.WithContext(context.WithValue(r.Context(), serverContextKey, &ServerContext{StoreScope: scopeB}))
	got = battlepassStoreForCtx(rc)
	if got == nil || got.serverID != scopeB {
		t.Errorf("context scope = %v, want serverID=%d", got, scopeB)
	}
}

// TestHandleBattlepassProgress_ServerScope is the regression test for the
// multi-server collision bug: the same account_id has different claims on two
// servers, and the progress handler must return only the requesting server's
// claims. Fails against the pre-fix handler, which always reads the "default"
// scope and would see neither server's claim.
func TestHandleBattlepassProgress_ServerScope(t *testing.T) {
	db := openSharedScopeDB(t)
	prev := globalBattlepassStore
	globalBattlepassStore = newBattlepassStore(db, defaultServerID)
	t.Cleanup(func() { globalBattlepassStore = prev })

	if err := globalBattlepassStore.withScope(scopeA).recordClaim("level:5", 42, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("seed scopeA: %v", err)
	}
	if err := globalBattlepassStore.withScope(scopeB).recordClaim("level:9", 42, 200, battlepassClaimEarned); err != nil {
		t.Fatalf("seed scopeB: %v", err)
	}

	progress := func(scope int) (claims []battlepassClaim, pending int64) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/progress/42", nil)
		r = r.WithContext(context.WithValue(r.Context(), serverContextKey, &ServerContext{StoreScope: scope}))
		r.SetPathValue("accountId", "42")
		w := httptest.NewRecorder()
		handleBattlepassProgress(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("scope %d: status %d body %s", scope, w.Code, w.Body.String())
		}
		var resp struct {
			Claims  []battlepassClaim `json:"claims"`
			Pending int64             `json:"pending_intel"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("scope %d: decode: %v", scope, err)
		}
		return resp.Claims, resp.Pending
	}

	claims, pending := progress(scopeA)
	if len(claims) != 1 || claims[0].TierKey != "level:5" || pending != 100 {
		t.Errorf("scopeA progress = %+v pending=%d, want one level:5 claim pending=100", claims, pending)
	}

	claims, pending = progress(scopeB)
	if len(claims) != 1 || claims[0].TierKey != "level:9" || pending != 200 {
		t.Errorf("scopeB progress = %+v pending=%d, want one level:9 claim pending=200", claims, pending)
	}
}
