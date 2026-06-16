package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
		return "dune-admin_darwin_all.tar.gz"
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
			componentLog("update").Error().Err(err).Msg("check latest release failed")
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
//
// @Summary Check for a newer release on GitHub
// @Tags update
// @Produce json
// @Success 200 {object} updateCheckResponse
// @Failure 502 {object} map[string]string
// @Router /api/v1/update/check [get]
func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	makeUpdateCheckHandler(updateFetcher)(w, r)
}

// Dev builds ("-dev" suffix or bare "dev") are never auto-updated.
// Uses semver ordering so a running binary newer than the latest release
// (e.g. 0.17.0 vs published 0.16.0) is not offered as an "update".
func needsUpdate(current, latest string) bool {
	if strings.Contains(current, "-dev") || current == "dev" {
		return false
	}
	cv := parseSemver(strings.TrimPrefix(current, "v"))
	lv := parseSemver(strings.TrimPrefix(latest, "v"))
	if lv[0] != cv[0] {
		return lv[0] > cv[0]
	}
	if lv[1] != cv[1] {
		return lv[1] > cv[1]
	}
	return lv[2] > cv[2]
}

// parseSemver parses "major.minor.patch" into [3]int, ignoring pre-release suffixes.
func parseSemver(v string) [3]int {
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		// strip any pre-release suffix on the patch component
		p := strings.SplitN(parts[i], "-", 2)[0]
		for _, c := range p {
			if c >= '0' && c <= '9' {
				out[i] = out[i]*10 + int(c-'0')
			}
		}
	}
	return out
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

// extractBinaryFromZip reads a .zip from data, finds binaryName, writes it to dest with mode 0755.
func extractBinaryFromZip(r io.ReaderAt, size int64, binaryName, dest string) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != binaryName && !strings.HasSuffix(f.Name, "/"+binaryName) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("create dest: %w", err)
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		if copyErr != nil {
			_ = out.Close()
			return fmt.Errorf("write binary: %w", copyErr)
		}
		return out.Close()
	}
	return fmt.Errorf("binary %q not found in zip archive", binaryName)
}

// extractBinary dispatches to the correct extractor based on the artifact file extension.
func extractBinary(archiveData []byte, artifact, binaryName, dest string) error {
	if strings.HasSuffix(artifact, ".zip") {
		return extractBinaryFromZip(bytes.NewReader(archiveData), int64(len(archiveData)), binaryName, dest)
	}
	return extractBinaryFromTarGz(bytes.NewReader(archiveData), binaryName, dest)
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

	componentLog("update").Info().Str("artifact", artifact).Str("tag", tag).Msg("downloading release")
	archiveData, err := fetcher(base + "/" + artifact)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	if err := verifySHA256(archiveData, expectedSum); err != nil {
		return err
	}
	componentLog("update").Info().Msg("checksum verified")

	tmp := currentBin + ".new"
	if err := extractBinary(archiveData, artifact, filepath.Base(currentBin), tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("extract binary: %w", err)
	}

	prev := currentBin + ".prev"
	_ = os.Remove(prev) // Windows rename won't overwrite; remove stale .prev first
	if err := os.Rename(currentBin, prev); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := os.Rename(tmp, currentBin); err != nil {
		_ = os.Rename(prev, currentBin) // best-effort rollback
		return fmt.Errorf("swap binary: %w", err)
	}

	componentLog("update").Info().Str("tag", tag).Msg("swapped binary")
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
			componentLog("update").Error().Err(err).Msg("apply: check release failed")
			jsonErr(w, fmt.Errorf("could not reach GitHub"), http.StatusBadGateway)
			return
		}
		if !req.Force && !needsUpdate(AppVersion, tag) {
			jsonOK(w, updateApplyResponse{Updated: false, Message: "already on latest " + AppVersion})
			return
		}
		if err := applyUpdate(tag, goos, goarch, currentBin, applyFetcher); err != nil {
			componentLog("update").Error().Err(err).Msg("apply update failed")
			jsonErr(w, fmt.Errorf("update failed"), http.StatusInternalServerError)
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

// reExecSelf replaces the current process image with the freshly-swapped binary
// via execve, keeping the same PID. This makes self-update work regardless of
// the systemd Restart= policy — a unit with Restart=on-failure would otherwise
// not restart after the clean SIGTERM exit, leaving the service down.
//
// bin MUST be the executable path captured BEFORE applyUpdate ran: applyUpdate
// renames the running binary to <bin>.prev before moving the new one to <bin>,
// so os.Executable() now resolves (via /proc/self/exe) to the OLD .prev inode.
// Exec'ing that would relaunch the old binary; exec'ing the captured <bin> path
// runs the newly-installed one. Returns an error (rather than replacing the
// process) on platforms where Exec is unsupported, e.g. Windows.
func reExecSelf(bin string) error {
	if bin == "" {
		var err error
		if bin, err = os.Executable(); err != nil {
			return fmt.Errorf("locate executable: %w", err)
		}
	}
	return syscall.Exec(bin, os.Args, os.Environ()) // #nosec G204,G702 -- re-exec of our own binary at its original install path with our own args/env; no external input
}

// signalSelfTERM sends SIGTERM to the current process so a Restart=always unit
// restarts it. Used as the fallback when in-place re-exec is unavailable.
func signalSelfTERM() error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

// restartProcess re-executes the new binary in place; if that fails it invokes
// fallback (SIGTERM on Unix, spawn-and-exit on Windows). Extracted from
// scheduleRestart so the success/fallback branching is testable without
// actually replacing or killing the test process.
func restartProcess(reExec, fallback func() error) {
	if err := reExec(); err != nil {
		componentLog("update").Warn().Err(err).Msg("in-place re-exec failed; falling back to process restart")
		if serr := fallback(); serr != nil {
			componentLog("update").Error().Err(serr).Msg("process restart fallback failed")
		}
	}
}

// defaultSpawn starts bin as a new child process, inheriting stdout/stderr/stdin.
func defaultSpawn(bin string) error {
	cmd := exec.Command(bin, os.Args[1:]...) // #nosec G204,G702 -- own binary at its install path; no external input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Start()
}

// spawnAndExit starts a replacement process at bin and then calls exitFn(0).
// Returns an error if spawning fails (exitFn is NOT called in that case).
// exitFn is injected so tests can capture the call without actually exiting.
func spawnAndExit(bin string, spawn func(string) error, exitFn func(int)) error {
	if err := spawn(bin); err != nil {
		return fmt.Errorf("spawn replacement process: %w", err)
	}
	exitFn(0)
	return nil
}

// platformSignalOrSpawn returns the appropriate restart fallback for the OS.
// On Unix it sends SIGTERM (relies on systemd Restart=always to revive the service).
// On Windows, SIGTERM is unsupported, so it spawns a new process and exits instead.
func platformSignalOrSpawn(bin string) func() error {
	if runtime.GOOS == "windows" {
		return func() error { return spawnAndExit(bin, defaultSpawn, os.Exit) }
	}
	return signalSelfTERM
}

// scheduleRestart restarts into the new binary after a short delay so the HTTP
// response can flush. It prefers in-place re-exec (works under any systemd
// Restart= policy) and falls back to the platform-appropriate restart strategy.
// bin is the executable path captured before the binary swap — see reExecSelf.
func scheduleRestart(bin string) {
	time.Sleep(500 * time.Millisecond)
	restartProcess(func() error { return reExecSelf(bin) }, platformSignalOrSpawn(bin))
}

// @Summary Download and apply the latest release, then restart
// @Tags update
// @Accept json
// @Produce json
// @Param body body updateApplyRequest false "Set force=true to reinstall even when up to date"
// @Success 200 {object} updateApplyResponse
// @Failure 500 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Router /api/v1/update/apply [post]
func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	exe, err := os.Executable()
	if err != nil {
		jsonErr(w, fmt.Errorf("cannot determine executable path"), http.StatusInternalServerError)
		return
	}
	makeUpdateApplyHandler(
		updateFetcher, updateFetcher,
		exe, runtime.GOOS, runtime.GOARCH,
		// Capture exe (the pre-swap install path) so the re-exec runs the newly
		// installed binary, not the renamed .prev inode. See reExecSelf.
		func() { go scheduleRestart(exe) },
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
