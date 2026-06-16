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
