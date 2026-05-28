package main

import (
	"errors"
	"testing"
)

func TestPreserveMaskedDBPass(t *testing.T) {
	t.Parallel()

	t.Run("keeps explicit password", func(t *testing.T) {
		cfg := appConfig{DBPass: "new-pass"}
		preserveMaskedDBPass(&cfg, func(string) ([]byte, error) {
			t.Fatalf("readFile should not be called for explicit password")
			return nil, nil
		}, "/tmp/unused", "fallback")
		if cfg.DBPass != "new-pass" {
			t.Fatalf("expected explicit password to stay unchanged, got %q", cfg.DBPass)
		}
	})

	t.Run("uses existing config password", func(t *testing.T) {
		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedDBPass(&cfg, func(string) ([]byte, error) {
			return []byte("db_pass: stored-pass\n"), nil
		}, "/tmp/config.yaml", "fallback")
		if cfg.DBPass != "stored-pass" {
			t.Fatalf("expected stored password from config file, got %q", cfg.DBPass)
		}
	})

	t.Run("falls back to in-memory password", func(t *testing.T) {
		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedDBPass(&cfg, func(string) ([]byte, error) {
			return nil, errors.New("no file")
		}, "/tmp/missing.yaml", "fallback")
		if cfg.DBPass != "fallback" {
			t.Fatalf("expected fallback password, got %q", cfg.DBPass)
		}
	})
}
