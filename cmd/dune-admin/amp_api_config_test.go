package main

import (
	"errors"
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
// through the /api/v1/config GET endpoint.
func TestMaskSecrets_MasksAmpAPIPass(t *testing.T) {
	t.Parallel()
	cfg := appConfig{AmpAPIPass: "secret"}
	maskSecrets(&cfg)
	if cfg.AmpAPIPass != masked {
		t.Errorf("AmpAPIPass = %q, want masked", cfg.AmpAPIPass)
	}
	// An empty password stays empty (not masked) so the UI shows "unset".
	empty := appConfig{}
	maskSecrets(&empty)
	if empty.AmpAPIPass != "" {
		t.Errorf("empty AmpAPIPass = %q, want empty", empty.AmpAPIPass)
	}
}

// TestPreserveMaskedSecrets_RestoresAmpAPIPass verifies that when the client
// posts back the masked placeholder, the stored AMP API password is restored
// (here from the in-memory loadedConfig fallback when the file is unreadable).
func TestPreserveMaskedSecrets_RestoresAmpAPIPass(t *testing.T) {
	orig := loadedConfig
	t.Cleanup(func() { loadedConfig = orig })
	loadedConfig = appConfig{AmpAPIPass: "stored-amp-pass"}

	cfg := appConfig{AmpAPIPass: masked}
	preserveMaskedSecrets(&cfg, func(string) ([]byte, error) { return nil, errors.New("no file") }, "ignored")
	if cfg.AmpAPIPass != "stored-amp-pass" {
		t.Errorf("AmpAPIPass = %q, want restored stored-amp-pass", cfg.AmpAPIPass)
	}
}

// TestPreserveMaskedSecrets_KeepsExplicitAmpAPIPass verifies an explicitly-set
// (non-masked) password is written through unchanged.
func TestPreserveMaskedSecrets_KeepsExplicitAmpAPIPass(t *testing.T) {
	orig := loadedConfig
	t.Cleanup(func() { loadedConfig = orig })
	loadedConfig = appConfig{AmpAPIPass: "stored"}

	cfg := appConfig{AmpAPIPass: "new-pass"}
	preserveMaskedSecrets(&cfg, func(string) ([]byte, error) { return nil, errors.New("no file") }, "ignored")
	if cfg.AmpAPIPass != "new-pass" {
		t.Errorf("AmpAPIPass = %q, want new-pass (explicit value preserved)", cfg.AmpAPIPass)
	}
}
