package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const maxJSONBodyBytes int64 = 1 << 20 // 1 MiB

var k8sNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var adminToken string
var allowedOrigins = "http://localhost:5173,https://dune-admin.layout.tools"

func init() {
	if v := os.Getenv("ADMIN_TOKEN"); v != "" {
		adminToken = v
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		allowedOrigins = v
	}
}

func generateAdminToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("generate admin token: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if adminToken == "" {
			adminToken = os.Getenv("ADMIN_TOKEN")
		}
		if adminToken == "" {
			log.Printf("security: rejecting request because ADMIN_TOKEN is not configured")
			jsonErr(w, fmt.Errorf("backend admin token is not configured"), http.StatusServiceUnavailable)
			return
		}
		provided := bearerToken(r.Header.Get("Authorization"))
		if provided == "" {
			provided = r.Header.Get("X-Admin-Token")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(adminToken)) != 1 {
			jsonErr(w, fmt.Errorf("unauthorized"), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func parseAllowedOrigins(raw string) map[string]bool {
	allowed := map[string]bool{}
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = true
		}
	}
	return allowed
}

func originAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		allowedOrigins = v
	}
	return parseAllowedOrigins(allowedOrigins)[origin]
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func warnIfExternallyBound(addr string) {
	if addr == "" || strings.HasPrefix(addr, ":") || strings.HasPrefix(addr, "0.0.0.0:") || strings.HasPrefix(addr, "[::]:") {
		log.Printf("security: LISTEN_ADDR %q binds beyond loopback; expose only behind trusted auth/TLS", addr)
		return
	}
	if !isLoopbackAddr(addr) {
		log.Printf("security: LISTEN_ADDR %q is not loopback; expose only behind trusted auth/TLS", addr)
	}
}

func isValidK8sName(s string) bool {
	return k8sNamePattern.MatchString(s)
}

func isAllowedLogTarget(ns, pod string) bool {
	if !isValidK8sName(ns) || !isValidK8sName(pod) {
		return false
	}
	return ns == globalPodNS || ns == "funcom-operators"
}

func isReadOnlySQL(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	trimmed = strings.TrimLeft(trimmed, "(\n\r\t ")
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, ";") {
		return false
	}
	allowedPrefixes := []string{"select ", "with ", "show ", "explain "}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func limitBody(w http.ResponseWriter, r *http.Request, n int64) {
	r.Body = http.MaxBytesReader(w, r.Body, n)
}
