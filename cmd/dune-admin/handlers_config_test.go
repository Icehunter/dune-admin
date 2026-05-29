package main

import (
	"errors"
	"testing"
)

// TestPreserveMaskedDBPass exercises the preserveMaskedSecrets function for the
// DBPass field specifically. Not parallel because subtests mutate loadedConfig.
func TestPreserveMaskedDBPass(t *testing.T) {
	t.Run("keeps explicit password", func(t *testing.T) {
		cfg := appConfig{DBPass: "new-pass"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			t.Fatalf("readFile should not be called for explicit password")
			return nil, nil
		}, "/tmp/unused")
		if cfg.DBPass != "new-pass" {
			t.Fatalf("expected explicit password to stay unchanged, got %q", cfg.DBPass)
		}
	})

	t.Run("uses existing config password from file", func(t *testing.T) {
		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return []byte("db_pass: stored-pass\n"), nil
		}, "/tmp/config.yaml")
		if cfg.DBPass != "stored-pass" {
			t.Fatalf("expected stored password from config file, got %q", cfg.DBPass)
		}
	})

	t.Run("falls back to loadedConfig when file missing", func(t *testing.T) {
		orig := loadedConfig
		loadedConfig = appConfig{DBPass: "in-memory-pass"}
		t.Cleanup(func() { loadedConfig = orig })

		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return nil, errors.New("no file")
		}, "/tmp/missing.yaml")
		if cfg.DBPass != "in-memory-pass" {
			t.Fatalf("expected in-memory fallback password, got %q", cfg.DBPass)
		}
	})
}
