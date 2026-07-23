package main

import (
	"path/filepath"
	"testing"
)

// saveWebInterfaces must preserve the NoProxy flag through its {Label,URL}
// rebuild, and loadWebInterfaces must read it back unchanged. (#261 — the
// rebuild previously dropped any field not explicitly copied, e.g. Target.)
func TestSaveWebInterfaces_RoundTripsNoProxy(t *testing.T) {
	prevPath := webIfacePath
	prevIfaces := webIfaces
	prevLoaded := webIfaceLoaded
	t.Cleanup(func() {
		webIfacePath = prevPath
		webIfaces = prevIfaces
		webIfaceLoaded = prevLoaded
	})
	webIfacePath = filepath.Join(t.TempDir(), "web-interfaces.json")

	in := []webInterface{
		{Label: "Panel", URL: "https://panel.example/", NoProxy: true},
		{Label: "AMP", URL: "http://host:8080"}, // default NoProxy=false, unaffected
	}
	if err := saveWebInterfaces(in); err != nil {
		t.Fatalf("saveWebInterfaces: %v", err)
	}

	// Force a reload from disk (not just the in-memory cache saveWebInterfaces
	// also updates) to prove the flag survives the JSON round-trip.
	webIfaceLoaded = false
	got := getWebInterfaces()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if !got[0].NoProxy {
		t.Errorf("Panel.NoProxy = false after round-trip, want true: %+v", got[0])
	}
	if got[1].NoProxy {
		t.Errorf("AMP.NoProxy = true after round-trip, want false: %+v", got[1])
	}
}

func TestValidateWebInterfaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []webInterface
		ok   bool
	}{
		{"valid mix", []webInterface{{Label: "Director", URL: "/director/"}, {Label: "AMP", URL: "http://host:8080"}}, true},
		{"https ok", []webInterface{{Label: "Panel", URL: "https://example.com/x"}}, true},
		{"empty list ok", []webInterface{}, true},
		{"empty label", []webInterface{{Label: "", URL: "/x"}}, false},
		{"empty url", []webInterface{{Label: "X", URL: ""}}, false},
		{"javascript scheme rejected", []webInterface{{Label: "X", URL: "javascript:alert(1)"}}, false},
		{"bare host rejected", []webInterface{{Label: "X", URL: "host:8080"}}, false},
		{"ftp rejected", []webInterface{{Label: "X", URL: "ftp://h/x"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebInterfaces(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("validateWebInterfaces(%v) = %v, want nil", tt.in, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("validateWebInterfaces(%v) = nil, want error", tt.in)
			}
		})
	}

	// too many entries rejected
	many := make([]webInterface, maxWebInterfaces+1)
	for i := range many {
		many[i] = webInterface{Label: "L", URL: "/x"}
	}
	if validateWebInterfaces(many) == nil {
		t.Fatalf("expected error for > %d entries", maxWebInterfaces)
	}
}
