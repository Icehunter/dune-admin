package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ── Web interfaces (#155) ────────────────────────────────────────────────────
// A configurable, deployment-agnostic list of operator-defined web links (AMP
// panel, file browser, etc.) the Server Health "Web Interfaces" card renders.
// Persisted as a small JSON file in configDir, mirroring the scheduled-restarts
// store. The Battlegroup Director is NOT stored here — the card shows it
// automatically from director_url (via the /director/ proxy) when configured.

type webInterface struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	// NoProxy opts this entry out of the mesh web proxy: the SPA opens URL
	// as-is instead of a rewritten proxy port. For operators behind NAT/a
	// reverse proxy where only fixed published ports are reachable, the
	// proxy's rewritten URL is unreachable — this restores the pre-v0.42.0
	// "just open the configured URL" behaviour on a per-entry basis. (#261)
	NoProxy bool `json:"noProxy,omitempty"`
	// Target is the raw host:port dune-admin can Dial through the executor to
	// reach the service (the address before any host rewrite). Set for
	// control-plane-discovered entries; empty for hand-configured links. Not
	// persisted — recomputed on discovery. Enables the mesh web proxy.
	Target string `json:"-"`
}

const (
	maxWebInterfaces  = 20
	maxWebIfaceLabel  = 64
	maxWebIfaceURLLen = 512
)

var (
	webIfaceMu     sync.RWMutex
	webIfaces      []webInterface
	webIfaceLoaded bool
	webIfacePath   string // overridable in tests
)

func webInterfacesPath() string {
	if webIfacePath != "" {
		return webIfacePath
	}
	return filepath.Join(configDir(), "web-interfaces.json")
}

func loadWebInterfaces() {
	webIfaceMu.Lock()
	defer webIfaceMu.Unlock()
	webIfaceLoaded = true
	data, err := os.ReadFile(webInterfacesPath())
	if err != nil {
		webIfaces = []webInterface{} // no file yet → empty (the Director shows automatically)
		return
	}
	var list []webInterface
	if err := json.Unmarshal(data, &list); err != nil {
		componentLog("web_interfaces").Error().Err(err).Msg("config parse failed")
		webIfaces = []webInterface{}
		return
	}
	webIfaces = list
}

func getWebInterfaces() []webInterface {
	webIfaceMu.RLock()
	if !webIfaceLoaded {
		webIfaceMu.RUnlock()
		loadWebInterfaces()
		webIfaceMu.RLock()
	}
	defer webIfaceMu.RUnlock()
	out := make([]webInterface, len(webIfaces))
	copy(out, webIfaces)
	return out
}

// validateWebInterfaces enforces non-empty fields, a safe URL scheme (only
// http(s):// or a same-origin "/path", so a "javascript:" URL can't be smuggled
// into a link target), and sane caps.
func validateWebInterfaces(list []webInterface) error {
	if len(list) > maxWebInterfaces {
		return fmt.Errorf("too many web interfaces (max %d)", maxWebInterfaces)
	}
	for _, w := range list {
		label := strings.TrimSpace(w.Label)
		url := strings.TrimSpace(w.URL)
		if label == "" || url == "" {
			return fmt.Errorf("each web interface needs a label and a URL")
		}
		if len(label) > maxWebIfaceLabel || len(url) > maxWebIfaceURLLen {
			return fmt.Errorf("web interface label or URL too long")
		}
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "/") {
			return fmt.Errorf("web interface URL %q must start with http://, https:// or /", url)
		}
	}
	return nil
}

func saveWebInterfaces(list []webInterface) error {
	if err := validateWebInterfaces(list); err != nil {
		return err
	}
	clean := make([]webInterface, len(list))
	for i, w := range list {
		clean[i] = webInterface{Label: strings.TrimSpace(w.Label), URL: strings.TrimSpace(w.URL), NoProxy: w.NoProxy}
	}
	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(webInterfacesPath(), data, 0o600); err != nil {
		return err
	}
	webIfaceMu.Lock()
	webIfaces = clean
	webIfaceLoaded = true
	webIfaceMu.Unlock()
	return nil
}
