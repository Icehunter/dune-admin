package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// resolveProxyTargets picks proxyable entries, sorts them deterministically by
// dial address, and assigns ports = listenPort+10+index. Sorting by dialAddr
// means File Browser (…:18888) sorts before Director (…:31003).
func TestResolveProxyTargets(t *testing.T) {
	ifaces := []webInterface{
		{Label: "File Browser", URL: "http://vm:18888/", Target: "10.0.0.5:18888"},
		{Label: "Battlegroup Director", URL: "http://vm:31003/", Target: "10.0.0.5:31003"},
		{Label: "Wiki", URL: "https://wiki.example/"},                  // manual absolute root → proxied, port from scheme
		{Label: "Docs", URL: "https://docs.example/handbook"},          // non-root path → NOT proxied (would be dropped)
		{Label: "Search", URL: "https://s.example/?q=x"},               // query → NOT proxied
		{Label: "Local", URL: "/grafana"},                              // same-origin → NOT proxied
		{Label: "Panel", URL: "https://panel.example/", NoProxy: true}, // opted out → NOT proxied despite being an otherwise-proxyable root URL
	}
	got := resolveProxyTargets(ifaces, 8080)
	want := []proxyTarget{
		{label: "File Browser", scheme: "http", dialAddr: "10.0.0.5:18888", port: 8090},
		{label: "Battlegroup Director", scheme: "http", dialAddr: "10.0.0.5:31003", port: 8091},
		{label: "Wiki", scheme: "https", dialAddr: "wiki.example:443", port: 8092}, // https preserved
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// resolveProxyTargets must skip a NoProxy entry even when it's a discovered
// entry with a Target set (defensive — the UI can only set NoProxy on
// hand-configured entries today, but the guard must not be bypassable by Target).
func TestResolveProxyTargets_NoProxySkipsDiscoveredTarget(t *testing.T) {
	ifaces := []webInterface{
		{Label: "Director", Target: "10.0.0.5:31003", NoProxy: true},
	}
	if got := resolveProxyTargets(ifaces, 8080); got != nil {
		t.Errorf("NoProxy with Target set: got %+v, want nil", got)
	}
}

// resolveProxyTargets yields no targets when the listen port can't be derived
// (port <= 0) — proxying with a bogus low/privileged port base is worse than off.
func TestResolveProxyTargets_NoPortBase(t *testing.T) {
	ifaces := []webInterface{{Label: "Director", Target: "10.0.0.5:31003"}}
	if got := resolveProxyTargets(ifaces, 0); got != nil {
		t.Errorf("listenPort 0: got %+v, want nil", got)
	}
	if got := resolveProxyTargets(ifaces, -1); got != nil {
		t.Errorf("listenPort -1: got %+v, want nil", got)
	}
}

// withProxyPorts attaches each entry's assigned proxy port (0 when not proxied)
// so the frontend can build the open-URL from window.location + the port.
func TestWithProxyPorts(t *testing.T) {
	ifaces := []webInterface{
		{Label: "Battlegroup Director", URL: "http://vm:31003/", Target: "10.0.0.5:31003"},
		{Label: "Local", URL: "/grafana"}, // same-origin → not proxied
	}
	targets := resolveProxyTargets(ifaces, 8080) // Director → 8090
	got := withProxyPorts(ifaces, targets)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Label != "Battlegroup Director" || got[0].ProxyPort != 8090 || got[0].ProxyScheme != "http" {
		t.Errorf("director = %+v, want proxyPort 8090 + proxyScheme http", got[0])
	}
	if got[1].ProxyPort != 0 || got[1].ProxyScheme != "" {
		t.Errorf("local = %+v, want proxyPort 0 + empty scheme (not proxied)", got[1])
	}
}

// withProxyPorts must attach no port/scheme for a NoProxy entry — even one
// with an otherwise-proxyable absolute root URL — and must echo NoProxy back
// so the frontend edit form reflects the persisted flag. (#261)
func TestWithProxyPorts_NoProxyEntryGetsNoPort(t *testing.T) {
	ifaces := []webInterface{
		{Label: "Panel", URL: "https://panel.example/", NoProxy: true},
	}
	targets := resolveProxyTargets(ifaces, 8080)
	got := withProxyPorts(ifaces, targets)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].ProxyPort != 0 || got[0].ProxyScheme != "" {
		t.Errorf("panel = %+v, want proxyPort 0 + empty scheme (opted out)", got[0])
	}
	if !got[0].NoProxy {
		t.Errorf("panel = %+v, want NoProxy echoed back true", got[0])
	}
}

// newRootProxy must forward the whole path unchanged so absolute asset paths
// (/Script/…, /static/…) resolve at the proxied service's root.
func TestNewRootProxy_PassesAbsoluteAssetPaths(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "path=%s", r.URL.Path)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	h := newRootProxy(u, httpTransportVia(net.Dial))

	for _, p := range []string{"/", "/Script/app.js", "/static/css/app.css"} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "path="+p {
			t.Errorf("%s → %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

// newRootProxy must speak HTTPS upstream for an https target (not plain HTTP to
// :443), so hand-configured https interfaces proxy correctly.
func TestNewRootProxy_HTTPSUpstream(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "tls-path=%s", r.URL.Path)
	}))
	defer upstream.Close()

	// Inject a transport that trusts the test server's cert — no global state
	// mutation, no InsecureSkipVerify.
	pool := x509.NewCertPool()
	pool.AddCert(upstream.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}

	u, _ := url.Parse(upstream.URL) // https://127.0.0.1:<port>
	h := newRootProxy(u, transport)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/Script/app.js", nil))
	// Success proves TLS was spoken upstream — a plain-HTTP dial to the TLS port
	// would fail the handshake and yield 502.
	if rec.Code != http.StatusOK || rec.Body.String() != "tls-path=/Script/app.js" {
		t.Errorf("https upstream → %d %q, want 200 tls-path=/Script/app.js", rec.Code, rec.Body.String())
	}
}

// listenHost extracts the bind interface from listenAddr so proxy ports inherit
// the main server's exposure (localhost-bound UI → localhost-bound proxies).
func TestListenHost(t *testing.T) {
	prev := listenAddr
	t.Cleanup(func() { listenAddr = prev })
	for addr, want := range map[string]string{
		"127.0.0.1:8080": "127.0.0.1",
		":8080":          "",
		"0.0.0.0:8080":   "0.0.0.0",
		"bogus":          "", // unparseable → empty
	} {
		listenAddr = addr
		if got := listenHost(); got != want {
			t.Errorf("listenHost(%q) = %q, want %q", addr, got, want)
		}
	}
}

// startWebProxies binds to the listenAddr host: a localhost-bound main server
// must not expose the proxy ports on other interfaces.
func TestStartWebProxies_BindsToListenHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	uu, _ := url.Parse(upstream.URL)

	prevCfg := loadedConfig
	prevAddr := listenAddr
	t.Cleanup(func() { loadedConfig = prevCfg; listenAddr = prevAddr })
	loadedConfig = appConfig{} // auth off
	listenAddr = "127.0.0.1:18080"

	port := freePort(t)
	_, stop := startWebProxies([]proxyTarget{{label: "x", scheme: "http", dialAddr: uu.Host, port: port}}, net.Dial)
	defer stop()

	// reachable on the bound loopback interface
	if body := httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", port)); body != "ok" {
		t.Fatalf("proxy on 127.0.0.1 body = %q, want ok", body)
	}
}

// withProxyAuth enforces the dashboard session only when auth is enabled.
func TestWithProxyAuth(t *testing.T) {
	prev := loadedConfig
	t.Cleanup(func() { loadedConfig = prev })

	called := false
	gated := withProxyAuth(func(http.ResponseWriter, *http.Request) { called = true })

	// auth off → passes through
	loadedConfig = appConfig{}
	rec := httptest.NewRecorder()
	gated(rec, httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Errorf("auth off: inner handler should have been called")
	}

	// auth on, no session cookie → blocked with 401
	called = false
	enabled := true
	loadedConfig = appConfig{AuthEnabled: &enabled}
	rec = httptest.NewRecorder()
	gated(rec, httptest.NewRequest("GET", "/", nil))
	if called {
		t.Errorf("auth on: inner handler should have been blocked")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("auth on: code = %d, want 401", rec.Code)
	}
}

// startWebProxies binds a listener per target synchronously, serves it, and the
// returned stop func shuts them down.
func TestStartWebProxies_StartStop(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	uu, _ := url.Parse(upstream.URL)

	prev := loadedConfig
	t.Cleanup(func() { loadedConfig = prev })
	loadedConfig = appConfig{} // auth off

	port := freePort(t)
	tgt := proxyTarget{label: "x", dialAddr: uu.Host, port: port}
	_, stop := startWebProxies([]proxyTarget{tgt}, net.Dial)

	base := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if body := httpGet(t, base); body != "ok" {
		t.Fatalf("proxy body = %q, want ok", body)
	}

	stop()
	// listener closed → a fresh request must fail
	client := &http.Client{Timeout: time.Second}
	if _, err := client.Get(base); err == nil {
		t.Errorf("expected error after stop, got none")
	}
}

// startWebProxies returns only the targets whose listener actually bound. A
// target whose port is already taken is skipped (logged), so the manager never
// stores — and the SPA is never told — a proxyPort for a dead listener.
// (Icehunter review point 2.)
func TestStartWebProxies_ReturnsOnlyStarted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	uu, _ := url.Parse(upstream.URL)

	prevCfg := loadedConfig
	prevAddr := listenAddr
	t.Cleanup(func() { loadedConfig = prevCfg; listenAddr = prevAddr })
	loadedConfig = appConfig{} // auth off
	listenAddr = "127.0.0.1:18083"

	// Occupy a port on the bind host so the "bad" target's listener fails.
	occupied := freePort(t)
	busy, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(occupied)))
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer func() { _ = busy.Close() }()

	good := freePort(t)
	targets := []proxyTarget{
		{label: "good", scheme: "http", dialAddr: uu.Host, port: good},
		{label: "bad", scheme: "http", dialAddr: uu.Host, port: occupied},
	}
	started, stop := startWebProxies(targets, net.Dial)
	defer stop()

	if len(started) != 1 || started[0].label != "good" {
		t.Fatalf("started = %+v, want only the 'good' target", started)
	}
	if body := httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", good)); body != "ok" {
		t.Fatalf("good proxy body = %q, want ok", body)
	}
}

// dialViaExecutor binds the dial path to a specific server's executor: a nil
// executor dials the requested address directly, a set executor tunnels through
// it (so the proxy reaches hosts reachable from wherever that server runs).
func TestDialViaExecutor(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()
	bu, _ := url.Parse(backend.URL)

	// nil executor → direct net.Dial to the requested addr.
	conn, err := dialViaExecutor(nil)("tcp", bu.Host)
	if err != nil {
		t.Fatalf("nil exec dial: %v", err)
	}
	_ = conn.Close()

	// set executor → routed through it; the requested addr is recorded but the
	// executor connects to its own target instead.
	rec := &dialRecordingExecutor{target: bu.Host}
	conn, err = dialViaExecutor(rec)("tcp", "unreachable.invalid:9999")
	if err != nil {
		t.Fatalf("exec dial: %v", err)
	}
	_ = conn.Close()
	if rec.dialAddr != "unreachable.invalid:9999" {
		t.Errorf("recorded dialAddr = %q, want unreachable.invalid:9999", rec.dialAddr)
	}
}

// webProxyManager owns the active server's proxy set: apply starts it,
// currentTargets reports it, a re-apply swaps it (old port stops serving), and
// apply(nil) / shutdown tears it down. This is what a server switch relies on.
func TestWebProxyManager_ApplyRebuildShutdown(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	uu, _ := url.Parse(upstream.URL)

	prev := loadedConfig
	t.Cleanup(func() { loadedConfig = prev })
	loadedConfig = appConfig{} // auth off

	m := &webProxyManager{}
	client := &http.Client{Timeout: time.Second}

	// apply → serving on the assigned port, targets reported.
	p1 := freePort(t)
	m.apply([]proxyTarget{{label: "a", dialAddr: uu.Host, port: p1}}, net.Dial)
	if got := m.currentTargets(); len(got) != 1 || got[0].port != p1 {
		t.Fatalf("currentTargets after apply = %+v", got)
	}
	if body := httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", p1)); body != "ok" {
		t.Fatalf("proxy body = %q, want ok", body)
	}

	// re-apply on a new port → old listener must close (server switch).
	p2 := freePort(t)
	m.apply([]proxyTarget{{label: "b", dialAddr: uu.Host, port: p2}}, net.Dial)
	if _, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", p1)); err == nil {
		t.Errorf("old port %d still served after rebuild", p1)
	}
	if body := httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", p2)); body != "ok" {
		t.Fatalf("new proxy body = %q, want ok", body)
	}

	// shutdown → fully stopped, no targets (no active server).
	m.shutdown()
	if got := m.currentTargets(); got != nil {
		t.Errorf("currentTargets after shutdown = %+v, want nil", got)
	}
	if _, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", p2)); err == nil {
		t.Errorf("port %d still served after shutdown", p2)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// httpGet relies on startWebProxies having bound the listener before returning:
// the connection queues in the listen backlog and is served once Serve accepts,
// so no readiness sleep is needed.
func httpGet(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
