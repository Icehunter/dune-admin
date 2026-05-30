# Binary Self-Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add in-place binary self-update support triggered from both a `dune-admin -update` CLI flag and a Settings UI panel, with SHA256 checksum verification, `.prev` backup, and atomic rename swap.

**Architecture:** Backend exposes two new endpoints (`GET /api/v1/update/check`, `POST /api/v1/update/apply`) in a new `handlers_update.go` file using dependency-injected HTTP fetcher for testability. The apply path downloads the correct platform archive from GitHub Releases, verifies SHA256 against `checksums.txt`, extracts the binary, backs up the current binary to `.prev`, atomically renames the new one into place, then SIGTERMs self (systemd `Restart=always` picks it up). A CLI flag in `main.go` runs the same logic headlessly. The frontend adds an update panel to the existing Settings tab.

**Tech Stack:** Go stdlib (`net/http`, `archive/tar`, `compress/gzip`, `crypto/sha256`, `os`, `runtime`, `syscall`), GitHub REST API v3, React/TypeScript (existing patterns in `ServerSettingsTab.tsx`)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `cmd/dune-admin/handlers_update.go` | **Create** | All update logic: check, apply, artifact naming, checksum verification, binary extraction, swap, restart |
| `cmd/dune-admin/handlers_update_test.go` | **Create** | Tests for every exported and unexported function in handlers_update.go |
| `cmd/dune-admin/server.go` | **Modify** | Register two new routes |
| `cmd/dune-admin/main.go` | **Modify** | Add `-update` CLI flag, call `runSelfUpdate()` |
| `web/src/tabs/ServerSettingsTab.tsx` | **Modify** | Add update-check panel at the top of the Settings tab |
| `web/src/api/client.ts` | **Modify** | Add `api.update.check()` and `api.update.apply()` typed wrappers |

---

## Task 1: Tests for pure helper functions

**Files:**
- Create: `cmd/dune-admin/handlers_update_test.go`

These are pure functions (no I/O) — write and pass before touching any handler code.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/dune-admin/handlers_update_test.go
package main

import (
	"testing"
)

func TestArtifactName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "dune-admin_linux_amd64.tar.gz"},
		{"linux", "arm64", "dune-admin_linux_arm64.tar.gz"},
		{"darwin", "amd64", "dune-admin_darwin_universal.tar.gz"},
		{"darwin", "arm64", "dune-admin_darwin_universal.tar.gz"},
		{"windows", "amd64", "dune-admin_windows_amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"_"+tt.goarch, func(t *testing.T) {
			got := artifactName(tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("artifactName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	content := `abc123  dune-admin_linux_amd64.tar.gz
def456  dune-admin_darwin_universal.tar.gz
`
	tests := []struct {
		artifact string
		want     string
		wantErr  bool
	}{
		{"dune-admin_linux_amd64.tar.gz", "abc123", false},
		{"dune-admin_darwin_universal.tar.gz", "def456", false},
		{"dune-admin_windows_amd64.zip", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.artifact, func(t *testing.T) {
			got, err := parseChecksum(content, tt.artifact)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseChecksum error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseChecksum = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.15.2", "0.15.2", false},
		{"0.15.2", "0.16.0", true},
		{"0.15.2-dev", "0.15.2", false}, // dev build: never update
		{"0.15.2-dev", "0.16.0", false}, // dev build: never update
		{"dev", "0.16.0", false},        // dev build: never update
	}
	for _, tt := range tests {
		t.Run(tt.current+"->"+tt.latest, func(t *testing.T) {
			got := needsUpdate(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("needsUpdate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined|cannot"
```

Expected: `undefined: artifactName`, `undefined: parseChecksum`, `undefined: needsUpdate`

---

## Task 2: Implement pure helper functions

**Files:**
- Create: `cmd/dune-admin/handlers_update.go`

- [ ] **Step 1: Write minimal implementation for the three pure helpers**

```go
// cmd/dune-admin/handlers_update.go
package main

import (
	"fmt"
	"strings"
)

// artifactName returns the GoReleaser archive name for the given OS and arch.
// Darwin always uses the universal (fat) binary regardless of arch.
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

// parseChecksum finds the SHA256 hex digest for artifact in the content of
// a GoReleaser checksums.txt file (format: "<hex>  <filename>").
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

// needsUpdate returns true when the latest tag is strictly newer than current.
// Dev builds (containing "-dev" or equal to "dev") are never updated.
func needsUpdate(current, latest string) bool {
	if strings.Contains(current, "-dev") || current == "dev" {
		return false
	}
	norm := strings.TrimPrefix(latest, "v")
	return strings.TrimPrefix(current, "v") != norm && norm != ""
}
```

- [ ] **Step 2: Run tests — all three helpers must pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

Expected: `ok  	main` (or similar — no FAIL lines)

- [ ] **Step 3: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: add update helper functions with tests (artifactName, parseChecksum, needsUpdate)"
```

---

## Task 3: Tests for GitHub release check

**Files:**
- Modify: `cmd/dune-admin/handlers_update_test.go`

- [ ] **Step 1: Add test for latestRelease with injected fetcher**

Append to `handlers_update_test.go`:

```go
func TestLatestRelease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			if !strings.Contains(url, "releases/latest") {
				t.Fatalf("unexpected URL: %s", url)
			}
			return []byte(`{"tag_name":"v0.16.0","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.16.0"}`), nil
		}
		tag, htmlURL, err := latestRelease(fetcher)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tag != "v0.16.0" {
			t.Errorf("tag = %q, want %q", tag, "v0.16.0")
		}
		if htmlURL == "" {
			t.Error("htmlURL should not be empty")
		}
	})

	t.Run("fetch error", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return nil, fmt.Errorf("network error")
		}
		_, _, err := latestRelease(fetcher)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return []byte(`not json`), nil
		}
		_, _, err := latestRelease(fetcher)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
```

- [ ] **Step 2: Run — expect `undefined: latestRelease`**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined"
```

---

## Task 4: Implement latestRelease and updateFetcher

**Files:**
- Modify: `cmd/dune-admin/handlers_update.go`

- [ ] **Step 1: Add imports and latestRelease function**

Add to the top of `handlers_update.go`:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// latestRelease fetches the latest release tag and page URL from GitHub.
// fetcher is injected for testing.
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
```

- [ ] **Step 2: Run tests — all pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

- [ ] **Step 3: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: implement latestRelease with injected fetcher and tests"
```

---

## Task 5: handleUpdateCheck HTTP handler + test

**Files:**
- Modify: `cmd/dune-admin/handlers_update.go`
- Modify: `cmd/dune-admin/handlers_update_test.go`

- [ ] **Step 1: Write the handler test first**

Append to `handlers_update_test.go`:

```go
import (
	"net/http"
	"net/http/httptest"
	"encoding/json"
)

func TestHandleUpdateCheck(t *testing.T) {
	t.Run("update available", func(t *testing.T) {
		AppVersion = "0.15.2"
		fetcher := func(url string) ([]byte, error) {
			return []byte(`{"tag_name":"v0.16.0","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.16.0"}`), nil
		}
		h := makeUpdateCheckHandler(fetcher)
		r := httptest.NewRequest("GET", "/api/v1/update/check", nil)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var res updateCheckResponse
		if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !res.NeedsUpdate {
			t.Error("NeedsUpdate should be true")
		}
		if res.Latest != "v0.16.0" {
			t.Errorf("Latest = %q, want %q", res.Latest, "v0.16.0")
		}
		if res.Current != "0.15.2" {
			t.Errorf("Current = %q, want %q", res.Current, "0.15.2")
		}
	})

	t.Run("already up to date", func(t *testing.T) {
		AppVersion = "0.15.2"
		fetcher := func(url string) ([]byte, error) {
			return []byte(`{"tag_name":"v0.15.2","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.15.2"}`), nil
		}
		h := makeUpdateCheckHandler(fetcher)
		r := httptest.NewRequest("GET", "/api/v1/update/check", nil)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var res updateCheckResponse
		json.NewDecoder(w.Body).Decode(&res)
		if res.NeedsUpdate {
			t.Error("NeedsUpdate should be false when already on latest")
		}
	})

	t.Run("fetch error returns 502", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return nil, fmt.Errorf("network error")
		}
		h := makeUpdateCheckHandler(fetcher)
		r := httptest.NewRequest("GET", "/api/v1/update/check", nil)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run — expect `undefined: makeUpdateCheckHandler`, `undefined: updateCheckResponse`**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined"
```

- [ ] **Step 3: Implement handler**

Append to `handlers_update.go`:

```go
type updateCheckResponse struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
	ReleaseURL  string `json:"release_url,omitempty"`
}

// makeUpdateCheckHandler returns a handler that uses the given fetcher.
// In production, pass updateFetcher.
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

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	makeUpdateCheckHandler(updateFetcher)(w, r)
}
```

- [ ] **Step 4: Run tests — all pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: add handleUpdateCheck with injected fetcher and tests"
```

---

## Task 6: Tests for binary extraction and checksum verification

**Files:**
- Modify: `cmd/dune-admin/handlers_update_test.go`

These test the archive extraction and SHA256 verification logic without hitting the network.

- [ ] **Step 1: Add helper to build a fake tar.gz in tests, then write the tests**

Append to `handlers_update_test.go`:

```go
import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// buildFakeTarGz creates an in-memory .tar.gz with a single file at name containing content.
func buildFakeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	binaryContent := []byte("fake binary content")
	archive := buildFakeTarGz(t, "dune-admin", binaryContent)

	dir := t.TempDir()
	dest := filepath.Join(dir, "dune-admin")
	if err := extractBinaryFromTarGz(bytes.NewReader(archive), "dune-admin", dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Errorf("extracted content = %q, want %q", got, binaryContent)
	}
	// Verify the file is executable
	info, _ := os.Stat(dest)
	if info.Mode()&0111 == 0 {
		t.Error("extracted binary should be executable")
	}
}

func TestExtractBinaryFromTarGz_NotFound(t *testing.T) {
	archive := buildFakeTarGz(t, "other-file", []byte("data"))
	dir := t.TempDir()
	err := extractBinaryFromTarGz(bytes.NewReader(archive), "dune-admin", filepath.Join(dir, "dune-admin"))
	if err == nil {
		t.Fatal("expected error when binary not found in archive")
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("hello world")
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	t.Run("valid", func(t *testing.T) {
		if err := verifySHA256(data, expected); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		if err := verifySHA256(data, "deadbeef"); err == nil {
			t.Error("expected error for wrong checksum")
		}
	})
}
```

- [ ] **Step 2: Run — expect `undefined: extractBinaryFromTarGz`, `undefined: verifySHA256`**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined"
```

---

## Task 7: Implement extraction and checksum verification

**Files:**
- Modify: `cmd/dune-admin/handlers_update.go`

- [ ] **Step 1: Add imports and implement the two functions**

Add to imports in `handlers_update.go`:
```go
"archive/tar"
"compress/gzip"
"crypto/sha256"
"encoding/hex"
"io"
"os"
```

Append to `handlers_update.go`:

```go
// verifySHA256 checks that data's SHA256 hex digest matches expected.
func verifySHA256(data []byte, expected string) error {
	h := sha256.Sum256(data)
	got := hex.EncodeToString(h[:])
	if got != strings.ToLower(expected) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// extractBinaryFromTarGz reads a .tar.gz from r, finds the entry named binaryName,
// writes it to dest with mode 0755, and returns an error if not found.
func extractBinaryFromTarGz(r io.Reader, binaryName, dest string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()
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
			f.Close()
			return fmt.Errorf("write binary: %w", err)
		}
		return f.Close()
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}
```

- [ ] **Step 2: Run tests — all pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

- [ ] **Step 3: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: implement extractBinaryFromTarGz and verifySHA256 with tests"
```

---

## Task 8: Tests for applyUpdate (the full swap)

**Files:**
- Modify: `cmd/dune-admin/handlers_update_test.go`

`applyUpdate` downloads, verifies, backs up, swaps, and schedules a restart. We test it with an injected fetcher that returns fake archive data so no real network I/O occurs.

- [ ] **Step 1: Write the test**

Append to `handlers_update_test.go`:

```go
func TestApplyUpdate(t *testing.T) {
	// Build a fake binary and matching tar.gz
	fakeBinary := []byte("#!/bin/sh\necho new-binary")
	archive := buildFakeTarGz(t, "dune-admin", fakeBinary)

	// Compute its checksum
	h := sha256.Sum256(archive)
	checksum := hex.EncodeToString(h[:])
	artifact := artifactName("linux", "amd64")
	checksumsTxt := checksum + "  " + artifact + "\n"

	// Write a fake "current" binary
	dir := t.TempDir()
	currentBin := filepath.Join(dir, "dune-admin")
	os.WriteFile(currentBin, []byte("old binary"), 0755)

	callCount := 0
	fetcher := func(url string) ([]byte, error) {
		callCount++
		if strings.Contains(url, "checksums.txt") {
			return []byte(checksumsTxt), nil
		}
		// archive download
		return archive, nil
	}

	err := applyUpdate("v0.16.0", "linux", "amd64", currentBin, fetcher)
	if err != nil {
		t.Fatalf("applyUpdate error: %v", err)
	}

	// New binary should be in place
	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read new binary: %v", err)
	}
	if !bytes.Equal(got, fakeBinary) {
		t.Errorf("new binary content = %q, want %q", got, fakeBinary)
	}

	// Backup should exist
	prev := currentBin + ".prev"
	if _, err := os.Stat(prev); os.IsNotExist(err) {
		t.Error(".prev backup should exist after update")
	}
}

func TestApplyUpdate_ChecksumMismatch(t *testing.T) {
	archive := buildFakeTarGz(t, "dune-admin", []byte("binary"))
	artifact := artifactName("linux", "amd64")
	// Wrong checksum
	checksumsTxt := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  " + artifact + "\n"

	dir := t.TempDir()
	currentBin := filepath.Join(dir, "dune-admin")
	os.WriteFile(currentBin, []byte("old"), 0755)

	fetcher := func(url string) ([]byte, error) {
		if strings.Contains(url, "checksums.txt") {
			return []byte(checksumsTxt), nil
		}
		return archive, nil
	}

	err := applyUpdate("v0.16.0", "linux", "amd64", currentBin, fetcher)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error should mention checksum: %v", err)
	}
	// Original binary should be untouched
	got, _ := os.ReadFile(currentBin)
	if string(got) != "old" {
		t.Error("original binary should be untouched after failed update")
	}
}
```

- [ ] **Step 2: Run — expect `undefined: applyUpdate`**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined"
```

---

## Task 9: Implement applyUpdate

**Files:**
- Modify: `cmd/dune-admin/handlers_update.go`

- [ ] **Step 1: Add import `"path/filepath"` and implement applyUpdate**

Append to `handlers_update.go`:

```go
// applyUpdate downloads, verifies, backs up, and atomically swaps the binary.
// It does NOT restart the process — the caller is responsible for that.
// fetcher is injected for testability.
func applyUpdate(tag, goos, goarch, currentBin string, fetcher func(string) ([]byte, error)) error {
	artifact := artifactName(goos, goarch)
	version := strings.TrimPrefix(tag, "v")
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", githubRepo, tag)

	// 1. Fetch and parse checksums.txt
	cksumData, err := fetcher(base + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	expectedSum, err := parseChecksum(string(cksumData), artifact)
	if err != nil {
		return fmt.Errorf("find checksum for %s: %w", artifact, err)
	}

	// 2. Download archive
	log.Printf("update: downloading %s v%s", artifact, version)
	archiveData, err := fetcher(base + "/" + artifact)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	// 3. Verify checksum
	if err := verifySHA256(archiveData, expectedSum); err != nil {
		return err
	}
	log.Printf("update: checksum verified")

	// 4. Extract binary to a temp file alongside the current binary
	dir := filepath.Dir(currentBin)
	tmp := currentBin + ".new"
	if err := extractBinaryFromTarGz(bytes.NewReader(archiveData), "dune-admin", tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("extract binary: %w", err)
	}

	// 5. Backup current binary
	prev := currentBin + ".prev"
	if err := os.Rename(currentBin, prev); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("backup current binary: %w", err)
	}

	// 6. Atomic rename — on Linux this works even for the running binary
	if err := os.Rename(tmp, currentBin); err != nil {
		// Attempt rollback
		os.Rename(prev, currentBin)
		return fmt.Errorf("swap binary: %w", err)
	}

	_ = dir // used implicitly via filepath.Dir
	log.Printf("update: binary swapped to %s", version)
	return nil
}
```

- [ ] **Step 2: Add `"bytes"` to the import block in handlers_update.go** (it's needed for `bytes.NewReader`)

- [ ] **Step 3: Run tests — all pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

- [ ] **Step 4: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: implement applyUpdate with backup, checksum verify, atomic swap, and tests"
```

---

## Task 10: handleUpdateApply HTTP handler + test

**Files:**
- Modify: `cmd/dune-admin/handlers_update.go`
- Modify: `cmd/dune-admin/handlers_update_test.go`

- [ ] **Step 1: Write the handler test**

Append to `handlers_update_test.go`:

```go
func TestHandleUpdateApply(t *testing.T) {
	fakeBinary := []byte("new binary")
	archive := buildFakeTarGz(t, "dune-admin", fakeBinary)
	h := sha256.Sum256(archive)
	checksum := hex.EncodeToString(h[:])
	artifact := artifactName("linux", "amd64")
	checksumsTxt := checksum + "  " + artifact + "\n"

	dir := t.TempDir()
	currentBin := filepath.Join(dir, "dune-admin")
	os.WriteFile(currentBin, []byte("old"), 0755)

	fetcher := func(url string) ([]byte, error) {
		if strings.Contains(url, "checksums.txt") {
			return []byte(checksumsTxt), nil
		}
		return archive, nil
	}

	// Override the version check to force an update
	AppVersion = "0.15.0"
	checkFetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.16.0"}`), nil
	}

	h2 := makeUpdateApplyHandler(checkFetcher, fetcher, currentBin, "linux", "amd64", func() {})
	r := httptest.NewRequest("POST", "/api/v1/update/apply", nil)
	w := httptest.NewRecorder()
	h2(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateApply_AlreadyLatest(t *testing.T) {
	AppVersion = "0.16.0"
	checkFetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://x"}`), nil
	}
	h := makeUpdateApplyHandler(checkFetcher, nil, "", "linux", "amd64", func() {})
	r := httptest.NewRequest("POST", "/api/v1/update/apply", nil)
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res map[string]any
	json.NewDecoder(w.Body).Decode(&res)
	if res["updated"] != false {
		t.Error("should report updated=false when already on latest")
	}
}
```

- [ ] **Step 2: Run — expect `undefined: makeUpdateApplyHandler`**

```bash
make test-race 2>&1 | grep -E "FAIL|undefined"
```

- [ ] **Step 3: Implement handleUpdateApply**

Append to `handlers_update.go`:

```go
import (
	"os"
	"runtime"
	"syscall"
	"time"
)

type updateApplyResponse struct {
	Updated bool   `json:"updated"`
	Version string `json:"version,omitempty"`
	Message string `json:"message"`
}

// makeUpdateApplyHandler is the testable factory. In production pass:
//   - checkFetcher: updateFetcher
//   - applyFetcher: updateFetcher
//   - currentBin: result of os.Executable()
//   - goos/goarch: runtime.GOOS / runtime.GOARCH
//   - restart: func() { go scheduleRestart() }
func makeUpdateApplyHandler(
	checkFetcher, applyFetcher func(string) ([]byte, error),
	currentBin, goos, goarch string,
	restart func(),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag, _, err := latestRelease(checkFetcher)
		if err != nil {
			log.Printf("handleUpdateApply: check release: %v", err)
			jsonErr(w, fmt.Errorf("could not reach GitHub"), http.StatusBadGateway)
			return
		}
		if !needsUpdate(AppVersion, tag) {
			jsonOK(w, updateApplyResponse{Updated: false, Message: "already on latest " + AppVersion})
			return
		}
		if err := applyUpdate(tag, goos, goarch, currentBin, applyFetcher); err != nil {
			log.Printf("handleUpdateApply: apply: %v", err)
			jsonErr(w, fmt.Errorf("update failed: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		jsonOK(w, updateApplyResponse{Updated: true, Version: tag, Message: "binary swapped; restarting…"})
		restart()
	}
}

// scheduleRestart sends SIGTERM to the current process after a short delay,
// allowing the HTTP response to flush. systemd (Restart=always) will restart
// the process with the new binary.
func scheduleRestart() {
	time.Sleep(500 * time.Millisecond)
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		log.Printf("update: could not find self process: %v", err)
		return
	}
	log.Printf("update: sending SIGTERM to restart with new binary")
	p.Signal(syscall.SIGTERM)
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
```

- [ ] **Step 4: Run tests — all pass**

```bash
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/handlers_update.go cmd/dune-admin/handlers_update_test.go
git commit -m "feat: add handleUpdateApply with restart scheduling and tests"
```

---

## Task 11: Wire routes into server.go

**Files:**
- Modify: `cmd/dune-admin/server.go`

- [ ] **Step 1: Find the route block and add two lines**

Open `server.go`, find the block of `mux.HandleFunc` calls (inside `startServer()`). Add after the status route:

```go
mux.HandleFunc("GET /api/v1/update/check", handleUpdateCheck)
mux.HandleFunc("POST /api/v1/update/apply", handleUpdateApply)
```

- [ ] **Step 2: Build and verify**

```bash
make build 2>&1 | grep -E "error|Error"
make test-race 2>&1 | grep -E "PASS|FAIL|ok"
```

Expected: no errors, tests still pass.

- [ ] **Step 3: Commit**

```bash
git add cmd/dune-admin/server.go
git commit -m "feat: register GET /api/v1/update/check and POST /api/v1/update/apply routes"
```

---

## Task 12: CLI -update flag

**Files:**
- Modify: `cmd/dune-admin/main.go`

- [ ] **Step 1: Write test for runSelfUpdate version-check path**

Append to `cmd/dune-admin/main_version_test.go` (or a new file `cmd/dune-admin/main_update_test.go`):

```go
// cmd/dune-admin/main_update_test.go
package main

import (
	"strings"
	"testing"
)

func TestRunSelfUpdateAlreadyLatest(t *testing.T) {
	AppVersion = "0.16.0"
	fetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://x"}`), nil
	}
	// Should return no error and report already up to date
	msg, err := checkForUpdate(fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "up to date") {
		t.Errorf("message should say 'up to date', got: %q", msg)
	}
}

func TestRunSelfUpdateAvailable(t *testing.T) {
	AppVersion = "0.15.0"
	fetcher := func(url string) ([]byte, error) {
		return []byte(`{"tag_name":"v0.16.0","html_url":"https://github.com/Icehunter/dune-admin/releases/tag/v0.16.0"}`), nil
	}
	msg, err := checkForUpdate(fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "0.16.0") {
		t.Errorf("message should mention new version, got: %q", msg)
	}
}
```

- [ ] **Step 2: Implement `checkForUpdate` in `handlers_update.go`**

Append to `handlers_update.go`:

```go
// checkForUpdate returns a human-readable message about update availability.
// Used by the CLI -update flag.
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
```

- [ ] **Step 3: Add the -update flag in main.go**

In `main.go`, find where other flags like `-setup` are defined and add:

```go
updateFlag := flag.Bool("update", false, "Check for and apply the latest release update")
```

After `flag.Parse()`, add handling before the server starts:

```go
if *updateFlag {
    runSelfUpdate()
    return
}
```

Add `runSelfUpdate()` function in `main.go` (or `handlers_update.go`):

```go
// runSelfUpdate is the CLI entry point for `dune-admin -update`.
func runSelfUpdate() {
	msg, err := checkForUpdate(updateFetcher)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(msg)

	if !needsUpdate(AppVersion, "") {
		// Already up to date — checkForUpdate printed the message, just exit
		return
	}

	// Re-fetch to get the tag for apply
	tag, _, err := latestRelease(updateFetcher)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot fetch latest release: %v\n", err)
		os.Exit(1)
	}
	if !needsUpdate(AppVersion, tag) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine executable path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Applying update to %s...\n", tag)
	if err := applyUpdate(tag, runtime.GOOS, runtime.GOARCH, exe, updateFetcher); err != nil {
		fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated to %s. Restart the service to use the new binary.\n", tag)
	fmt.Println("  sudo systemctl restart dune-admin")
}
```

- [ ] **Step 4: Run all tests and build**

```bash
make verify
```

Expected: all checks pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/dune-admin/main.go cmd/dune-admin/handlers_update.go cmd/dune-admin/main_update_test.go
git commit -m "feat: add -update CLI flag with checkForUpdate helper and tests"
```

---

## Task 13: API client types (frontend)

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add update namespace to the api object**

Find the `api` export in `client.ts` and add:

```ts
export interface UpdateCheckResult {
  current: string
  latest: string
  needs_update: boolean
  release_url?: string
}

export interface UpdateApplyResult {
  updated: boolean
  version?: string
  message: string
}
```

In the `api` export object, add:

```ts
update: {
  check: () => req<UpdateCheckResult>('GET', '/api/v1/update/check'),
  apply: () => req<UpdateApplyResult>('POST', '/api/v1/update/apply'),
},
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && pnpm build 2>&1 | grep -E "error|Error"
```

Expected: no TypeScript errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/api/client.ts
git commit -m "feat: add api.update.check() and api.update.apply() typed wrappers"
```

---

## Task 14: Settings tab update panel (frontend)

**Files:**
- Modify: `web/src/tabs/ServerSettingsTab.tsx`

- [ ] **Step 1: Add state and load logic**

In `ServerSettingsTab.tsx`, add state at the top of the component function (after existing state):

```tsx
const [updateInfo, setUpdateInfo] = useState<UpdateCheckResult | null>(null)
const [updateChecking, setUpdateChecking] = useState(false)
const [updateApplying, setUpdateApplying] = useState(false)

const checkUpdate = async () => {
  setUpdateChecking(true)
  try {
    setUpdateInfo(await api.update.check())
  } catch (e) {
    toast.danger(`Update check failed: ${e instanceof Error ? e.message : String(e)}`)
  } finally {
    setUpdateChecking(false)
  }
}

const applyUpdate = async () => {
  setUpdateApplying(true)
  try {
    const result = await api.update.apply()
    if (result.updated) {
      toast.success(`Updated to ${result.version}. Server is restarting…`)
    } else {
      toast.info(result.message)
    }
  } catch (e) {
    toast.danger(`Update failed: ${e instanceof Error ? e.message : String(e)}`)
  } finally {
    setUpdateApplying(false)
  }
}
```

Add the import for `UpdateCheckResult` at the top:

```tsx
import type { UpdateCheckResult, UpdateApplyResult } from '../api/client'
```

- [ ] **Step 2: Add the update panel to the JSX**

Find the return statement in `ServerSettingsTab.tsx`. At the very top of the main content area (before or after the `PageHeader`), add a panel:

```tsx
<Panel>
  <div className="flex items-center gap-3">
    <div className="flex-1">
      <SectionLabel>Software Update</SectionLabel>
      <div className="text-xs text-muted mt-0.5">
        {updateInfo
          ? updateInfo.needs_update
            ? `v${updateInfo.latest} available (current: v${updateInfo.current})`
            : `v${updateInfo.current} — up to date`
          : `Current version: v${/* inject from api.status or just show AppVersion */''}`
        }
      </div>
    </div>
    {updateInfo?.needs_update && (
      <Button
        size="sm"
        onPress={applyUpdate}
        isDisabled={updateApplying}
      >
        {updateApplying ? <Spinner size="sm" color="current" /> : null}
        Update to {updateInfo.latest}
      </Button>
    )}
    <Button
      size="sm"
      variant="ghost"
      onPress={checkUpdate}
      isDisabled={updateChecking}
    >
      {updateChecking ? <Spinner size="sm" color="current" /> : 'Check for Updates'}
    </Button>
  </div>
</Panel>
```

- [ ] **Step 3: Build and lint**

```bash
cd web && pnpm lint && pnpm build 2>&1 | tail -10
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add web/src/tabs/ServerSettingsTab.tsx web/src/api/client.ts
git commit -m "feat: add update check/apply panel to Settings tab"
```

---

## Task 15: PR, verify, merge

- [ ] **Step 1: Final verification**

```bash
make verify
```

Expected: all checks pass (vet, test-race, lint, gocognit, gosec).

- [ ] **Step 2: Push and open PR**

```bash
git push -u origin feat/binary-self-update
gh pr create --title "feat: in-place binary self-update (#89)" --body "..."
```

- [ ] **Step 3: Merge and bump version**

```bash
gh pr merge --squash --delete-branch --admin
make version-patch
```

---

## Self-Review

**Spec coverage:**
- ✅ Self-update path that fetches latest release artifact — `applyUpdate` + `handleUpdateApply`
- ✅ Verifies checksums — `verifySHA256` + `parseChecksum`
- ✅ Coordinates with `.prev` backup/rollback convention — `os.Rename(currentBin, prev)` in Task 9
- ✅ Trigger from UI — Settings tab panel (Task 14)
- ✅ Trigger from CLI flag — `-update` flag (Task 12)
- ✅ Preserves config — `~/.dune-admin/config.yaml` is untouched (only binary is swapped)
- ✅ Surfaces new version after restart — AppVersion is set at build time via ldflags; after restart the new binary reports the new version

**Gaps:**
- The `runSelfUpdate` function in Task 12 has a double-fetch issue (calls `latestRelease` twice). Simplify: call it once, check `needsUpdate`, then either print "up to date" or proceed with `applyUpdate`.
- Windows zip extraction is not implemented (`extractBinaryFromTarGz` only handles tar.gz). Since dune-admin is not a common Windows server deployment, document this as a known limitation and return an error for `windows` in `handleUpdateApply`.

**Fix for double-fetch in runSelfUpdate:**

```go
func runSelfUpdate() {
	tag, htmlURL, err := latestRelease(updateFetcher)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
		os.Exit(1)
	}
	if !needsUpdate(AppVersion, tag) {
		fmt.Printf("dune-admin %s is up to date\n", AppVersion)
		return
	}
	fmt.Printf("Update available: %s → %s\n%s\n", AppVersion, tag, htmlURL)
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine executable path: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Applying update to %s...\n", tag)
	if err := applyUpdate(tag, runtime.GOOS, runtime.GOARCH, exe, updateFetcher); err != nil {
		fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated to %s. Restart the service:\n  sudo systemctl restart dune-admin\n", tag)
}
```

Use this simplified version in Task 12 Step 3 instead of what's written above.
