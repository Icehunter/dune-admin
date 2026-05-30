package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Darwin always ships a universal (fat) binary regardless of host arch.
func artifactName(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "dune-admin_darwin_universal.tar.gz"
	case "windows":
		return fmt.Sprintf("dune-admin_windows_%s.zip", goarch)
	default:
		return fmt.Sprintf("dune-admin_%s_%s.tar.gz", goos, goarch)
	}
}

// parseChecksum parses GoReleaser checksums.txt format: "<hex>  <filename>" (two spaces).
func parseChecksum(content, artifact string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == artifact {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", artifact)
}

const githubRepo = "Icehunter/dune-admin"

// updateFetcher is the real HTTP fetcher used in production.
// It is wired into handlers in server.go; the unused linter fires before those handlers exist.
//
//nolint:unused
func updateFetcher(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "dune-admin/"+AppVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// latestRelease fetches the tag and release page URL for the most recent GitHub release.
// fetcher is injected so callers can substitute a mock in tests.
func latestRelease(fetcher func(string) ([]byte, error)) (tag, htmlURL string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	body, err := fetcher(url)
	if err != nil {
		return "", "", fmt.Errorf("fetch latest release: %w", err)
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", "", fmt.Errorf("parse release JSON: %w", err)
	}
	if rel.TagName == "" {
		return "", "", fmt.Errorf("empty tag_name in response")
	}
	return rel.TagName, rel.HTMLURL, nil
}

// Dev builds ("-dev" suffix or bare "dev") are never auto-updated.
func needsUpdate(current, latest string) bool {
	if strings.Contains(current, "-dev") || current == "dev" {
		return false
	}
	norm := strings.TrimPrefix(latest, "v")
	return strings.TrimPrefix(current, "v") != norm && norm != ""
}
