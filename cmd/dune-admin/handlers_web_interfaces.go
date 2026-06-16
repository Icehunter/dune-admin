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
	jsonOK(w, map[string]string{"ok": "web interfaces saved"})
}
