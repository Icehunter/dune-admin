// cmd/dune-admin/handlers_update_test.go
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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	content := "abc123  dune-admin_linux_amd64.tar.gz\ndef456  dune-admin_darwin_universal.tar.gz\n"
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
		{"0.15.2-dev", "0.15.2", false},
		{"0.15.2-dev", "0.16.0", false},
		{"dev", "0.16.0", false},
		{"0.15.2", "", false},
		// semver: running newer than latest must NOT trigger update
		{"0.17.0", "v0.16.0", false},
		{"1.0.0", "v0.99.0", false},
		{"0.16.1", "v0.16.0", false},
		// semver: patch bump only
		{"0.15.2", "v0.15.3", true}, // no release tag: treat as no update available
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

	t.Run("fetch_error", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return nil, fmt.Errorf("network error")
		}
		_, _, err := latestRelease(fetcher)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return []byte(`not json`), nil
		}
		_, _, err := latestRelease(fetcher)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty_tag", func(t *testing.T) {
		fetcher := func(url string) ([]byte, error) {
			return []byte(`{"tag_name":"","html_url":""}`), nil
		}
		_, _, err := latestRelease(fetcher)
		if err == nil {
			t.Fatal("expected error for empty tag_name")
		}
	})
}

func TestHandleUpdateCheck(t *testing.T) {
	t.Run("update_available", func(t *testing.T) {
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

	t.Run("already_up_to_date", func(t *testing.T) {
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
		if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if res.NeedsUpdate {
			t.Error("NeedsUpdate should be false when already on latest")
		}
	})

	t.Run("fetch_error_returns_502", func(t *testing.T) {
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

// buildFakeTarGz creates an in-memory .tar.gz containing one file at name with the given content.
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
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildFakeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFromZip(t *testing.T) {
	binaryContent := []byte("fake windows binary")
	archive := buildFakeZip(t, "dune-admin.exe", binaryContent)

	dir := t.TempDir()
	dest := filepath.Join(dir, "dune-admin.exe")
	if err := extractBinaryFromZip(bytes.NewReader(archive), int64(len(archive)), "dune-admin.exe", dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Errorf("extracted content = %q, want %q", got, binaryContent)
	}
}

func TestExtractBinaryFromZip_NotFound(t *testing.T) {
	archive := buildFakeZip(t, "other.exe", []byte("data"))
	dir := t.TempDir()
	err := extractBinaryFromZip(bytes.NewReader(archive), int64(len(archive)), "dune-admin.exe", filepath.Join(dir, "dune-admin.exe"))
	if err == nil {
		t.Fatal("expected error when binary not found in archive")
	}
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

func TestApplyUpdate(t *testing.T) {
	fakeBinary := []byte("#!/bin/sh\necho new-binary")
	archive := buildFakeTarGz(t, "dune-admin", fakeBinary)

	h := sha256.Sum256(archive)
	checksum := hex.EncodeToString(h[:])
	artifact := artifactName("linux", "amd64")
	checksumsTxt := checksum + "  " + artifact + "\n"

	dir := t.TempDir()
	currentBin := filepath.Join(dir, "dune-admin")
	if err := os.WriteFile(currentBin, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}

	fetcher := func(url string) ([]byte, error) {
		if strings.Contains(url, "checksums.txt") {
			return []byte(checksumsTxt), nil
		}
		return archive, nil
	}

	if err := applyUpdate("v0.16.0", "linux", "amd64", currentBin, fetcher); err != nil {
		t.Fatalf("applyUpdate error: %v", err)
	}

	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read new binary: %v", err)
	}
	if !bytes.Equal(got, fakeBinary) {
		t.Errorf("new binary content = %q, want %q", got, fakeBinary)
	}
	if _, err := os.Stat(currentBin + ".prev"); os.IsNotExist(err) {
		t.Error(".prev backup should exist after update")
	}
}

func TestApplyUpdate_ChecksumMismatch(t *testing.T) {
	archive := buildFakeTarGz(t, "dune-admin", []byte("binary"))
	artifact := artifactName("linux", "amd64")
	checksumsTxt := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  " + artifact + "\n"

	dir := t.TempDir()
	currentBin := filepath.Join(dir, "dune-admin")
	if err := os.WriteFile(currentBin, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

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
	got, _ := os.ReadFile(currentBin)
	if string(got) != "old" {
		t.Error("original binary should be untouched after failed update")
	}
}

func TestHandleUpdateApply(t *testing.T) {
	// Build reusable test archive
	fakeBinary := []byte("new binary")
	archive := buildFakeTarGz(t, "dune-admin", fakeBinary)
	h := sha256.Sum256(archive)
	checksum := hex.EncodeToString(h[:])
	artifact := artifactName("linux", "amd64")
	checksumsTxt := checksum + "  " + artifact + "\n"

	makeApplyFetcher := func() func(string) ([]byte, error) {
		return func(url string) ([]byte, error) {
			if strings.Contains(url, "checksums.txt") {
				return []byte(checksumsTxt), nil
			}
			return archive, nil
		}
	}
	makeCheckFetcher := func(tag string) func(string) ([]byte, error) {
		return func(url string) ([]byte, error) {
			return []byte(`{"tag_name":"` + tag + `","html_url":"https://x"}`), nil
		}
	}

	tests := []struct {
		name        string
		appVersion  string
		latestTag   string
		body        string
		checkErr    bool
		applyErr    bool
		wantUpdated bool
		wantStatus  int
	}{
		{"normal_update", "0.15.0", "v0.16.0", "", false, false, true, http.StatusOK},
		{"already_latest", "0.16.0", "v0.16.0", "", false, false, false, http.StatusOK},
		{"force_reinstall", "0.16.0", "v0.16.0", `{"force":true}`, false, false, true, http.StatusOK},
		{name: "github_unreachable", checkErr: true, wantStatus: http.StatusBadGateway, wantUpdated: false},
		{name: "apply_failure", appVersion: "0.15.0", latestTag: "v0.16.0", applyErr: true, wantStatus: http.StatusInternalServerError, wantUpdated: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AppVersion = tt.appVersion

			dir := t.TempDir()
			currentBin := filepath.Join(dir, "dune-admin")
			if err := os.WriteFile(currentBin, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}

			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = http.NoBody
			}

			checkFetcher := makeCheckFetcher(tt.latestTag)
			if tt.checkErr {
				checkFetcher = func(url string) ([]byte, error) {
					return nil, fmt.Errorf("network error")
				}
			}
			applyFetcher := makeApplyFetcher()
			if tt.applyErr {
				applyFetcher = func(url string) ([]byte, error) {
					if strings.Contains(url, "checksums.txt") {
						// checksums.txt fetches fine, archive download fails
						h2 := sha256.Sum256(archive)
						ck := hex.EncodeToString(h2[:])
						return []byte(ck + "  " + artifact + "\n"), nil
					}
					return nil, fmt.Errorf("download failed")
				}
			}

			handler := makeUpdateApplyHandler(
				checkFetcher,
				applyFetcher,
				currentBin, "linux", "amd64",
				func() {}, // no-op restart in tests
			)
			r := httptest.NewRequest("POST", "/api/v1/update/apply", body)
			w := httptest.NewRecorder()
			handler(w, r)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				var res updateApplyResponse
				if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if res.Updated != tt.wantUpdated {
					t.Errorf("Updated = %v, want %v (message: %s)", res.Updated, tt.wantUpdated, res.Message)
				}
			}
		})
	}
}
