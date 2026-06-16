# In-app Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an owner-gated Diagnostics tab that streams dune-admin's own logs live (raw) and offers a "Report an issue" flow that opens a prefilled GitHub issue plus a hard-redacted diagnostics bundle.

**Architecture:** A fixed-size in-memory ring buffer is added as a second zerolog sink so every log event is captured in addition to stderr. Pure redaction + environment helpers build the artifacts that leave the machine. Four owner-gated HTTP endpoints expose live streaming (raw, reusing the existing gorilla/websocket pattern) and the redacted report/bundle. A new React tab consumes them, reusing LogsTab's streaming machinery.

**Tech Stack:** Go (`package main`, zerolog, gorilla/websocket, stdlib `archive/zip`), React + TypeScript (HeroUI v3, Vite), pnpm.

**Reference spec:** `docs/superpowers/specs/2026-06-15-in-app-diagnostics-design.md`

**Conventions (read before starting):**
- Everything is `package main` in `cmd/dune-admin/`. Never create sub-packages.
- TDD: write the failing test first. Run `make test-race` (or `go test -race ./...`) to verify.
- Run `make verify` before the final commit. Run `make gosec` separately (the pre-push hook gates on it) since this PR touches file paths / zip writing.
- Use `jsonOK` / `jsonErr` from `server.go`; never write to `http.ResponseWriter` directly (except the WebSocket and zip streaming endpoints, which must).
- Frontend: all API calls via `api/client.ts`; semantic colour tokens only; no `any`; `cd web && pnpm lint` must pass.
- Do not commit to `main` (push to main triggers a prod Cloudflare deploy). Work on `feature/in-app-diagnostics`.

---

## File Structure

New (all in `cmd/dune-admin/`):
- `logring.go` / `logring_test.go` — the `logRing` ring buffer + subscription.
- `redact.go` / `redact_test.go` — `redactLine` masking rules (security-critical).
- `diagnostics.go` / `diagnostics_test.go` — `environmentSummary`, `buildEnvironment`, `buildReport`, `writeDiagnosticsBundle`.
- `handlers_diagnostics.go` / `handlers_diagnostics_test.go` — the four HTTP handlers.

Modified:
- `cmd/dune-admin/logging.go` — wire the ring into `initLogging` via `MultiLevelWriter`.
- `cmd/dune-admin/auth_capabilities.go` — add `capDiagnosticsRead`.
- `cmd/dune-admin/server.go` — register four routes.
- `web/src/api/client.ts` — `api.diagnostics` namespace + types.
- `web/src/types.ts` — add `'diagnostics'` to `TabId`.
- `web/src/components/app/nav.ts` — tab id, icon, capability.
- `web/src/components/app/AppRoutes.tsx` — lazy import + `renderTab`.
- `web/src/tabs/DiagnosticsTab.tsx` — new tab (created in a frontend task).

---

## Task 1: Log ring buffer

**Files:**
- Create: `cmd/dune-admin/logring.go`
- Test: `cmd/dune-admin/logring_test.go`

The ring receives the JSON event bytes zerolog produces (zerolog always encodes to JSON before the writer; `ConsoleWriter` is just another writer that pretty-prints for stderr). Implementing `zerolog.LevelWriter` lets us capture the level without parsing.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogRingOverflowDropsOldest(t *testing.T) {
	r := newLogRing(3)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		_, _ = r.WriteLevel(zerolog.InfoLevel, []byte(m+"\n"))
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if got[i].Line != w {
			t.Errorf("entry %d = %q, want %q", i, got[i].Line, w)
		}
	}
}

func TestLogRingCapturesLevelAndTrimsNewline(t *testing.T) {
	r := newLogRing(10)
	_, _ = r.WriteLevel(zerolog.WarnLevel, []byte("hello\n"))
	got := r.Snapshot()
	if len(got) != 1 || got[0].Line != "hello" || got[0].Level != "warn" {
		t.Fatalf("got %+v", got)
	}
}

func TestLogRingSnapshotIsCopy(t *testing.T) {
	r := newLogRing(5)
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("x\n"))
	s := r.Snapshot()
	s[0].Line = "mutated"
	if r.Snapshot()[0].Line != "x" {
		t.Fatal("Snapshot must return a copy, not aliased backing storage")
	}
}

func TestLogRingSubscribeReceivesLiveAndCancelStops(t *testing.T) {
	r := newLogRing(5)
	ch, cancel := r.Subscribe()
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("live\n"))
	if got := <-ch; got.Line != "live" {
		t.Fatalf("got %q, want live", got.Line)
	}
	cancel()
	// After cancel, the channel is closed and further writes are not delivered.
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("after\n"))
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestLogRingConcurrentWriters(t *testing.T) {
	r := newLogRing(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("x\n"))
			_ = r.Snapshot()
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run TestLogRing -v`
Expected: FAIL — `undefined: newLogRing`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ringLine is one captured log event: the JSON bytes zerolog produced (newline
// trimmed) plus the level it was emitted at.
type ringLine struct {
	Level string `json:"level"`
	Line  string `json:"line"`
}

// logRing is a fixed-capacity in-memory ring buffer of recent log events. It
// implements zerolog.LevelWriter so it can be installed as a second sink
// alongside stderr. Writes never block: on overflow the oldest entry is
// dropped, and slow subscribers drop events rather than stalling the logger.
type logRing struct {
	mu      sync.Mutex
	buf     []ringLine
	start   int
	n       int
	subs    map[int]chan ringLine
	nextSub int
}

func newLogRing(capacity int) *logRing {
	if capacity < 1 {
		capacity = 1
	}
	return &logRing{
		buf:  make([]ringLine, capacity),
		subs: make(map[int]chan ringLine),
	}
}

func (r *logRing) Write(p []byte) (int, error) {
	return r.WriteLevel(zerolog.NoLevel, p)
}

func (r *logRing) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	entry := ringLine{Level: level.String(), Line: strings.TrimRight(string(p), "\n")}
	r.mu.Lock()
	idx := (r.start + r.n) % len(r.buf)
	r.buf[idx] = entry
	if r.n < len(r.buf) {
		r.n++
	} else {
		r.start = (r.start + 1) % len(r.buf)
	}
	for _, ch := range r.subs {
		select {
		case ch <- entry:
		default: // slow subscriber — drop rather than block logging
		}
	}
	r.mu.Unlock()
	return len(p), nil
}

// Snapshot returns a copy of the buffered entries oldest-first.
func (r *logRing) Snapshot() []ringLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ringLine, r.n)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

// Subscribe returns a channel of live entries and a cancel func that closes it.
func (r *logRing) Subscribe() (<-chan ringLine, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextSub
	r.nextSub++
	ch := make(chan ringLine, 256)
	r.subs[id] = ch
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if c, ok := r.subs[id]; ok {
				delete(r.subs, id)
				close(c)
			}
		})
	}
	return ch, cancel
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run TestLogRing -race -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/logring.go cmd/dune-admin/logring_test.go
git commit -m "feat(diagnostics): add in-memory log ring buffer"
```

---

## Task 2: Wire the ring into logging setup

**Files:**
- Modify: `cmd/dune-admin/logging.go`
- Test: `cmd/dune-admin/logring_test.go` (add one integration test)

- [ ] **Step 1: Write the failing test**

Append to `logring_test.go`:

```go
func TestInitLoggingCapturesToRing(t *testing.T) {
	t.Setenv("DIAG_LOG_BUFFER", "50")
	initLogging()
	if globalLogRing == nil {
		t.Fatal("globalLogRing must be initialised by initLogging")
	}
	componentLog("test").Info().Msg("ring-capture-probe")
	found := false
	for _, e := range globalLogRing.Snapshot() {
		if strings.Contains(e.Line, "ring-capture-probe") {
			found = true
		}
	}
	if !found {
		t.Fatal("log event was not captured in the ring")
	}
}
```

Add `"strings"` to the test file imports if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run TestInitLoggingCapturesToRing -v`
Expected: FAIL — `undefined: globalLogRing`.

- [ ] **Step 3: Write minimal implementation**

In `logging.go`, add the global and buffer-size helper, and rebuild `initLogging`:

```go
// globalLogRing captures recent log events in memory for the Diagnostics tab.
var globalLogRing *logRing

// logBufferSize returns the ring capacity from DIAG_LOG_BUFFER (default 2000).
func logBufferSize() int {
	if v := os.Getenv("DIAG_LOG_BUFFER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 2000
}

func initLogging() {
	zerolog.SetGlobalLevel(parseLogLevel(os.Getenv("LOG_LEVEL")))

	var base io.Writer = os.Stderr
	if !strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		base = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
	}
	globalLogRing = newLogRing(logBufferSize())
	appLogger = zerolog.New(zerolog.MultiLevelWriter(base, globalLogRing)).
		With().Timestamp().Logger()

	stdlog.SetFlags(0)
	stdlog.SetOutput(appLogger)
}
```

Add `"strconv"` to the `logging.go` import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run "TestInitLoggingCapturesToRing|TestLogRing" -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/logging.go cmd/dune-admin/logring_test.go
git commit -m "feat(diagnostics): capture log events into the ring buffer"
```

---

## Task 3: Redaction rules

**Files:**
- Create: `cmd/dune-admin/redact.go`
- Test: `cmd/dune-admin/redact_test.go`

This is the security-critical unit. Every artifact that leaves the machine passes through `redactLine`.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestRedactLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // substring that MUST appear (redacted form)
		gone string // substring that MUST NOT appear
	}{
		{"ipv4", "dialing 192.168.0.59:8080 now", "[redacted-host]", "192.168.0.59"},
		{"bearer", `Authorization: Bearer abc.def.ghi`, "[redacted-token]", "abc.def.ghi"},
		{"service token", `ServiceAuthToken=SECRETVALUE123`, "[redacted-token]", "SECRETVALUE123"},
		{"kv password", `password=hunter2 extra`, "[redacted-token]", "hunter2"},
		{"ssh target", `ssh amp@192.168.0.59`, "[redacted-host]", "amp@192.168.0.59"},
		{"home path", `/Users/icehunter/.dune-admin/config.yaml`, "[redacted-path]", "icehunter"},
		{"account id", `account_id=1099511628800 done`, "[redacted-id]", "1099511628800"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redactLine(c.in)
			if c.want != "" && !contains(got, c.want) {
				t.Errorf("redactLine(%q) = %q, want substring %q", c.in, got, c.want)
			}
			if c.gone != "" && contains(got, c.gone) {
				t.Errorf("redactLine(%q) = %q, must not contain %q", c.in, got, c.gone)
			}
		})
	}
}

func TestRedactLineLeavesSafeTextAlone(t *testing.T) {
	in := `level=info component=handlers msg="server started"`
	if got := redactLine(in); got != in {
		t.Errorf("redactLine altered safe text: %q -> %q", in, got)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run TestRedactLine -v`
Expected: FAIL — `undefined: redactLine`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import "regexp"

// redactRule masks a category of sensitive content. Order matters: token and
// path rules run before the host rule so their internal host-like substrings
// are masked under the more specific label.
type redactRule struct {
	re   *regexp.Regexp
	repl string
}

var redactRules = []redactRule{
	// bearer / authorization tokens
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`), "Bearer [redacted-token]"},
	// ServiceAuthToken / token / key / password / secret = value
	{regexp.MustCompile(`(?i)(serviceauthtoken|token|api[_-]?key|key|password|passwd|secret)\s*[=:]\s*\S+`), "$1=[redacted-token]"},
	// numeric account / player / fls ids in key=value form
	{regexp.MustCompile(`(?i)(account_id|player_id|fls_id|owner_id)\s*[=:]\s*\d+`), "$1=[redacted-id]"},
	// home directory paths (mask the username segment and below)
	{regexp.MustCompile(`(?i)(/home/|/Users/|C:\\Users\\)[^\s"']+`), "[redacted-path]"},
	// user@host ssh targets
	{regexp.MustCompile(`\b[A-Za-z0-9._\-]+@[A-Za-z0-9.\-]+(:\d+)?\b`), "[redacted-host]"},
	// host:port and bare IPv4
	{regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`), "[redacted-host]"},
}

// redactLine masks sensitive content for any artifact that leaves the machine.
// Defaults to masking on ambiguity (never passes suspicious content through).
func redactLine(s string) string {
	for _, rule := range redactRules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run TestRedactLine -v`
Expected: PASS. If the "kv password" case captures the label group differently, adjust the expected substring to match `password=[redacted-token]` — the test asserts the secret value is gone, which is the invariant that matters.

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/redact.go cmd/dune-admin/redact_test.go
git commit -m "feat(diagnostics): add redaction rules for exported artifacts"
```

---

## Task 4: Environment summary (allowlist)

**Files:**
- Create: `cmd/dune-admin/diagnostics.go`
- Test: `cmd/dune-admin/diagnostics_test.go`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"runtime"
	"testing"
)

func TestBuildEnvironmentAllowlist(t *testing.T) {
	origCfg := loadedConfig
	enabled := true
	loadedConfig = appConfig{Control: "amp", AuthEnabled: &enabled, MarketBotEnabled: &enabled}
	t.Cleanup(func() { loadedConfig = origCfg })

	env := buildEnvironment()
	if env.ControlPlane != "amp" {
		t.Errorf("ControlPlane = %q, want amp", env.ControlPlane)
	}
	if !env.AuthEnabled || !env.MarketBot {
		t.Errorf("expected auth + market bot enabled, got %+v", env)
	}
	if env.GoVersion != runtime.Version() || env.OS != runtime.GOOS {
		t.Errorf("runtime fields wrong: %+v", env)
	}
	if env.Version != AppVersion {
		t.Errorf("Version = %q, want %q", env.Version, AppVersion)
	}
}

func TestBuildEnvironmentControlDefault(t *testing.T) {
	origCfg := loadedConfig
	loadedConfig = appConfig{} // blank control
	t.Cleanup(func() { loadedConfig = origCfg })
	if got := buildEnvironment().ControlPlane; got != "local" {
		t.Errorf("blank control should default to local, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run TestBuildEnvironment -v`
Expected: FAIL — `undefined: buildEnvironment`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import "runtime"

// environmentSummary is the allowlist-only environment block included in
// diagnostics artifacts. Adding a field is a deliberate code change — nothing
// is emitted that is not named here.
type environmentSummary struct {
	Version      string `json:"version"`
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	ControlPlane string `json:"control_plane"`
	AuthEnabled  bool   `json:"auth_enabled"`
	MarketBot    bool   `json:"market_bot_enabled"`
	ServerCount  int    `json:"active_server_count"`
}

func buildEnvironment() environmentSummary {
	return environmentSummary{
		Version:      AppVersion,
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		ControlPlane: controlOrDefault(loadedConfig.Control),
		AuthEnabled:  authEnabled(loadedConfig),
		MarketBot:    loadedConfig.MarketBotEnabled != nil && *loadedConfig.MarketBotEnabled,
		ServerCount:  len(globalRegistry.All()),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run TestBuildEnvironment -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/diagnostics.go cmd/dune-admin/diagnostics_test.go
git commit -m "feat(diagnostics): add allowlist environment summary"
```

---

## Task 5: Report body + bundle assembly

**Files:**
- Modify: `cmd/dune-admin/diagnostics.go`
- Test: `cmd/dune-admin/diagnostics_test.go`

- [ ] **Step 1: Write the failing test**

Append to `diagnostics_test.go`:

```go
import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
)

func TestBuildReportRedactsAndTrims(t *testing.T) {
	lines := []ringLine{
		{Level: "info", Line: "dialing 192.168.0.59:8080"},
		{Level: "error", Line: "ServiceAuthToken=SECRET123 failed"},
	}
	env := environmentSummary{Version: "1.2.3", ControlPlane: "amp"}

	title, body := buildReport(lines, env, 8000)
	if !strings.Contains(title, "1.2.3") {
		t.Errorf("title missing version: %q", title)
	}
	if strings.Contains(body, "192.168.0.59") || strings.Contains(body, "SECRET123") {
		t.Fatalf("body leaked sensitive content:\n%s", body)
	}
	if !strings.Contains(body, "amp") {
		t.Errorf("body missing environment summary")
	}
}

func TestBuildReportTruncates(t *testing.T) {
	var lines []ringLine
	for i := 0; i < 5000; i++ {
		lines = append(lines, ringLine{Level: "info", Line: "padding line of text"})
	}
	_, body := buildReport(lines, environmentSummary{}, 2000)
	if len(body) > 2000 {
		t.Errorf("body = %d bytes, want <= 2000", len(body))
	}
	if !strings.Contains(body, "truncated") {
		t.Errorf("oversized body must carry a truncation marker")
	}
}

func TestWriteDiagnosticsBundleContents(t *testing.T) {
	lines := []ringLine{{Level: "info", Line: "user amp@192.168.0.59 connected"}}
	env := environmentSummary{Version: "9.9.9", ControlPlane: "local"}

	var buf bytes.Buffer
	if err := writeDiagnosticsBundle(&buf, lines, env); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		names[f.Name] = string(b)
	}
	if _, ok := names["app.log"]; !ok {
		t.Fatal("bundle missing app.log")
	}
	if _, ok := names["environment.txt"]; !ok {
		t.Fatal("bundle missing environment.txt")
	}
	if strings.Contains(names["app.log"], "192.168.0.59") {
		t.Fatalf("app.log not redacted: %s", names["app.log"])
	}
	if !strings.Contains(names["environment.txt"], "9.9.9") {
		t.Errorf("environment.txt missing version")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run "TestBuildReport|TestWriteDiagnosticsBundle" -v`
Expected: FAIL — `undefined: buildReport`, `undefined: writeDiagnosticsBundle`.

- [ ] **Step 3: Write minimal implementation**

Append to `diagnostics.go` (add `"archive/zip"`, `"fmt"`, `"io"`, `"strings"` to its imports):

```go
// reportLogLines is how many recent log lines the issue body carries inline.
const reportLogLines = 50

// buildReport returns a GitHub issue title and a redacted markdown body
// (environment summary + recent log lines), trimmed to maxBytes.
func buildReport(lines []ringLine, env environmentSummary, maxBytes int) (title, body string) {
	title = fmt.Sprintf("[bug] dune-admin %s", env.Version)

	var b strings.Builder
	b.WriteString("## Environment\n\n")
	b.WriteString(environmentMarkdown(env))
	b.WriteString("\n## Recent logs\n\n```\n")

	start := 0
	if len(lines) > reportLogLines {
		start = len(lines) - reportLogLines
	}
	for _, e := range lines[start:] {
		b.WriteString(redactLine(e.Line))
		b.WriteByte('\n')
	}
	b.WriteString("```\n")

	body = b.String()
	if len(body) > maxBytes {
		marker := "\n... (truncated, see attached bundle)\n```\n"
		cut := maxBytes - len(marker)
		if cut < 0 {
			cut = 0
		}
		body = body[:cut] + marker
	}
	return title, body
}

func environmentMarkdown(env environmentSummary) string {
	return fmt.Sprintf(
		"| field | value |\n|---|---|\n"+
			"| version | %s |\n| go | %s |\n| os/arch | %s/%s |\n"+
			"| control plane | %s |\n| auth enabled | %t |\n"+
			"| market bot | %t |\n| active servers | %d |\n",
		env.Version, env.GoVersion, env.OS, env.Arch,
		env.ControlPlane, env.AuthEnabled, env.MarketBot, env.ServerCount,
	)
}

// writeDiagnosticsBundle writes a zip with a redacted app.log and an
// environment.txt to w.
func writeDiagnosticsBundle(w io.Writer, lines []ringLine, env environmentSummary) error {
	zw := zip.NewWriter(w)

	logf, err := zw.Create("app.log")
	if err != nil {
		return fmt.Errorf("bundle app.log: %w", err)
	}
	for _, e := range lines {
		if _, err := io.WriteString(logf, redactLine(e.Line)+"\n"); err != nil {
			return fmt.Errorf("write app.log: %w", err)
		}
	}

	envf, err := zw.Create("environment.txt")
	if err != nil {
		return fmt.Errorf("bundle environment.txt: %w", err)
	}
	if _, err := io.WriteString(envf, environmentMarkdown(env)); err != nil {
		return fmt.Errorf("write environment.txt: %w", err)
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("close bundle: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run "TestBuildReport|TestWriteDiagnosticsBundle" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/diagnostics.go cmd/dune-admin/diagnostics_test.go
git commit -m "feat(diagnostics): assemble redacted report body and bundle"
```

---

## Task 6: Diagnostics capability

**Files:**
- Modify: `cmd/dune-admin/auth_capabilities.go`

- [ ] **Step 1: Add the constant**

In the capability `const` block (after `capAuthManage`), add:

```go
	capDiagnosticsRead  capability = "diagnostics:read"
```

- [ ] **Step 2: Add the description**

In the `allCapabilities` map, add:

```go
	capDiagnosticsRead: "View dune-admin's own logs and report an issue",
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/dune-admin/`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add cmd/dune-admin/auth_capabilities.go
git commit -m "feat(diagnostics): add diagnostics:read capability"
```

---

## Task 7: HTTP handlers + routes

**Files:**
- Create: `cmd/dune-admin/handlers_diagnostics.go`
- Modify: `cmd/dune-admin/server.go`
- Test: `cmd/dune-admin/handlers_diagnostics_test.go`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDiagnosticsEnvironment(t *testing.T) {
	origCfg := loadedConfig
	loadedConfig = appConfig{Control: "docker"}
	t.Cleanup(func() { loadedConfig = origCfg })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/environment", nil)
	handleDiagnosticsEnvironment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var env environmentSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.ControlPlane != "docker" {
		t.Errorf("ControlPlane = %q, want docker", env.ControlPlane)
	}
}

func TestHandleDiagnosticsReport(t *testing.T) {
	globalLogRing = newLogRing(10)
	_, _ = globalLogRing.WriteLevel(0, []byte("dialing 192.168.0.59:8080\n"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/report", nil)
	handleDiagnosticsReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var out struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Body, "192.168.0.59") {
		t.Fatalf("report leaked host: %s", out.Body)
	}
}

func TestHandleDiagnosticsBundle(t *testing.T) {
	globalLogRing = newLogRing(10)
	_, _ = globalLogRing.WriteLevel(0, []byte("ok\n"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "diagnostics.zip") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dune-admin/ -run TestHandleDiagnostics -v`
Expected: FAIL — `undefined: handleDiagnosticsEnvironment` (etc).

- [ ] **Step 3: Write minimal implementation**

Create `handlers_diagnostics.go`:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// issueRepo is the upstream repository new issues are filed against,
// regardless of which fork is running.
const issueRepo = "Icehunter/dune-admin"

// @Summary dune-admin environment summary
// @Tags diagnostics
// @Produce json
// @Success 200 {object} environmentSummary
// @Router /api/v1/diagnostics/environment [get]
func handleDiagnosticsEnvironment(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, buildEnvironment())
}

// @Summary Build a redacted GitHub issue title and body
// @Tags diagnostics
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/diagnostics/report [get]
func handleDiagnosticsReport(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		jsonErr(w, fmt.Errorf("logging not initialised"), http.StatusServiceUnavailable)
		return
	}
	// ~6KB keeps the prefilled issue URL comfortably under the ~8KB cap.
	title, body := buildReport(globalLogRing.Snapshot(), buildEnvironment(), 6000)
	jsonOK(w, map[string]string{"title": title, "body": body, "repo": issueRepo})
}

// @Summary Download a redacted diagnostics bundle (zip)
// @Tags diagnostics
// @Produce application/zip
// @Router /api/v1/diagnostics/bundle [get]
func handleDiagnosticsBundle(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		jsonErr(w, fmt.Errorf("logging not initialised"), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="diagnostics.zip"`)
	if err := writeDiagnosticsBundle(w, globalLogRing.Snapshot(), buildEnvironment()); err != nil {
		log.Printf("handleDiagnosticsBundle: %v", err)
		// Headers/body may be partially written; nothing more we can do safely.
	}
}

// @Summary Stream dune-admin's own logs (raw) via WebSocket
// @Tags diagnostics
// @Produce text/plain
// @Router /api/v1/diagnostics/logs/stream [get]
func handleDiagnosticsLogStream(w http.ResponseWriter, r *http.Request) {
	if globalLogRing == nil {
		http.Error(w, "logging not initialised", http.StatusServiceUnavailable)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	// Replay the buffer, then tail live. Subscribe before replaying so no event
	// emitted between snapshot and subscribe is lost (at worst a line repeats).
	ch, cancel := globalLogRing.Subscribe()
	defer cancel()
	for _, e := range globalLogRing.Snapshot() {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(e.Line)); err != nil {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(e.Line)); err != nil {
				return
			}
		}
	}
}
```

In `server.go`, add the routes near the existing logs routes (around line 249):

```go
	// ── diagnostics (dune-admin self-logs) ───────────────────────────────────
	handleAPI(mux, "GET /api/v1/diagnostics/environment", capDiagnosticsRead, handleDiagnosticsEnvironment)
	handleAPI(mux, "GET /api/v1/diagnostics/report", capDiagnosticsRead, handleDiagnosticsReport)
	handleAPI(mux, "GET /api/v1/diagnostics/bundle", capDiagnosticsRead, handleDiagnosticsBundle)
	handleAPI(mux, "GET /api/v1/diagnostics/logs/stream", capDiagnosticsRead, handleDiagnosticsLogStream)
```

These tests call the handlers directly (the established pattern in this codebase), so they exercise handler logic, not the capability middleware. Capability enforcement comes from registering the routes via `handleAPI(..., capDiagnosticsRead, ...)` and is covered by the existing auth-middleware test suite (`auth_middleware_test.go`) — the registration in `server.go` is the assertion that these routes are gated. Owners bypass the matrix (`auth_middleware.go:184`); a session without `diagnostics:read` gets 403. Do not duplicate the middleware test here.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/dune-admin/ -run TestHandleDiagnostics -race -v`
Expected: PASS.

- [ ] **Step 5: Run gosec (touches zip/file writing)**

Run: `make gosec`
Expected: no high-severity findings for the new code. If the zip writer trips a false positive, suppress with `// #nosec G110,G702 -- bounded ring buffer, no decompression bomb` and the required dual IDs.

- [ ] **Step 6: Commit**

```bash
git add cmd/dune-admin/handlers_diagnostics.go cmd/dune-admin/handlers_diagnostics_test.go cmd/dune-admin/server.go
git commit -m "feat(diagnostics): add owner-gated diagnostics endpoints"
```

---

## Task 8: Frontend API client

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add types and the namespace**

Near the other exported types in `client.ts`, add:

```ts
export interface DiagnosticsEnvironment {
  version: string
  go_version: string
  os: string
  arch: string
  control_plane: string
  auth_enabled: boolean
  market_bot_enabled: boolean
  active_server_count: number
}

export interface DiagnosticsReport {
  title: string
  body: string
  repo: string
}
```

Inside the `export const api = { ... }` object, add a `diagnostics` namespace (place it next to `logs`):

```ts
  diagnostics: {
    environment: () => req<DiagnosticsEnvironment>('GET', '/diagnostics/environment'),
    report: () => req<DiagnosticsReport>('GET', '/diagnostics/report'),
    // bundle + stream are not JSON: built from the resolved bases at call sites
    bundleUrl: () => `${getApiBase()}/diagnostics/bundle`,
    streamUrl: () => `${getWsBase()}/diagnostics/logs/stream`,
  },
```

`getApiBase` is already defined in this file; export is not required since `bundleUrl` lives in the same module. If `getApiBase` is not in scope where `api` is declared, reference `apiBase` (the exported constant) instead: `bundleUrl: () => \`${apiBase}/diagnostics/bundle\``.

- [ ] **Step 2: Verify types compile**

Run: `cd web && pnpm lint`
Expected: no errors from `client.ts`.

- [ ] **Step 3: Commit**

```bash
git add web/src/api/client.ts
git commit -m "feat(diagnostics): add diagnostics API client namespace"
```

---

## Task 9: Diagnostics tab component

**Files:**
- Create: `web/src/tabs/DiagnosticsTab.tsx`

- [ ] **Step 1: Write the component**

This mirrors LogsTab's streaming machinery (ref buffer flushed every 200ms, autoscroll) and adds the environment card + report button. Uses semantic colour tokens and `dune-ui` wrappers only.

```tsx
import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Switch, toast } from '@heroui/react'
import { api } from '../api/client'
import type { DiagnosticsEnvironment } from '../api/client'
import { Panel, PageHeader, InfoCard } from '../dune-ui'

const MAX_LINES = 5000

export const DiagnosticsTab: React.FC = () => {
  const { t } = useTranslation()
  const [env, setEnv] = React.useState<DiagnosticsEnvironment | null>(null)
  const [lines, setLines] = React.useState<string[]>([])
  const [autoScroll, setAutoScroll] = React.useState(true)
  const [connected, setConnected] = React.useState(false)

  const wsRef = React.useRef<WebSocket | null>(null)
  const bufRef = React.useRef<string[]>([])
  const timerRef = React.useRef<ReturnType<typeof setInterval> | null>(null)
  const preRef = React.useRef<HTMLPreElement | null>(null)

  const loadEnv = React.useCallback(async () => {
    try {
      setEnv(await api.diagnostics.environment())
    } catch (e) {
      toast.danger(`Failed: ${e instanceof Error ? e.message : String(e)}`)
    }
  }, [])

  React.useEffect(() => {
    void loadEnv()
    const ws = new WebSocket(api.diagnostics.streamUrl())
    wsRef.current = ws
    ws.onopen = () => setConnected(true)
    ws.onclose = () => setConnected(false)
    ws.onmessage = (ev) => { bufRef.current.push(ev.data as string) }

    timerRef.current = setInterval(() => {
      if (bufRef.current.length === 0) return
      setLines((prev) => {
        const next = [...prev, ...bufRef.current]
        bufRef.current = []
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
      })
    }, 200)

    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
      ws.close()
    }
  }, [loadEnv])

  React.useEffect(() => {
    if (autoScroll && preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight
    }
  }, [lines, autoScroll])

  const reportIssue = async () => {
    try {
      const rep = await api.diagnostics.report()
      const url = `https://github.com/${rep.repo}/issues/new?title=${encodeURIComponent(rep.title)}&body=${encodeURIComponent(rep.body)}`
      window.open(url, '_blank', 'noopener,noreferrer')
      // Trigger the bundle download for manual attach.
      window.location.assign(api.diagnostics.bundleUrl())
    } catch (e) {
      toast.danger(`Failed: ${e instanceof Error ? e.message : String(e)}`)
    }
  }

  return (
    <Panel>
      <PageHeader title={t('diagnostics.title', 'Diagnostics')} onRefresh={loadEnv} loading={false} />

      {env && (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <InfoCard label={t('diagnostics.version', 'Version')} value={env.version} />
          <InfoCard label={t('diagnostics.controlPlane', 'Control plane')} value={env.control_plane} />
          <InfoCard label="OS / Arch" value={`${env.os}/${env.arch}`} />
          <InfoCard label={t('diagnostics.servers', 'Active servers')} value={String(env.active_server_count)} />
        </div>
      )}

      <div className="flex items-center gap-3 py-2">
        <span className={connected ? 'text-accent' : 'text-muted'}>
          {connected ? t('diagnostics.live', 'Live') : t('diagnostics.disconnected', 'Disconnected')}
        </span>
        <Switch isSelected={autoScroll} onChange={setAutoScroll} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{t('logs.autoScroll', 'Auto-scroll')}</Switch.Content>
        </Switch>
        <Button size="sm" variant="ghost" onPress={() => setLines([])}>
          {t('common.clear', 'Clear')}
        </Button>
        <Button size="sm" color="primary" onPress={() => void reportIssue()}>
          {t('diagnostics.report', 'Report an issue')}
        </Button>
      </div>

      <pre
        ref={preRef}
        className="h-[60vh] overflow-auto rounded border border-border bg-surface p-3 font-mono text-xs text-foreground"
      >
        {lines.join('\n')}
      </pre>
    </Panel>
  )
}
```

- [ ] **Step 2: Verify it builds and lints**

Run: `cd web && pnpm lint`
Expected: no errors. If `InfoCard` props differ from `{ label, value }`, open `web/src/dune-ui/InfoCard.tsx`, match its actual prop names, and adjust. Do not invent props.

- [ ] **Step 3: Commit**

```bash
git add web/src/tabs/DiagnosticsTab.tsx
git commit -m "feat(diagnostics): add Diagnostics tab component"
```

---

## Task 10: Register the tab

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/components/app/nav.ts`
- Modify: `web/src/components/app/AppRoutes.tsx`

- [ ] **Step 1: Extend the TabId union**

In `web/src/types.ts`, add `'diagnostics'` to the `TabId` union (after `'permissions'`):

```ts
    | 'permissions'
    | 'diagnostics'
```

- [ ] **Step 2: Register in nav.ts**

Add `'diagnostics'` to the end of `TAB_IDS`:

```ts
  'permissions',
  'diagnostics',
] as const
```

Add the icon to `TAB_ICONS`:

```ts
  permissions: 'lock',
  diagnostics: 'stethoscope',
```

Add the capability to `TAB_CAPABILITIES` (use `'owner'` so the tab is owner-only in the UI, matching the Permissions tab):

```ts
  permissions: 'owner',
  diagnostics: 'owner',
```

- [ ] **Step 3: Wire the route in AppRoutes.tsx**

Add the lazy import alongside the others:

```ts
const DiagnosticsTab = React.lazy(() => import('../../tabs/DiagnosticsTab').then((m) => ({ default: m.DiagnosticsTab })))
```

Add the render line next to the other `renderTab` calls (e.g. after the `permissions` one):

```tsx
      {renderTab('diagnostics', <DiagnosticsTab />)}
```

- [ ] **Step 4: Verify the full frontend builds**

Run: `cd web && pnpm lint && pnpm build`
Expected: type-checks and builds clean.

- [ ] **Step 5: Commit**

```bash
git add web/src/types.ts web/src/components/app/nav.ts web/src/components/app/AppRoutes.tsx
git commit -m "feat(diagnostics): register Diagnostics tab (owner-only)"
```

---

## Task 11: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Backend verify**

Run: `make verify`
Expected: vet, `go test -race`, govulncheck, golangci-lint, markdownlint, gocognit all pass.

- [ ] **Step 2: Security scan**

Run: `make gosec`
Expected: no high-severity findings. Resolve any with a justified `// #nosec <ID1>,<ID2> -- reason` (both IDs required) only if a genuine false positive.

- [ ] **Step 3: Frontend lint + build**

Run: `cd web && pnpm lint && pnpm build`
Expected: clean.

- [ ] **Step 4: Manual smoke (optional, requires a running backend)**

Run `make dev`, sign in as an owner, open the Diagnostics tab: confirm the env card populates, the log stream shows live lines, and Report an issue opens a prefilled GitHub issue (with no IPs/tokens in the body) and downloads `diagnostics.zip`. Verify a non-owner session does not see the tab and that the endpoints return 403 for a session without `diagnostics:read`.

- [ ] **Step 5: No final commit needed**

All work was committed per task. Stop here for user review before pushing or opening a PR (per the no-commit-without-permission / no-push-without-approval rules).

---

## Notes for the implementer

- The ring receives zerolog's JSON event bytes (the encoder runs before any writer; `ConsoleWriter` only reformats for stderr). So `app.log` and the live stream show JSON lines — that is expected and clean (no ANSI colour codes).
- Redaction applies only to the report body and the bundle. The live WebSocket stream is intentionally raw because it is owner-only — do not add redaction there.
- Keep every function under cognitive complexity 15 (`make gocognit`). If a handler grows, extract a helper.
- Do not thread `context.Background()` into anything request-scoped; the stream handler correctly uses `r.Context()`.
