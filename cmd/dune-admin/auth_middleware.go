package main

import (
	"errors"
	"net/http"
	"strings"
	"sync"
)

// authLogf is indirected so tests can silence auth logging. It defaults to a
// zerolog-backed sink that emits an error-level "auth" log; tests may reassign
// it to a no-op.
var authLogf = func(msg string) {
	componentLog("auth").Error().Msg(msg)
}

// sessionSecretStore guards the HMAC signing key so a live config save (which
// runs on a request goroutine) can initialize it without racing reads.
var sessionSecretStore struct {
	mu     sync.RWMutex
	secret []byte
}

func currentSessionSecret() []byte {
	sessionSecretStore.mu.RLock()
	defer sessionSecretStore.mu.RUnlock()
	return sessionSecretStore.secret
}

func setSessionSecret(secret []byte) {
	sessionSecretStore.mu.Lock()
	defer sessionSecretStore.mu.Unlock()
	sessionSecretStore.secret = secret
}

// initAuthRuntime prepares auth state for the current config. Called at
// startup and after EVERY live config save, so enabling/disabling auth or
// Discord login from the UI takes effect without a restart. Each sub-init is
// idempotent and handles its own teardown.
func initAuthRuntime(cfg appConfig) {
	if !authEnabled(cfg) {
		// Tear down Discord clients so a later re-enable starts clean; the
		// middleware itself checks authEnabled per request.
		setDiscordAuth(nil, nil)
		return
	}
	if currentSessionSecret() == nil {
		secret, err := loadOrCreateSessionSecret(configDir())
		if err != nil {
			// Fail loud: auth is enabled but sessions cannot be signed.
			// Requests will be rejected with 401 (nil secret verifies nothing).
			logAuthError("session secret unavailable: " + err.Error())
			return
		}
		setSessionSecret(secret)
	}
	initPermissionsMatrix()
	initDiscordAuth(cfg)
	initAuditLog()
	if !cfgHasLoginMethod(cfg) {
		logAuthError("auth_enabled is true but no login method is configured — " +
			"all API requests will be rejected. Run `dune-admin --set-password` " +
			"or configure Discord OAuth (auth_discord_*).")
	}
}

// cfgHasLoginMethod reports whether at least one login method is usable.
func cfgHasLoginMethod(cfg appConfig) bool {
	local := cfg.AuthLocalUsername != "" && cfg.AuthLocalPasswordHash != ""
	return local || discordLoginConfigured(cfg)
}

// authExemptPath reports whether a path bypasses authentication entirely:
// the auth endpoints themselves and all non-API content (the SPA shell and
// assets must load so the login page can render).
func authExemptPath(path string) bool {
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/swagger/") || strings.HasPrefix(path, "/director/") {
		return false
	}
	return true
}

// authMiddleware enforces session authentication and capability checks for
// API requests. When auth is disabled in config it passes everything through
// untouched — byte-for-byte the pre-auth behavior.
func authMiddleware(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authEnabled(loadedConfig) {
			next.ServeHTTP(w, r)
			return
		}
		if authExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		claims, ok := authenticateRequest(w, r)
		if !ok {
			return
		}

		// CSRF defense-in-depth: cookies are SameSite=Lax, and any cross-site
		// mutation that still carries one is rejected by the Origin check.
		// Requests without an Origin header are not browser cross-site
		// requests (browsers always send Origin on cross-site mutations), so
		// only enforce when the header is present.
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			r.Header.Get("Origin") != "" && !originAllowedForRequest(r) {
			jsonErr(w, errors.New("origin not allowed"), http.StatusForbidden)
			return
		}

		if !sessionAllowed(mux, r, claims) {
			jsonErr(w, errors.New("insufficient permissions"), http.StatusForbidden)
			return
		}

		auditRequest(w, r, claims, next)
	})
}

// authenticateRequest verifies the session cookie and lazily refreshes a
// stale Discord role snapshot. Writes the error response itself and returns
// ok=false when the request must not proceed.
func authenticateRequest(w http.ResponseWriter, r *http.Request) (*sessionClaims, bool) {
	claims, err := sessionFromRequest(r)
	if err != nil {
		jsonErr(w, errors.New("authentication required"), http.StatusUnauthorized)
		return nil, false
	}
	// Lazy Discord role refresh: no-op for local sessions, fresh snapshots,
	// or when Discord auth is not wired.
	refreshed, err := refreshDiscordSession(w, r, claims)
	if err != nil {
		clearSessionCookie(w, r)
		jsonErr(w, errors.New("session no longer valid"), http.StatusUnauthorized)
		return nil, false
	}
	return refreshed, true
}

// sessionFromRequest extracts and verifies the session cookie.
func sessionFromRequest(r *http.Request) (*sessionClaims, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	secret := currentSessionSecret()
	if secret == nil {
		return nil, errors.New("session secret not initialized")
	}
	return verifySession(c.Value, secret)
}

// sessionAllowed checks the session's capabilities against the route's
// required capability. Owners bypass the matrix. Unmapped API routes fail
// closed for everyone.
func sessionAllowed(mux *http.ServeMux, r *http.Request, claims *sessionClaims) bool {
	// The bare /api/v1/status heartbeat is shell infrastructure — the SPA
	// polls it to render the app frame, connection badges, and version. Any
	// authenticated session may read it; it carries no privileged data and
	// gating it would trap a zero-capability user on the error screen.
	if r.URL.Path == "/api/v1/status" {
		return true
	}
	cap, ok := capabilityForRequest(mux, r)
	if !ok {
		// Routes outside the capability table: swagger is read-level, the
		// director proxy is control-level. Anything else fails closed for
		// everyone (including owners) so a registration mistake never opens
		// an ungated route.
		switch {
		case strings.HasPrefix(r.URL.Path, "/swagger/"):
			cap = capServerRead
		case strings.HasPrefix(r.URL.Path, "/director/"):
			cap = capServerControl
		default:
			return false
		}
	}
	if claims.Owner {
		return true
	}
	return capsForSession(claims)[cap]
}

// auditRequest runs the handler, recording authenticated mutations. The
// audit sink is installed by the audit layer; nil means no logging.
func auditRequest(w http.ResponseWriter, r *http.Request, claims *sessionClaims, next http.Handler) {
	sink := currentAuditSink()
	if r.Method == http.MethodGet || r.Method == http.MethodHead || sink == nil {
		next.ServeHTTP(w, r)
		return
	}
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(rec, r)
	sink(claims, r, rec.status)
}

// auditSinkState holds the mutation recorder, installed in auth_audit.go.
// Guarded because a live config save can install it while requests read.
var auditSinkState struct {
	mu   sync.RWMutex
	sink func(claims *sessionClaims, r *http.Request, status int)
}

func currentAuditSink() func(*sessionClaims, *http.Request, int) {
	auditSinkState.mu.RLock()
	defer auditSinkState.mu.RUnlock()
	return auditSinkState.sink
}

func setAuditSink(sink func(*sessionClaims, *http.Request, int)) {
	auditSinkState.mu.Lock()
	defer auditSinkState.mu.Unlock()
	auditSinkState.sink = sink
}

// statusRecorder captures the response status for audit logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// appCSP is the strict policy for the SPA: no inline scripts (the Vite build
// emits none), inline styles allowed (Tailwind/HeroUI inject them).
// img-src allows any https host so the LiveMap can load tiles/images from
// external CDNs (cdn.th.gl, the configurable VITE_CDN_BASE_URL host, Discord
// avatars). This mirrors connect-src, which already allows https:, and avoids a
// brittle hardcoded host list that would break self-hosters overriding the CDN.
const appCSP = "default-src 'self'; img-src 'self' data: https:; " +
	"connect-src 'self' ws: wss: https:; style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; frame-ancestors 'none'"

// swaggerCSP relaxes script-src for the Swagger UI, whose index.html
// bootstraps SwaggerUIBundle from an inline <script> and uses blob workers.
const swaggerCSP = "default-src 'self'; img-src 'self' data:; " +
	"script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"worker-src 'self' blob:; font-src 'self' data:"

// securityHeadersMiddleware adds browser hardening headers when auth is
// enabled. Gated on the auth flag so existing deployments see zero change
// until they opt in.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authEnabled(loadedConfig) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if strings.HasPrefix(r.URL.Path, "/swagger/") {
				h.Set("Content-Security-Policy", swaggerCSP)
			} else {
				h.Set("Content-Security-Policy", appCSP)
			}
			if requestIsTLS(r) {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
		}
		next.ServeHTTP(w, r)
	})
}

func logAuthError(msg string) {
	// authLogf indirection so tests can run without spamming output.
	authLogf(msg)
}
