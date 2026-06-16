package main

import (
	"archive/zip"
	"fmt"
	"io"
	"runtime"
	"strings"
)

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

// buildReport returns a GitHub issue title and a redacted markdown body
// (environment summary + the most recent log lines that fit within maxBytes).
// The newest lines are preferred; if older lines are dropped a truncation
// marker points the reader at the attached bundle.
func buildReport(lines []ringLine, env environmentSummary, maxBytes int) (title, body string) {
	title = fmt.Sprintf("[bug] dune-admin %s", env.Version)

	header := "## Environment\n\n" + environmentMarkdown(env) + "\n## Recent logs\n\n```\n"
	const fence = "```\n"
	const marker = "... (truncated, see attached bundle)\n"

	// Reserve room for the closing fence and a possible truncation marker so
	// the assembled body never exceeds maxBytes.
	budget := maxBytes - len(header) - len(fence) - len(marker)
	if budget < 0 {
		budget = 0
	}

	// Walk newest-to-oldest, keeping redacted lines that fit the budget.
	var chosen []string
	used := 0
	truncated := false
	for i := len(lines) - 1; i >= 0; i-- {
		ln := redactLine(lines[i].Line) + "\n"
		if used+len(ln) > budget {
			truncated = true
			break
		}
		chosen = append(chosen, ln)
		used += len(ln)
	}
	// chosen is newest-first; reverse to chronological order.
	for l, r := 0, len(chosen)-1; l < r; l, r = l+1, r-1 {
		chosen[l], chosen[r] = chosen[r], chosen[l]
	}

	var b strings.Builder
	b.WriteString(header)
	if truncated {
		b.WriteString(marker)
	}
	for _, ln := range chosen {
		b.WriteString(ln)
	}
	b.WriteString(fence)
	return title, b.String()
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
