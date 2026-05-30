// cmd/dune-admin/handlers_update_test.go
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		{"0.15.2", "", false}, // no release tag: treat as no update available
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
