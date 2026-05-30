package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
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

type updateCheckResponse struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
	ReleaseURL  string `json:"release_url,omitempty"`
}

// makeUpdateCheckHandler returns a handler wired to the given fetcher.
// Pass updateFetcher in production; inject a stub in tests.
func makeUpdateCheckHandler(fetcher func(string) ([]byte, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag, htmlURL, err := latestRelease(fetcher)
		if err != nil {
			log.Printf("handleUpdateCheck: %v", err)
			jsonErr(w, fmt.Errorf("could not reach GitHub"), http.StatusBadGateway)
			return
		}
		jsonOK(w, updateCheckResponse{
			Current:     AppVersion,
			Latest:      tag,
			NeedsUpdate: needsUpdate(AppVersion, tag),
			ReleaseURL:  htmlURL,
		})
	}
}

// handleUpdateCheck is the production handler wired into the HTTP mux in server.go.
func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	makeUpdateCheckHandler(updateFetcher)(w, r)
}

// Dev builds ("-dev" suffix or bare "dev") are never auto-updated.
func needsUpdate(current, latest string) bool {
	if strings.Contains(current, "-dev") || current == "dev" {
		return false
	}
	norm := strings.TrimPrefix(latest, "v")
	return strings.TrimPrefix(current, "v") != norm && norm != ""
}

// verifySHA256 checks that data's SHA256 hex digest matches expected.
func verifySHA256(data []byte, expected string) error {
	h := sha256.Sum256(data)
	got := hex.EncodeToString(h[:])
	if got != strings.ToLower(expected) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// extractBinaryFromTarGz reads r as a .tar.gz, finds the entry named binaryName
// (matching the full name or the base component), writes it to dest with mode 0755.
func extractBinaryFromTarGz(r io.Reader, binaryName, dest string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if hdr.Name != binaryName && !strings.HasSuffix(hdr.Name, "/"+binaryName) {
			continue
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("create dest: %w", err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return fmt.Errorf("write binary: %w", err)
		}
		return f.Close()
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// applyUpdate downloads the release archive for tag, verifies SHA256, backs up
// the current binary to .prev, and atomically swaps in the new one.
// Does NOT restart the process — the caller handles that.
func applyUpdate(tag, goos, goarch, currentBin string, fetcher func(string) ([]byte, error)) error {
	artifact := artifactName(goos, goarch)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", githubRepo, tag)

	cksumData, err := fetcher(base + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	expectedSum, err := parseChecksum(string(cksumData), artifact)
	if err != nil {
		return fmt.Errorf("find checksum for %s: %w", artifact, err)
	}

	log.Printf("update: downloading %s %s", artifact, tag)
	archiveData, err := fetcher(base + "/" + artifact)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	if err := verifySHA256(archiveData, expectedSum); err != nil {
		return err
	}
	log.Printf("update: checksum verified")

	tmp := currentBin + ".new"
	if err := extractBinaryFromTarGz(bytes.NewReader(archiveData), filepath.Base(currentBin), tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("extract binary: %w", err)
	}

	prev := currentBin + ".prev"
	if err := os.Rename(currentBin, prev); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := os.Rename(tmp, currentBin); err != nil {
		_ = os.Rename(prev, currentBin) // best-effort rollback
		return fmt.Errorf("swap binary: %w", err)
	}

	log.Printf("update: swapped binary to %s", tag)
	return nil
}

type updateApplyRequest struct {
	Force bool `json:"force"`
}

type updateApplyResponse struct {
	Updated bool   `json:"updated"`
	Version string `json:"version,omitempty"`
	Message string `json:"message"`
}

// makeUpdateApplyHandler is the testable factory. In production pass:
//   - checkFetcher / applyFetcher: updateFetcher
//   - currentBin: result of os.Executable()
//   - goos / goarch: runtime.GOOS / runtime.GOARCH
//   - restart: func() { go scheduleRestart() }
func makeUpdateApplyHandler(
	checkFetcher, applyFetcher func(string) ([]byte, error),
	currentBin, goos, goarch string,
	restart func(),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateApplyRequest
		// Ignore decode errors — body is optional; force defaults to false.
		_ = json.NewDecoder(r.Body).Decode(&req)

		tag, _, err := latestRelease(checkFetcher)
		if err != nil {
			log.Printf("handleUpdateApply: check release: %v", err)
			jsonErr(w, fmt.Errorf("could not reach GitHub"), http.StatusBadGateway)
			return
		}
		if !req.Force && !needsUpdate(AppVersion, tag) {
			jsonOK(w, updateApplyResponse{Updated: false, Message: "already on latest " + AppVersion})
			return
		}
		if err := applyUpdate(tag, goos, goarch, currentBin, applyFetcher); err != nil {
			log.Printf("handleUpdateApply: apply: %v", err)
			jsonErr(w, fmt.Errorf("update failed: %w", err), http.StatusInternalServerError)
			return
		}
		msg := "binary swapped; restarting…"
		if req.Force {
			msg = "reinstalled " + tag + "; restarting…"
		}
		jsonOK(w, updateApplyResponse{Updated: true, Version: tag, Message: msg})
		restart()
	}
}

// scheduleRestart sends SIGTERM to the current process after a short delay so
// the HTTP response can flush. systemd (Restart=always) restarts with the new binary.
func scheduleRestart() {
	time.Sleep(500 * time.Millisecond)
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		log.Printf("update: cannot find self: %v", err)
		return
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		log.Printf("update: SIGTERM failed: %v", err)
	}
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	exe, err := os.Executable()
	if err != nil {
		jsonErr(w, fmt.Errorf("cannot determine executable path"), http.StatusInternalServerError)
		return
	}
	makeUpdateApplyHandler(
		updateFetcher, updateFetcher,
		exe, runtime.GOOS, runtime.GOARCH,
		func() { go scheduleRestart() },
	)(w, r)
}

// checkForUpdate returns a human-readable message about update availability.
// Used by the -update and -reinstall CLI flags.
func checkForUpdate(fetcher func(string) ([]byte, error)) (string, error) {
	tag, htmlURL, err := latestRelease(fetcher)
	if err != nil {
		return "", err
	}
	if !needsUpdate(AppVersion, tag) {
		return fmt.Sprintf("dune-admin %s is up to date", AppVersion), nil
	}
	return fmt.Sprintf("update available: %s → %s\n%s", AppVersion, tag, htmlURL), nil
}

// runSelfUpdate is the CLI entry point for -update and -reinstall flags.
// force=true reinstalls even when already on the latest version.
func runSelfUpdate(force bool) {
	tag, htmlURL, err := latestRelease(updateFetcher)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
		os.Exit(1)
	}
	if !force && !needsUpdate(AppVersion, tag) {
		fmt.Printf("dune-admin %s is up to date\n", AppVersion)
		return
	}
	if force && !needsUpdate(AppVersion, tag) {
		fmt.Printf("Reinstalling %s (current: %s)…\n", tag, AppVersion)
	} else {
		fmt.Printf("Update available: %s → %s\n%s\n", AppVersion, tag, htmlURL)
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine executable path: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Applying %s…\n", tag)
	if err := applyUpdate(tag, runtime.GOOS, runtime.GOARCH, exe, updateFetcher); err != nil {
		fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Done. Restart the service to run the new binary:\n  sudo systemctl restart dune-admin\n")
}
