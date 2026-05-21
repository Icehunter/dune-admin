package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	if got := bearerToken("Bearer secret"); got != "secret" {
		t.Fatalf("expected bearer token, got %q", got)
	}
	if got := bearerToken("Basic secret"); got != "" {
		t.Fatalf("expected empty token for non-bearer auth, got %q", got)
	}
	if got := bearerToken("Bearer"); got != "" {
		t.Fatalf("expected empty token for malformed bearer auth, got %q", got)
	}
}

func TestAuthMiddleware(t *testing.T) {
	old := adminToken
	adminToken = "test-token"
	t.Cleanup(func() { adminToken = old })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := authMiddleware(next)

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d", missing.Code)
	}

	bad := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	badReq.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(bad, badReq)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: got %d", bad.Code)
	}

	good := httptest.NewRecorder()
	goodReq := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	goodReq.Header.Set("Authorization", "Bearer test-token")
	h.ServeHTTP(good, goodReq)
	if good.Code != http.StatusNoContent {
		t.Fatalf("good token: got %d", good.Code)
	}
}

func TestOriginAllowed(t *testing.T) {
	old := allowedOrigins
	allowedOrigins = "http://localhost:5173,https://dune-admin.layout.tools"
	t.Cleanup(func() { allowedOrigins = old })

	if !originAllowed("http://localhost:5173") {
		t.Fatal("expected configured localhost origin to be allowed")
	}
	if originAllowed("https://evil.example") {
		t.Fatal("expected unconfigured origin to be denied")
	}
	if !originAllowed("") {
		t.Fatal("expected non-browser requests without Origin to be allowed")
	}
}

func TestK8sNameValidation(t *testing.T) {
	valid := []string{"bgd-0", "funcom-operators", "pod123"}
	invalid := []string{"", "PodUpper", "pod;rm-rf", "../pod", "pod name", "-bad", "bad-"}
	for _, v := range valid {
		if !isValidK8sName(v) {
			t.Fatalf("expected %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if isValidK8sName(v) {
			t.Fatalf("expected %q to be invalid", v)
		}
	}
}

func TestReadOnlySQL(t *testing.T) {
	allowed := []string{
		"select * from dune.actors",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"show search_path",
		"explain select * from dune.actors",
	}
	denied := []string{
		"delete from dune.actors",
		"update dune.actors set id = id",
		"drop table dune.actors",
		"select * from dune.actors; drop table dune.items",
		"insert into dune.items values (1)",
	}
	for _, q := range allowed {
		if !isReadOnlySQL(q) {
			t.Fatalf("expected query to be allowed: %q", q)
		}
	}
	for _, q := range denied {
		if isReadOnlySQL(q) {
			t.Fatalf("expected query to be denied: %q", q)
		}
	}
}
