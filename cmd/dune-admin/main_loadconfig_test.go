package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStaleEnvFile(t *testing.T) {
	t.Parallel()

	t.Run("no .env file returns false", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if detectStaleEnvFile(dir) {
			t.Fatal("expected false when no .env exists")
		}
	})

	t.Run("present .env file returns true", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=old-host\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if !detectStaleEnvFile(dir) {
			t.Fatal("expected true when .env exists alongside config.yaml")
		}
	})

	t.Run("unreadable directory returns false safely", func(t *testing.T) {
		t.Parallel()
		if detectStaleEnvFile("/nonexistent-path-that-cannot-exist") {
			t.Fatal("expected false for nonexistent directory")
		}
	})
}
