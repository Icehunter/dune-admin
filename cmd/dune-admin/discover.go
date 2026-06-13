package main

import (
	"fmt"
	"regexp"
	"strings"
)

// gameServerArgs holds the connection-relevant values parsed from a running
// DuneSandboxServer command line.
type gameServerArgs struct {
	DBUser, DBPass, DBName     string
	RMQGameHost, RMQGamePort   string
	RMQAdminHost, RMQAdminPort string
	RMQTLS                     bool
	DirectorURL                string
}

// argValue extracts the value of `flag=value` or `flag value` from a command
// line. Flags use one or two leading dashes. Returns "" when absent.
func argValue(cmdline, flag string) string {
	re := regexp.MustCompile(`(?:^|\s)-{1,2}` + regexp.QuoteMeta(flag) + `[= ]([^\s]+)`)
	m := re.FindStringSubmatch(cmdline)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// parseGameServerArgs extracts DB/RMQ/Director values from a command line.
// Pure — no I/O.
func parseGameServerArgs(cmdline string) gameServerArgs {
	return gameServerArgs{
		DBUser:       argValue(cmdline, "DatabaseUser"),
		DBPass:       argValue(cmdline, "DatabasePassword"),
		DBName:       argValue(cmdline, "DatabaseName"),
		RMQGameHost:  argValue(cmdline, "RMQGameHostname"),
		RMQGamePort:  argValue(cmdline, "RMQGamePort"),
		RMQAdminHost: argValue(cmdline, "RMQAdminHostname"),
		RMQAdminPort: argValue(cmdline, "RMQAdminPort"),
		RMQTLS:       strings.EqualFold(argValue(cmdline, "RMQGameTlsEnabled"), "true"),
		DirectorURL:  argValue(cmdline, "battlegroup-director-url"),
	}
}

// fillIfEmpty sets *dst to src only when *dst is empty (gap-fill precedence:
// explicit config always wins).
func fillIfEmpty(dst *string, src string) {
	if *dst == "" && src != "" {
		*dst = src
	}
}

// applyDiscovered fills empty cfg fields from discovered game-server args.
// Host fields (DB/RMQ/Director hosts) are resolved separately — see
// resolveServicePodIP — because the raw values are cluster-internal DNS.
func applyDiscovered(cfg *appConfig, g gameServerArgs) {
	fillIfEmpty(&cfg.DBUser, g.DBUser)
	fillIfEmpty(&cfg.DBPass, g.DBPass)
	fillIfEmpty(&cfg.DBName, g.DBName)
}

// fetchGameServerCmdline returns the command line of a running DuneSandboxServer
// process carrying the connection args, read through the executor. The pgrep
// pattern uses the `[D]uneSandboxServer` bracket trick so it does not match its
// own shell-wrapper process (whose command line contains the literal pattern),
// which would otherwise return the pipeline itself instead of the game server.
func fetchGameServerCmdline(exec Executor) (string, error) {
	out, err := exec.Exec("pgrep -af '[D]uneSandboxServer' 2>/dev/null | grep -- '-DatabasePassword=' | head -1")
	if err != nil {
		return "", fmt.Errorf("pgrep game server: %w", err)
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "", fmt.Errorf("no running DuneSandboxServer process found")
	}
	return line, nil
}

// discoverGameConfig fetches the game-server command line and parses it.
func discoverGameConfig(exec Executor) (gameServerArgs, error) {
	cmdline, err := fetchGameServerCmdline(exec)
	if err != nil {
		return gameServerArgs{}, err
	}
	return parseGameServerArgs(cmdline), nil
}

// maskSecret renders a secret as a short non-revealing token for logs.
func maskSecret(s string) string {
	if s == "" {
		return "(empty)"
	}
	return fmt.Sprintf("set, %d chars", len(s))
}

// portOf returns the ":port" suffix of a host:port string, or "" if none.
func portOf(hostPort string) string {
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		return hostPort[i+1:]
	}
	return ""
}

// applyDiscoveredEndpoints fills empty broker/director fields using resolved
// pod IPs plus the ports/TLS parsed from the args. Empty resolved IPs are
// skipped (no usable endpoint). DirectorURL's port comes from the discovered
// director host:port in the args.
func applyDiscoveredEndpoints(cfg *appConfig, g gameServerArgs, gameIP, adminIP, directorIP string) {
	if gameIP != "" && g.RMQGamePort != "" {
		fillIfEmpty(&cfg.BrokerGameAddr, gameIP+":"+g.RMQGamePort)
	}
	if adminIP != "" && g.RMQAdminPort != "" {
		fillIfEmpty(&cfg.BrokerAdminAddr, adminIP+":"+g.RMQAdminPort)
	}
	if g.RMQTLS && !cfg.BrokerTLS {
		cfg.BrokerTLS = true
	}
	if directorIP != "" {
		if p := portOf(g.DirectorURL); p != "" {
			fillIfEmpty(&cfg.DirectorURL, "http://"+directorIP+":"+p)
		}
	}
}

// persistDiscoveredConfig returns a copy of cfg with discovered DB + endpoint
// gaps filled. Pure — the caller persists/applies the result.
func persistDiscoveredConfig(cfg appConfig, g gameServerArgs, gameIP, adminIP, directorIP string) appConfig {
	applyDiscovered(&cfg, g)
	applyDiscoveredEndpoints(&cfg, g, gameIP, adminIP, directorIP)
	return cfg
}

// resolveServicePodIP returns the pod IP of the first pod whose name matches
// grepPattern, via kubectl through the executor (like discoverDBPod). Returns
// "" (no error) when not found, so callers can skip that endpoint.
func resolveServicePodIP(exec Executor, grepPattern string) string {
	kctl := kubectlCLI(exec)
	out, err := exec.Exec(fmt.Sprintf( // #nosec G204,G702 -- grepPattern is a constant service substring, not user input
		`%s get pods -A -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.podIP}{"\n"}{end}' 2>/dev/null | grep %s | head -1`,
		kctl, grepPattern))
	if err != nil {
		return ""
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
