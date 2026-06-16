package main

import (
	"context"
	"fmt"
	"net/http"
)

// webInterfaceDiscoverer is an optional control-plane capability: derive web
// interface links (director, file browser, …) from live infrastructure state so
// they don't have to be configured by hand. kubectl reads them from the
// battlegroup status; planes that can't discover simply don't implement it.
type webInterfaceDiscoverer interface {
	discoverWebInterfaces(ctx context.Context, exec Executor) []webInterface
}

// discoveredWebInterfaces returns control-plane-derived links, or nil when the
// active plane can't discover them or nothing is connected.
func discoveredWebInterfaces(ctx context.Context, ctrl ControlPlane, exec Executor) []webInterface {
	if ctrl == nil || exec == nil {
		return nil
	}
	d, ok := ctrl.(webInterfaceDiscoverer)
	if !ok {
		return nil
	}
	return d.discoverWebInterfaces(ctx, exec)
}

// directorInterfaceLabel is the label the kubectl control plane assigns the
// discovered Battlegroup Director (see control_kubectl.go). It doubles as the
// dedupe key in withoutDiscoveredDirector.
const directorInterfaceLabel = "Battlegroup Director"

// withoutDiscoveredDirector drops the control-plane-discovered Battlegroup
// Director when director_url is configured, preserving the invariant that the
// Director is shown from director_url (the same-origin /director/ proxy) rather
// than the discovered list — otherwise the card renders it twice. With no
// director_url the list is returned unchanged (no allocation).
func withoutDiscoveredDirector(discovered []webInterface, directorURL string) []webInterface {
	if directorURL == "" {
		return discovered
	}
	out := make([]webInterface, 0, len(discovered))
	for _, wi := range discovered {
		if wi.Label == directorInterfaceLabel {
			continue
		}
		out = append(out, wi)
	}
	return out
}

// @Summary List configured web interfaces
// @Tags web-interfaces
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/web-interfaces [get]
func handleGetWebInterfaces(w http.ResponseWriter, r *http.Request) {
	// interfaces are operator-defined (editable, persisted); discovered are
	// control-plane-derived (read-only) and never written back. Each entry is
	// enriched with its mesh-proxy port (0/omitted when not proxied) so the SPA
	// can open it via dune-admin's own host instead of an unreachable game-side URL.
	ifaces := getWebInterfaces()
	discovered := discoveredWebInterfaces(r.Context(), controlFromCtx(r), executorFromCtx(r))
	// When director_url is set the card shows the Director as its own DirectorRow,
	// so drop the discovered copy to avoid rendering it twice (dedupe by label).
	// Key off the ACTIVE server's director_url — the same source as /status (which
	// the DirectorRow reads) and as the proxy ports (currentTargets) — so the
	// dedupe can't disagree with the rendered DirectorRow during a view-switch
	// where the request server differs from the active one.
	discovered = withoutDiscoveredDirector(discovered, activeServerCfg().DirectorURL)
	// Ports come from the running proxy set for the active server (single source
	// of truth). When the request's server is the active one, the discovered dial
	// addresses match and each entry gets its proxy port; otherwise proxyPort is 0.
	targets := globalWebProxy.currentTargets()
	jsonOK(w, map[string]any{
		"interfaces": withProxyPorts(ifaces, targets),
		"discovered": withProxyPorts(discovered, targets),
	})
}

// @Summary Replace the configured web interfaces
// @Tags web-interfaces
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/web-interfaces [put]
func handleUpdateWebInterfaces(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Interfaces []webInterface `json:"interfaces"`
	}
	if err := decode(r, &body); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := validateWebInterfaces(body.Interfaces); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := saveWebInterfaces(body.Interfaces); err != nil {
		componentLog("web_interfaces").Error().Err(err).Msg("could not save web interfaces")
		jsonErr(w, fmt.Errorf("could not save web interfaces"), http.StatusInternalServerError)
		return
	}
	// A newly saved hand-configured absolute-URL interface needs a proxy port to
	// be reachable; rebuild the active server's proxy set now instead of leaving
	// it stale until the next server switch / restart. Mirrors the rebuild call
	// every other state change makes (boot, switch, delete, reconnect).
	rebuildWebProxiesForActive()
	jsonOK(w, map[string]string{"ok": "web interfaces saved"})
}
