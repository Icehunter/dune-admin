package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
)

func TestBuildEnvironmentAllowlist(t *testing.T) {
	origCfg := loadedConfig
	enabled := true
	loadedConfig = appConfig{Control: "amp", AuthEnabled: &enabled, MarketBotEnabled: &enabled}
	t.Cleanup(func() { loadedConfig = origCfg })

	env := buildEnvironment()
	if env.ControlPlane != "amp" {
		t.Errorf("ControlPlane = %q, want amp", env.ControlPlane)
	}
	if !env.AuthEnabled || !env.MarketBot {
		t.Errorf("expected auth + market bot enabled, got %+v", env)
	}
	if env.GoVersion != runtime.Version() || env.OS != runtime.GOOS {
		t.Errorf("runtime fields wrong: %+v", env)
	}
	if env.Version != AppVersion {
		t.Errorf("Version = %q, want %q", env.Version, AppVersion)
	}
}

func TestBuildEnvironmentControlDefault(t *testing.T) {
	origCfg := loadedConfig
	loadedConfig = appConfig{} // blank control
	t.Cleanup(func() { loadedConfig = origCfg })
	if got := buildEnvironment().ControlPlane; got != "local" {
		t.Errorf("blank control should default to local, got %q", got)
	}
}

func TestBuildReportRedactsAndTrims(t *testing.T) {
	lines := []ringLine{
		{Level: "info", Line: "dialing 192.168.0.59:8080"},
		{Level: "error", Line: "ServiceAuthToken=SECRET123 failed"},
	}
	env := environmentSummary{Version: "1.2.3", ControlPlane: "amp"}

	title, body := buildReport(lines, env, 8000)
	if !strings.Contains(title, "1.2.3") {
		t.Errorf("title missing version: %q", title)
	}
	if strings.Contains(body, "192.168.0.59") || strings.Contains(body, "SECRET123") {
		t.Fatalf("body leaked sensitive content:\n%s", body)
	}
	if !strings.Contains(body, "amp") {
		t.Errorf("body missing environment summary")
	}
}

func TestBuildReportTruncates(t *testing.T) {
	var lines []ringLine
	for i := 0; i < 5000; i++ {
		lines = append(lines, ringLine{Level: "info", Line: "padding line of text"})
	}
	_, body := buildReport(lines, environmentSummary{}, 2000)
	if len(body) > 2000 {
		t.Errorf("body = %d bytes, want <= 2000", len(body))
	}
	if !strings.Contains(body, "truncated") {
		t.Errorf("oversized body must carry a truncation marker")
	}
}

func TestWriteDiagnosticsBundleContents(t *testing.T) {
	lines := []ringLine{{Level: "info", Line: "user amp@192.168.0.59 connected"}}
	env := environmentSummary{Version: "9.9.9", ControlPlane: "local"}

	var buf bytes.Buffer
	if err := writeDiagnosticsBundle(&buf, lines, env); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		names[f.Name] = string(b)
	}
	if _, ok := names["app.log"]; !ok {
		t.Fatal("bundle missing app.log")
	}
	if _, ok := names["environment.txt"]; !ok {
		t.Fatal("bundle missing environment.txt")
	}
	if strings.Contains(names["app.log"], "192.168.0.59") {
		t.Fatalf("app.log not redacted: %s", names["app.log"])
	}
	if !strings.Contains(names["environment.txt"], "9.9.9") {
		t.Errorf("environment.txt missing version")
	}
}

func TestBuildReportKeepsNewestLines(t *testing.T) {
	var lines []ringLine
	for i := 0; i < 500; i++ {
		lines = append(lines, ringLine{Level: "info", Line: fmt.Sprintf("line-%03d", i)})
	}
	_, body := buildReport(lines, environmentSummary{}, 1200)
	// The newest line must be present...
	if !strings.Contains(body, "line-499") {
		t.Errorf("body should contain the newest line line-499:\n%s", body)
	}
	// ...and an old line must have been dropped (truncated).
	if strings.Contains(body, "line-000") {
		t.Errorf("oldest line line-000 should have been dropped")
	}
	if !strings.Contains(body, "truncated") {
		t.Errorf("dropped lines must be signalled with a truncation marker")
	}
	if len(body) > 1200 {
		t.Errorf("body = %d bytes, want <= 1200", len(body))
	}
}
