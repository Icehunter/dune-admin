package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleDirectorINI = `[ Database ]
address=localhost
password=seabass

[ Battlegroup ]
DbFetchInterval=5 ;; seconds between fetch
ForceIsWorldClosed=false ;; override db value

[ InstancingModes ]
Survival_1=Dimension
DeepDesert_1=ClassicalInstancing
`

func findDirectorSection(secs []directorSection, name string) *directorSection {
	for i := range secs {
		if secs[i].Name == name {
			return &secs[i]
		}
	}
	return nil
}

func findDirectorKV(sec *directorSection, key string) *directorKV {
	if sec == nil {
		return nil
	}
	for i := range sec.Lines {
		if sec.Lines[i].Key == key {
			return &sec.Lines[i]
		}
	}
	return nil
}

func TestParseDirectorINI(t *testing.T) {
	t.Parallel()
	secs := parseDirectorINI(sampleDirectorINI)

	db := findDirectorSection(secs, "Database")
	if db == nil || !db.ReadOnly {
		t.Fatalf("Database section missing or not read-only: %+v", db)
	}
	if pw := findDirectorKV(db, "password"); pw == nil || !pw.Secret || pw.Value != "" {
		t.Errorf("password should be secret + blanked, got %+v", pw)
	}

	bg := findDirectorSection(secs, "Battlegroup")
	if bg == nil || bg.ReadOnly {
		t.Fatalf("Battlegroup section missing or wrongly read-only")
	}
	fetch := findDirectorKV(bg, "DbFetchInterval")
	if fetch == nil || fetch.Value != "5" || fetch.Comment != "seconds between fetch" {
		t.Errorf("DbFetchInterval parse wrong: %+v", fetch)
	}

	im := findDirectorSection(secs, "InstancingModes")
	if dd := findDirectorKV(im, "DeepDesert_1"); dd == nil || dd.Value != "ClassicalInstancing" {
		t.Errorf("InstancingModes DeepDesert_1 parse wrong: %+v", dd)
	}
}

func TestApplyDirectorEdits(t *testing.T) {
	t.Parallel()
	edits := map[string]map[string]string{
		"Battlegroup":     {"DbFetchInterval": "10"},
		"InstancingModes": {"DeepDesert_1": "Dimension"},
		"Database":        {"address": "evil-host", "password": "hacked"}, // read-only + secret → ignored
	}
	out := applyDirectorEdits(sampleDirectorINI, edits)

	if !strings.Contains(out, "DbFetchInterval=10 ;; seconds between fetch") {
		t.Errorf("edited value/comment not preserved:\n%s", out)
	}
	if !strings.Contains(out, "DeepDesert_1=Dimension") || strings.Contains(out, "DeepDesert_1=ClassicalInstancing") {
		t.Errorf("InstancingModes edit not applied:\n%s", out)
	}
	if !strings.Contains(out, "address=localhost") || strings.Contains(out, "evil-host") {
		t.Errorf("read-only Database section must NOT be edited:\n%s", out)
	}
	if !strings.Contains(out, "password=seabass") || strings.Contains(out, "hacked") {
		t.Errorf("secret/read-only key must NOT be edited:\n%s", out)
	}
}

func TestHandleGetDirectorConfig_NotConnected(t *testing.T) {
	origC, origE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	defer func() { globalControl, globalExecutor = origC, origE }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/director-config", nil)
	rr := httptest.NewRecorder()
	handleGetDirectorConfig(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}
