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

// inlineCommentStart tests
func TestInlineCommentStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s    string
		want int
	}{
		{"false ;; some comment", 6},   // ;; starts at 6 (after "false ")
		{"false ; some comment", 5},    // " ; " starts at 5 (the leading space)
		{"false : some comment", 5},    // " : " starts at 5 (the leading space)
		{"value", -1},                  // no comment
		{"5", -1},                      // no comment
		{"ClassicalInstancing", -1},    // no comment
		{" ;; leading space delim", 1}, // ;; starts at 1 (after the leading space)
	}
	for _, tt := range tests {
		got := inlineCommentStart(tt.s)
		if got != tt.want {
			t.Errorf("inlineCommentStart(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

// splitDirectorLine tests for alternative comment delimiters
func TestSplitDirectorLine_Delimiters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line        string
		wantKey     string
		wantValue   string
		wantComment string
	}{
		{
			// existing double-semicolon style
			line:        "DbFetchInterval=5 ;; seconds between fetch",
			wantKey:     "DbFetchInterval",
			wantValue:   "5",
			wantComment: "seconds between fetch",
		},
		{
			// single semicolon with spaces (Funcom style)
			line:        "KeepPartiesTogether=false ; Remove when PlayerHardCap is changed to > 1",
			wantKey:     "KeepPartiesTogether",
			wantValue:   "false",
			wantComment: "Remove when PlayerHardCap is changed to > 1",
		},
		{
			// colon delimiter
			line:        "KeepPartiesTogether=false : Remove when PlayerHardCap is changed to > 1",
			wantKey:     "KeepPartiesTogether",
			wantValue:   "false",
			wantComment: "Remove when PlayerHardCap is changed to > 1",
		},
		{
			// no comment
			line:      "Survival_1=Dimension",
			wantKey:   "Survival_1",
			wantValue: "Dimension",
		},
	}
	for _, tt := range tests {
		eq := strings.Index(tt.line, "=")
		kv := splitDirectorLine(tt.line, eq)
		if kv.Key != tt.wantKey || kv.Value != tt.wantValue || kv.Comment != tt.wantComment {
			t.Errorf("splitDirectorLine(%q) = {Key:%q Value:%q Comment:%q}, want {Key:%q Value:%q Comment:%q}",
				tt.line, kv.Key, kv.Value, kv.Comment, tt.wantKey, tt.wantValue, tt.wantComment)
		}
	}
}

// applyDirectorEdits must preserve the original comment delimiter verbatim
func TestApplyDirectorEdits_Delimiters(t *testing.T) {
	t.Parallel()
	ini := `[ Battlegroup ]
KeepPartiesTogether=false : Remove when PlayerHardCap is changed to > 1
MaxPlayersPerParty=10 ; max players
NoComment=old
`
	edits := map[string]map[string]string{
		"Battlegroup": {
			"KeepPartiesTogether": "true",
			"MaxPlayersPerParty":  "20",
			"NoComment":           "new",
		},
	}
	out := applyDirectorEdits(ini, edits)

	if !strings.Contains(out, "KeepPartiesTogether=true : Remove when PlayerHardCap is changed to > 1") {
		t.Errorf("colon delimiter not preserved:\n%s", out)
	}
	if !strings.Contains(out, "MaxPlayersPerParty=20 ; max players") {
		t.Errorf("semicolon delimiter not preserved:\n%s", out)
	}
	if !strings.Contains(out, "NoComment=new") {
		t.Errorf("no-comment edit not applied:\n%s", out)
	}
}

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

// directorFakeControl is a ControlPlane that also implements directorConfigStore.
type directorFakeControl struct {
	stubControlPlane
	iniPath    string
	iniContent string
	iniErr     error
}

func (f *directorFakeControl) readDirectorConfig(_ Executor) (string, string, error) {
	return f.iniPath, f.iniContent, f.iniErr
}

func (f *directorFakeControl) writeDirectorConfig(_ Executor, content string) (string, error) {
	f.iniContent = content
	return f.iniPath, f.iniErr
}

func TestResolveDirectorStore_NilControl(t *testing.T) {
	_, ok := resolveDirectorStore(nil)
	if ok {
		t.Error("expected (nil, false) for nil control")
	}
}

func TestResolveDirectorStore_NoInterface(t *testing.T) {
	_, ok := resolveDirectorStore(&stubControlPlane{})
	if ok {
		t.Error("expected (nil, false) for control that does not implement directorConfigStore")
	}
}

func TestResolveDirectorStore_ImplementsInterface(t *testing.T) {
	ctrl := &directorFakeControl{iniPath: "/some/path", iniContent: "[ Battlegroup ]\nFoo=1\n"}
	store, ok := resolveDirectorStore(ctrl)
	if !ok || store == nil {
		t.Fatal("expected (store, true) for directorFakeControl")
	}
}

func TestHandleGetDirectorConfig_CtxControlOverridesGlobal(t *testing.T) {
	origC, origE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	defer func() { globalControl, globalExecutor = origC, origE }()

	ctrl := &directorFakeControl{
		iniPath:    "/director.ini",
		iniContent: "[ Battlegroup ]\nDbFetchInterval=5\n",
	}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Control: ctrl, Executor: exec}
	reg.Register(sc)

	inner := http.HandlerFunc(handleGetDirectorConfig)
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/director-config", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusServiceUnavailable {
		t.Error("ctx control should prevent 503 when globalControl is nil")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
