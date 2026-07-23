package main

import (
	"testing"
)

// TestNewControlPlane_AMPWiresAPICredentials verifies the factory threads the
// AMP Web API credentials from config into the ampControl so the settings-write
// path can authenticate.
func TestNewControlPlane_AMPWiresAPICredentials(t *testing.T) {
	t.Parallel()
	cp := newControlPlane("amp", appConfig{
		AmpInstance: "DuneTest01",
		AmpAPIUser:  "admin",
		AmpAPIPass:  "test123!",
		AmpAPIPort:  9090,
	})
	amp, ok := cp.(*ampControl)
	if !ok {
		t.Fatalf("expected *ampControl, got %T", cp)
	}
	if amp.apiUser != "admin" || amp.apiPass != "test123!" || amp.apiPort != 9090 {
		t.Errorf("api creds = (%q,%q,%d), want (admin, test123!, 9090)", amp.apiUser, amp.apiPass, amp.apiPort)
	}
}

// TestNewControlPlane_AMPWiresAPIHost verifies the factory threads
// amp_api_host into ampControl.apiHost, so a split control-plane topology
// (issue #284) can point the AMP Web API call at a host other than the game
// host's own loopback.
func TestNewControlPlane_AMPWiresAPIHost(t *testing.T) {
	t.Parallel()
	cp := newControlPlane("amp", appConfig{
		AmpInstance: "DuneTest01",
		AmpAPIHost:  "10.0.0.5",
	})
	amp, ok := cp.(*ampControl)
	if !ok {
		t.Fatalf("expected *ampControl, got %T", cp)
	}
	if amp.apiHost != "10.0.0.5" {
		t.Errorf("apiHost = %q, want 10.0.0.5", amp.apiHost)
	}
}

// TestNewControlPlane_AMPAPIHostDefaultsEmpty verifies that omitting
// amp_api_host leaves ampControl.apiHost empty — endpoint() then falls back to
// the loopback default and post() keeps wrapping calls into the container, so
// existing (non-split) installs are unaffected.
func TestNewControlPlane_AMPAPIHostDefaultsEmpty(t *testing.T) {
	t.Parallel()
	cp := newControlPlane("amp", appConfig{AmpInstance: "DuneTest01"})
	amp, ok := cp.(*ampControl)
	if !ok {
		t.Fatalf("expected *ampControl, got %T", cp)
	}
	if amp.apiHost != "" {
		t.Errorf("apiHost = %q, want empty (defaults applied later by server_defaults.go)", amp.apiHost)
	}
}

// TestMaskSecrets_MasksAmpAPIPass ensures the AMP API password is never exposed
// through the /api/v1/config GET endpoint. AmpAPIPass is now a per-server secret,
// so masking happens on each Servers[] entry (via maskServerSecrets).
func TestMaskSecrets_MasksAmpAPIPass(t *testing.T) {
	t.Parallel()
	cfg := appConfig{Servers: []ServerConfig{{AmpAPIPass: "secret"}}}
	maskSecrets(&cfg)
	if cfg.Servers[0].AmpAPIPass != masked {
		t.Errorf("Servers[0].AmpAPIPass = %q, want masked", cfg.Servers[0].AmpAPIPass)
	}
	// An empty password stays empty (not masked) so the UI shows "unset".
	empty := appConfig{Servers: []ServerConfig{{}}}
	maskSecrets(&empty)
	if empty.Servers[0].AmpAPIPass != "" {
		t.Errorf("empty AmpAPIPass = %q, want empty", empty.Servers[0].AmpAPIPass)
	}
}
