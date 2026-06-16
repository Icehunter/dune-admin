package main

import (
	"fmt"
	"net/http"
)

// Per-server auto-discovery for the add-server wizard. A new server isn't
// registered until the wizard finishes, so this opens an ephemeral executor
// against the supplied connection settings, reads what it can from the running
// game server (and, for kubectl, the cluster), and returns gap-fill values
// WITHOUT persisting or registering anything.

// assembleServerDiscovery runs best-effort discovery through exec for the given
// control plane and returns a snake_case map of discovered fields. Always
// returns a non-nil map; missing values are simply absent/empty. db_pass is
// included (the wizard submits it back) — callers must redact it from logs.
func assembleServerDiscovery(exec Executor, control string) map[string]any {
	out := map[string]any{
		"db_user": "", "db_pass": "", "db_name": "",
		"control_namespace": "",
		"broker_game_addr":  "", "broker_admin_addr": "", "broker_tls": false,
		"director_url": "",
	}

	g, err := discoverGameConfig(exec)
	if err != nil {
		// No running game server (or unreachable) — return the empty template so
		// the wizard can fall back to manual entry.
		return out
	}
	out["db_user"] = g.DBUser
	out["db_pass"] = g.DBPass
	out["db_name"] = g.DBName

	if control != "kubectl" {
		return out
	}

	// kubectl: resolve cluster-internal endpoints to pod IPs and the namespace.
	if ns, _, _, derr := discoverDBPod(exec); derr == nil {
		out["control_namespace"] = ns
	}
	pods := fetchClusterPodIPs(exec)
	gameIP := podIPByPattern(pods, "mq-game")
	adminIP := podIPByPattern(pods, "mq-admin")
	directorIP := podIPByPattern(pods, "bgd")
	if gameIP != "" && g.RMQGamePort != "" {
		out["broker_game_addr"] = gameIP + ":" + g.RMQGamePort
	}
	if adminIP != "" && g.RMQAdminPort != "" {
		out["broker_admin_addr"] = adminIP + ":" + g.RMQAdminPort
	}
	out["broker_tls"] = g.RMQTLS
	if directorIP != "" {
		if p := portOf(g.DirectorURL); p != "" {
			out["director_url"] = "http://" + directorIP + ":" + p
		}
	}
	return out
}

// discoveryMaskForLog renders a discovery result for logging with db_pass
// redacted.
func discoveryMaskForLog(d map[string]any) string {
	pass, _ := d["db_pass"].(string)
	return fmt.Sprintf("db_user=%v db_name=%v namespace=%v broker_game=%v pass=%s",
		d["db_user"], d["db_name"], d["control_namespace"], d["broker_game_addr"], maskSecret(pass))
}

// handleDiscoverServer runs auto-discovery for a not-yet-created server. It
// builds a throwaway executor from the posted connection settings, discovers
// what it can, and returns the values — nothing is registered or persisted.
//
// @Summary Discover connection values for a new server (no persistence)
// @Tags servers
// @Accept json
// @Produce json
// @Param config body ServerConfig true "Connection settings to probe"
// @Success 200 {object} map[string]any
// @Failure 502 {object} map[string]string
// @Router /api/v1/servers/discover [post]
func handleDiscoverServer(w http.ResponseWriter, r *http.Request) {
	var cfg ServerConfig
	if err := decode(r, &cfg); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	applyServerConfigDefaults(&cfg)

	ctrl := cfg.Control
	if ctrl == "" {
		ctrl = "local"
	}

	exec, err := newExecutor(cfg.SSHHost, cfg.SSHUser, cfg.SSHKey, cfg.SSHMode, cfg.SSHExtraOpts)
	if err != nil {
		jsonErr(w, fmt.Errorf("connect: %w", err), http.StatusBadGateway)
		return
	}
	defer exec.Close()
	if ctrl == "amp" {
		exec = &ampExecutor{Executor: exec, ampUser: cfg.AmpUser}
	}

	out := assembleServerDiscovery(exec, ctrl)
	componentLog("server_discover").Info().
		Str("server_name", cfg.Name).
		Str("control_plane", ctrl).
		Str("discovered", discoveryMaskForLog(out)).
		Msg("server discovery complete")
	jsonOK(w, out)
}
