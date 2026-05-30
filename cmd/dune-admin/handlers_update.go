package main

import (
	"fmt"
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

// Dev builds ("-dev" suffix or bare "dev") are never auto-updated.
func needsUpdate(current, latest string) bool {
	if strings.Contains(current, "-dev") || current == "dev" {
		return false
	}
	norm := strings.TrimPrefix(latest, "v")
	return strings.TrimPrefix(current, "v") != norm && norm != ""
}
