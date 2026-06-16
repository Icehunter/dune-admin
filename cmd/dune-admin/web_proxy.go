package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ── Mesh web proxy ───────────────────────────────────────────────────────────
// Game-side HTTP UIs (Battlegroup Director, File Browser) live on node ports the
// operator's browser can't reach over the Nebula mesh. dune-admin already reaches
// them through the executor/SSH tunnel (same path as the DB pool). This serves
// each such service from a dedicated local port via a root reverse proxy, so the
// browser only talks to dune-admin and the services' absolute asset paths
// (/Script/…, /static/…) resolve correctly.

// proxyTarget is a resolved web interface to reverse-proxy: its label, the
// upstream scheme (http/https), the host:port to Dial through the executor, and
// the local listener port.
type proxyTarget struct {
	label    string
	scheme   string // upstream scheme: "http" or "https"
	dialAddr string
	port     int
}

// resolveProxyTargets selects the proxyable web interfaces, sorts them
// deterministically by dial address, and assigns port = listenPort+10+index.
// A listenPort <= 0 yields no targets (the port base can't be derived).
//
// Proxyable = an entry dune-admin can Dial: a discovered entry (its Target, the
// raw CRD host:port, always plain HTTP) or a hand-configured absolute http(s)
// URL (whose scheme is preserved so HTTPS upstreams get a TLS dial). Same-origin
// "/path" entries are skipped — they are already reachable as-is.
func resolveProxyTargets(ifaces []webInterface, listenPort int) []proxyTarget {
	if listenPort <= 0 {
		return nil
	}
	var targets []proxyTarget
	for _, w := range ifaces {
		scheme, dial := schemeAndDialFor(w)
		if dial == "" {
			continue
		}
		targets = append(targets, proxyTarget{label: w.Label, scheme: scheme, dialAddr: dial})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].dialAddr < targets[j].dialAddr })
	for i := range targets {
		targets[i].port = listenPort + 10 + i
	}
	return targets
}

// schemeAndDialFor returns the upstream scheme and host:port dune-admin can Dial
// for a web interface: a discovered entry uses its Target (raw CRD host:port,
// always plain HTTP), otherwise the absolute http(s) URL is parsed. Returns
// ("", "") when the entry is not proxyable (same-origin/relative URL). Single
// source of truth so resolveProxyTargets and withProxyPorts can't drift.
func schemeAndDialFor(w webInterface) (scheme, dial string) {
	if w.Target != "" {
		return "http", w.Target
	}
	return schemeAndDialFromURL(w.URL)
}

// schemeAndDialFromURL returns (scheme, host:port) for an absolute http(s) URL,
// filling in the default port from the scheme. Returns ("", "") for relative or
// non-http(s) URLs.
func schemeAndDialFromURL(raw string) (string, string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", ""
	}
	// Only root URLs are proxyable. The root reverse proxy forwards the request
	// path verbatim and builds the upstream from scheme+host only, so a non-root
	// base path / query / fragment would be silently dropped (https://host/foo →
	// https://host/, opening the wrong page). Leave those unproxied (opened as-is).
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", ""
	}
	if u.Port() != "" {
		return u.Scheme, u.Host
	}
	if u.Scheme == "https" {
		return u.Scheme, net.JoinHostPort(u.Hostname(), "443")
	}
	return u.Scheme, net.JoinHostPort(u.Hostname(), "80")
}

// proxyListenScheme is the scheme the browser uses to reach a local proxy port.
// The proxy listeners are always plain HTTP (dune-admin terminates no TLS itself;
// any TLS is terminated by an external reverse proxy that does not forward these
// ports). So the SPA must build the open-URL from this scheme, not from
// window.location.protocol — otherwise an HTTPS-served dashboard would try
// https://host:proxyPort and fail.
const proxyListenScheme = "http"

// webInterfaceOut is a web interface enriched with its assigned proxy port for
// the API. ProxyPort is 0 (omitted) for entries that are not proxied; ProxyScheme
// is the scheme the browser must use to reach that port (only set when proxied).
type webInterfaceOut struct {
	Label       string `json:"label"`
	URL         string `json:"url"`
	ProxyPort   int    `json:"proxyPort,omitempty"`
	ProxyScheme string `json:"proxyScheme,omitempty"`
}

// withProxyPorts attaches each entry's proxy port + scheme (matched by dial
// address) so the frontend can open <proxyScheme>://<window.location.hostname>:<proxyPort>/
// instead of the unreachable rewritten URL.
func withProxyPorts(ifaces []webInterface, targets []proxyTarget) []webInterfaceOut {
	portByAddr := make(map[string]int, len(targets))
	for _, t := range targets {
		portByAddr[t.dialAddr] = t.port
	}
	out := make([]webInterfaceOut, 0, len(ifaces))
	for _, w := range ifaces {
		_, dial := schemeAndDialFor(w)
		entry := webInterfaceOut{Label: w.Label, URL: w.URL, ProxyPort: portByAddr[dial]}
		if entry.ProxyPort != 0 {
			entry.ProxyScheme = proxyListenScheme
		}
		out = append(out, entry)
	}
	return out
}

// listenPortNum returns the numeric port from listenAddr (e.g. ":8080" → 8080),
// or 0 when it can't be parsed.
func listenPortNum() int {
	_, p, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

// listenHost returns the host/interface from listenAddr (e.g. "127.0.0.1:8080" →
// "127.0.0.1", ":8080" → ""). The proxy ports bind to this same interface so
// their exposure matches the main server's: a localhost-bound UI (behind a
// reverse proxy) does not accidentally expose the proxy ports on all interfaces.
func listenHost() string {
	h, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}
	return h
}

// newRootProxy reverse-proxies the entire root path to target, tunneling upstream
// connections through dial. Unlike newDirectorProxy it does NOT strip a prefix:
// the proxied app owns the whole port, so its absolute asset paths resolve.
func newRootProxy(target *url.URL, transport http.RoundTripper) http.HandlerFunc {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	return func(w http.ResponseWriter, r *http.Request) {
		r.Host = target.Host
		proxy.ServeHTTP(w, r)
	}
}

// withProxyAuth enforces the same dashboard session as the main UI when auth is
// enabled. Session cookies are host-scoped (not port-scoped), so on a plain-HTTP
// deployment the login on the main port is presented to the proxy ports too.
//
// Known limitation: behind a TLS-terminating reverse proxy (X-Forwarded-Proto:
// https) the session cookie is set Secure, and browsers will not send a Secure
// cookie to the plain-HTTP proxy ports — so authenticateRequest sees no session
// and every proxied interface returns 401. The proxy ports terminate no TLS
// themselves; supporting auth behind TLS termination needs a non-cookie scheme
// (e.g. a per-target token) and is intentionally out of scope for this change.
func withProxyAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authEnabled(loadedConfig) {
			if _, ok := authenticateRequest(w, r); !ok {
				return // authenticateRequest already wrote 401
			}
		}
		next(w, r)
	}
}

// startWebProxies launches one root-proxy http.Server per target and returns the
// subset of targets whose listener actually bound, plus a func that shuts them
// all down. Each listener is bound synchronously (so the port is open before this
// returns); a bind failure is logged and that single proxy skipped — never fatal.
// The returned subset is what the manager stores, so the SPA is never told a
// proxyPort for a listener that failed to start. dial is injected for testability.
func startWebProxies(targets []proxyTarget, dial func(network, addr string) (net.Conn, error)) ([]proxyTarget, func()) {
	host := listenHost()
	var servers []*http.Server
	var started []proxyTarget
	for _, t := range targets {
		scheme := t.scheme
		if scheme == "" {
			scheme = "http"
		}
		upstream := &url.URL{Scheme: scheme, Host: t.dialAddr}
		mux := http.NewServeMux()
		mux.HandleFunc("/", withProxyAuth(newRootProxy(upstream, httpTransportVia(dial))))
		addr := net.JoinHostPort(host, strconv.Itoa(t.port))
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			log.Printf("web-proxy: %s on %s: %v (skipped)", t.label, addr, err)
			continue
		}
		log.Printf("web-proxy: %s → %s (upstream %s://%s)", t.label, addr, scheme, t.dialAddr)
		servers = append(servers, srv)
		started = append(started, t)
		go func() { _ = srv.Serve(ln) }()
	}
	return started, func() {
		// Close (not graceful Shutdown): on a server switch the old server is no
		// longer active, so its proxy connections should drop at once. Shutdown
		// would block on in-flight responses (e.g. a File Browser download) for up
		// to its timeout — and apply() calls this under the manager lock, which
		// would stall currentTargets() for that whole window.
		for _, s := range servers {
			_ = s.Close()
		}
	}
}

// dialViaExecutor binds the proxy dial path to a specific server's executor. A
// nil executor dials directly; a set executor tunnels every connection through
// it (the same path as that server's DB pool), so the proxy reaches hosts
// reachable from wherever that server runs rather than from this machine.
func dialViaExecutor(exec Executor) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		if exec != nil {
			return exec.Dial(network, addr)
		}
		return net.Dial(network, addr)
	}
}

// webProxyManager owns the proxy set for the currently active server. The set is
// rebuilt whenever the active server changes (boot, switch, removal) so the
// proxies always tunnel through the active server's executor. It is the single
// source of truth for the assigned ports the API reports to the SPA.
type webProxyManager struct {
	mu      sync.Mutex
	stop    func()
	targets []proxyTarget
}

// globalWebProxy is the process-wide proxy manager for the active server.
var globalWebProxy = &webProxyManager{}

// apply tears down the current proxy set and starts a fresh one for targets,
// tunneling through dial. An empty/nil targets list leaves the manager stopped
// (e.g. no active server). dial is injected for testability; production binds
// the active server's executor via dialViaExecutor.
func (m *webProxyManager) apply(targets []proxyTarget, dial func(network, addr string) (net.Conn, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil {
		m.stop()
		m.stop = nil
	}
	if len(targets) == 0 {
		m.targets = nil
		return
	}
	// Store only the targets whose listener actually bound, so currentTargets /
	// withProxyPorts never advertise a proxyPort for a dead listener. If none
	// bound, stay in the stopped state (keeping the invariant targets==nil iff
	// stop==nil that the empty-targets branch above establishes).
	started, stop := startWebProxies(targets, dial)
	if len(started) == 0 {
		stop()
		m.targets, m.stop = nil, nil
		return
	}
	m.targets, m.stop = started, stop
}

// currentTargets returns a copy of the proxy targets for the active server, or
// nil when no proxies are running. A copy (not the internal slice) so callers
// can't mutate or race the manager's state.
func (m *webProxyManager) currentTargets() []proxyTarget {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.targets == nil {
		return nil
	}
	return append([]proxyTarget(nil), m.targets...)
}

// shutdown stops all proxies and clears the targets (process teardown / no
// active server).
func (m *webProxyManager) shutdown() { m.apply(nil, nil) }

// rebuildWebProxiesForActive rebuilds the proxy set for the registry's active
// server, tunneling through that server's executor. With no active server it
// tears the proxies down. Safe to call repeatedly (boot, server switch, removal).
func rebuildWebProxiesForActive() {
	active := globalRegistry.Active()
	if active == nil {
		globalWebProxy.shutdown()
		return
	}
	port := listenPortNum()
	if port <= 0 {
		// Can't derive a port base (listenAddr has no parseable port) — disable
		// the web proxy rather than binding bogus low/privileged ports.
		log.Printf("web-proxy: disabled — listen address %q has no usable port", listenAddr)
		globalWebProxy.shutdown()
		return
	}
	ifaces := append(append([]webInterface{}, getWebInterfaces()...),
		discoveredWebInterfaces(context.Background(), active.Control, active.Executor)...)
	targets := resolveProxyTargets(ifaces, port)
	globalWebProxy.apply(targets, dialViaExecutor(active.Executor))
}
