# Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the dune-admin backend against CSRF-style attacks and unsafe SQL console usage, and add CI workflows for tests, SAST, and SCA.

**Architecture:** CORS allowlist replaces the wildcard to block cross-origin browser requests; SQL read-only guard and k8s name validation protect the two most dangerous free-form inputs; HTTP timeouts prevent connection exhaustion; three GitHub Actions workflows enforce quality on every push/PR.

**Tech Stack:** Go 1.26, `net/http`, `regexp`, `crypto/subtle` (not needed here), gorilla/websocket, GitHub Actions, gosec, govulncheck, npm audit.

---

## File Map

| File | Change |
|------|--------|
| `server.go` | Replace CORS wildcard with allowlist; add `originAllowed` + `init`; wrap server with timeouts |
| `handlers_database.go` | Add `isReadOnlySQL`; apply guard + body limit to `handleDBSQL` |
| `handlers_logs.go` | Add `isValidK8sName`; validate params in `handleLogStream`; fix `wsUpgrader.CheckOrigin` |
| `security_test.go` | New — unit tests for `isReadOnlySQL`, `isValidK8sName`, `originAllowed` |
| `.github/workflows/test.yml` | New — Go vet + test on push/PR |
| `.github/workflows/sast.yml` | New — gosec on push/PR |
| `.github/workflows/sca.yml` | New — govulncheck + npm audit on push/PR |

---

## Task 1: SQL Read-Only Guard + Body Limit

**Files:**

- Modify: `handlers_database.go`
- Create: `security_test.go`

- [ ] **Step 1: Write the failing tests**

Create `security_test.go`:

```go
package main

import "testing"

func TestIsReadOnlySQL(t *testing.T) {
 tests := []struct {
  name string
  sql  string
  want bool
 }{
  {"select uppercase", "SELECT * FROM players", true},
  {"select lowercase", "select id from players", true},
  {"select leading whitespace", "  SELECT 1", true},
  {"explain allowed", "EXPLAIN SELECT * FROM players", true},
  {"show allowed", "SHOW TABLES", true},
  {"update blocked", "UPDATE players SET x=1", false},
  {"delete blocked", "DELETE FROM players", false},
  {"insert blocked", "INSERT INTO players VALUES (1)", false},
  {"drop blocked", "DROP TABLE players", false},
  {"truncate blocked", "TRUNCATE players", false},
  {"line comment stripped, select kept", "-- comment\nSELECT 1", true},
  {"block comment stripped, select kept", "/* comment */ SELECT 1", true},
  {"block comment disguises write", "/* SELECT */ UPDATE players SET x=1", false},
  {"multiline block comment", "/*\n multi\n line\n*/SELECT 1", true},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got := isReadOnlySQL(tt.sql); got != tt.want {
    t.Errorf("isReadOnlySQL(%q) = %v, want %v", tt.sql, got, tt.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin
go test ./... -run TestIsReadOnlySQL -v
```

Expected: `undefined: isReadOnlySQL`

- [ ] **Step 3: Implement `isReadOnlySQL` in `handlers_database.go`**

Add imports `regexp` and `strings`. Replace the import block at the top of `handlers_database.go`:

```go
import (
 "fmt"
 "net/http"
 "regexp"
 "strconv"
 "strings"
)
```

Add these vars and function before `handleDBTables`:

```go
var (
 sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
 sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
 sqlReadOnlyPrefixes = []string{"select", "explain", "show"}
)

func isReadOnlySQL(sql string) bool {
 s := sqlBlockComment.ReplaceAllString(sql, " ")
 s = sqlLineComment.ReplaceAllString(s, " ")
 s = strings.ToLower(strings.TrimSpace(s))
 for _, prefix := range sqlReadOnlyPrefixes {
  if strings.HasPrefix(s, prefix) {
   return true
  }
 }
 return false
}
```

- [ ] **Step 4: Apply guard + body limit to `handleDBSQL`**

Replace the `handleDBSQL` function body in `handlers_database.go`:

```go
func handleDBSQL(w http.ResponseWriter, r *http.Request) {
 r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
 var req struct {
  SQL string `json:"sql"`
 }
 if err := decode(r, &req); err != nil {
  jsonErr(w, err, 400)
  return
 }
 if req.SQL == "" {
  jsonErr(w, fmt.Errorf("sql required"), 400)
  return
 }
 if !isReadOnlySQL(req.SQL) {
  jsonErr(w, fmt.Errorf("only SELECT, EXPLAIN, and SHOW statements are allowed"), 400)
  return
 }
 msg, ok := cmdRunSQL(req.SQL)().(msgSQL)
 if !ok {
  jsonErr(w, fmt.Errorf("internal error"), 500)
  return
 }
 if msg.err != nil {
  jsonErr(w, msg.err, 500)
  return
 }
 jsonOK(w, map[string]string{"result": msg.result})
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
go test ./... -run TestIsReadOnlySQL -v
```

Expected: all 14 subtests PASS

- [ ] **Step 6: Commit**

```bash
git add handlers_database.go security_test.go
git commit -m "security: add SQL read-only guard and body limit to handleDBSQL"
```

---

## Task 2: Log Stream Input Validation

**Files:**

- Modify: `handlers_logs.go`
- Modify: `security_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `security_test.go`:

```go
func TestIsValidK8sName(t *testing.T) {
 tests := []struct {
  name  string
  input string
  want  bool
 }{
  {"valid simple", "my-pod", true},
  {"valid with numbers", "pod-123", true},
  {"valid with dots", "my.pod.name", true},
  {"single char", "a", true},
  {"two chars", "ab", true},
  {"empty", "", false},
  {"starts with dash", "-bad-name", false},
  {"ends with dash", "bad-name-", false},
  {"uppercase blocked", "MyPod", false},
  {"space blocked", "my pod", false},
  {"semicolon injection", "pod; rm -rf /", false},
  {"backtick injection", "pod`whoami`", false},
  {"dollar injection", "pod$(id)", false},
  {"pipe injection", "pod|cat /etc/passwd", false},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got := isValidK8sName(tt.input); got != tt.want {
    t.Errorf("isValidK8sName(%q) = %v, want %v", tt.input, got, tt.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./... -run TestIsValidK8sName -v
```

Expected: `undefined: isValidK8sName`

- [ ] **Step 3: Add `isValidK8sName` to `handlers_logs.go`**

Add import `regexp` to the import block in `handlers_logs.go`:

```go
import (
 "fmt"
 "net/http"
 "regexp"
 "strings"

 "github.com/gorilla/websocket"
)
```

Add the var and function before `handleLogPods`:

```go
var k8sNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-\.]*[a-z0-9])?$`)

func isValidK8sName(name string) bool {
 return len(name) > 0 && len(name) <= 253 && k8sNameRe.MatchString(name)
}
```

- [ ] **Step 4: Apply validation to `handleLogStream`**

In `handlers_logs.go`, update `handleLogStream` to add validation after the empty check:

```go
func handleLogStream(w http.ResponseWriter, r *http.Request) {
 ns := r.URL.Query().Get("ns")
 pod := r.URL.Query().Get("pod")
 if ns == "" || pod == "" {
  http.Error(w, "ns and pod required", 400)
  return
 }
 if !isValidK8sName(ns) || !isValidK8sName(pod) {
  http.Error(w, "invalid ns or pod name", 400)
  return
 }

 conn, err := wsUpgrader.Upgrade(w, r, nil)
 if err != nil {
  return
 }
 defer conn.Close()

 cmd := fmt.Sprintf("sudo kubectl logs -f -n %s %s 2>&1", ns, pod)
 ch, cancel, err := sshStream(cmd)
 if err != nil {
  conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
  return
 }
 defer cancel()

 for line := range ch {
  if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
   return
  }
 }
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
go test ./... -run TestIsValidK8sName -v
```

Expected: all 14 subtests PASS

- [ ] **Step 6: Commit**

```bash
git add handlers_logs.go security_test.go
git commit -m "security: validate k8s namespace and pod name in handleLogStream"
```

---

## Task 3: CORS Lockdown + HTTP Timeouts

**Files:**

- Modify: `server.go`
- Modify: `handlers_logs.go`
- Modify: `security_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `security_test.go`:

```go
func TestOriginAllowed(t *testing.T) {
 orig := allowedOrigins
 allowedOrigins = []string{"https://dune-admin.layout.tools", "http://localhost:5173"}
 t.Cleanup(func() { allowedOrigins = orig })

 tests := []struct {
  name   string
  origin string
  want   bool
 }{
  {"production origin", "https://dune-admin.layout.tools", true},
  {"local vite dev", "http://localhost:5173", true},
  {"malicious site", "https://evil.com", false},
  {"empty origin", "", false},
  {"subdomain variation", "https://evil.dune-admin.layout.tools", false},
  {"http instead of https", "http://dune-admin.layout.tools", false},
  {"extra path", "https://dune-admin.layout.tools/extra", false},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got := originAllowed(tt.origin); got != tt.want {
    t.Errorf("originAllowed(%q) = %v, want %v", tt.origin, got, tt.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./... -run TestOriginAllowed -v
```

Expected: `undefined: allowedOrigins` or `undefined: originAllowed`

- [ ] **Step 3: Add `originAllowed` + `init` + updated CORS middleware to `server.go`**

Update the import block in `server.go`:

```go
import (
 "encoding/json"
 "fmt"
 "log"
 "net/http"
 "strings"
 "time"
)
```

Replace everything from the top of the file (after the `package main` line and the import block) through the end of `corsMiddleware` and `startServer`'s `log.Fatal` line, leaving the JSON helpers intact. The new content for those sections:

```go
var allowedOrigins []string

func init() {
 raw := envOr("ALLOWED_ORIGINS", "https://dune-admin.layout.tools,http://localhost:5173")
 for _, o := range strings.Split(raw, ",") {
  if o = strings.TrimSpace(o); o != "" {
   allowedOrigins = append(allowedOrigins, o)
  }
 }
}

func originAllowed(origin string) bool {
 for _, o := range allowedOrigins {
  if o == origin {
   return true
  }
 }
 return false
}

func corsMiddleware(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  origin := r.Header.Get("Origin")
  w.Header().Set("Vary", "Origin")
  if originAllowed(origin) {
   w.Header().Set("Access-Control-Allow-Origin", origin)
   w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
   w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
  }
  if r.Method == http.MethodOptions {
   w.WriteHeader(http.StatusNoContent)
   return
  }
  next.ServeHTTP(w, r)
 })
}
```

Replace the last line of `startServer` (the `log.Fatal(http.ListenAndServe(...))` call) with:

```go
 srv := &http.Server{
  Addr:              addr,
  Handler:           corsMiddleware(mux),
  ReadHeaderTimeout: 5 * time.Second,
  ReadTimeout:       15 * time.Second,
  WriteTimeout:      60 * time.Second,
  IdleTimeout:       60 * time.Second,
 }
 log.Printf("dune-admin listening on %s", addr)
 log.Fatal(srv.ListenAndServe())
```

- [ ] **Step 4: Update `wsUpgrader` in `handlers_logs.go` to use `originAllowed`**

Replace the `wsUpgrader` var declaration in `handlers_logs.go`:

```go
var wsUpgrader = websocket.Upgrader{
 CheckOrigin: func(r *http.Request) bool {
  return originAllowed(r.Header.Get("Origin"))
 },
}
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v
```

Expected: all tests in `TestIsReadOnlySQL`, `TestIsValidK8sName`, `TestOriginAllowed` PASS. Build must succeed.

- [ ] **Step 6: Verify it builds**

```bash
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 7: Commit**

```bash
git add server.go handlers_logs.go security_test.go
git commit -m "security: CORS allowlist, HTTP timeouts, WebSocket origin check"
```

---

## Task 4: CI Workflows

**Files:**

- Create: `.github/workflows/test.yml`
- Create: `.github/workflows/sast.yml`
- Create: `.github/workflows/sca.yml`

- [ ] **Step 1: Create `test.yml`**

```yaml
name: Test

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: go vet
        run: go vet ./...

      - name: go test
        run: go test -race ./...
```

- [ ] **Step 2: Create `sast.yml`**

```yaml
name: SAST

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  gosec:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Install gosec
        run: go install github.com/securego/gosec/v2/cmd/gosec@latest

      - name: Run gosec
        run: gosec -severity high -confidence high ./...
```

- [ ] **Step 3: Create `sca.yml`**

```yaml
name: SCA

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  govulncheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest

      - name: Run govulncheck
        run: govulncheck ./...

  npm-audit:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version-file: web/package.json
          cache: npm
          cache-dependency-path: web/package-lock.json

      - name: Install dependencies
        run: npm ci

      - name: npm audit
        run: npm audit --audit-level=high
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/test.yml .github/workflows/sast.yml .github/workflows/sca.yml
git commit -m "ci: add Go test, SAST (gosec), and SCA (govulncheck + npm audit) workflows"
```

---

## Self-Review Checklist

- [x] Spec §1.1 CORS → Task 3 (`originAllowed`, `corsMiddleware`, `wsUpgrader`)
- [x] Spec §1.2 Timeouts → Task 3 (`http.Server` with timeout fields)
- [x] Spec §1.3 SQL guard → Task 1 (`isReadOnlySQL`, comment stripping)
- [x] Spec §1.4 Body limit → Task 1 (`http.MaxBytesReader` in `handleDBSQL`)
- [x] Spec §1.5 Log validation → Task 2 (`isValidK8sName`, applied in `handleLogStream`)
- [x] Spec §2.1 test.yml → Task 4
- [x] Spec §2.2 sast.yml → Task 4
- [x] Spec §2.3 sca.yml → Task 4
- [x] No TBDs or placeholders
- [x] All function names consistent across tasks (`isReadOnlySQL`, `isValidK8sName`, `originAllowed`, `allowedOrigins`)
- [x] Imports updated in every modified file
