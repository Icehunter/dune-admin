package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupFileOnce(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dune-admin.db")
	dst := src + ".pre-migrate.bak"
	if err := os.WriteFile(src, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First call snapshots the source.
	backupFileOnce(src)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("backup content = %q, want %q", got, "original")
	}

	// Source changes, then a second call must NOT overwrite the pristine backup.
	if err := os.WriteFile(src, []byte("migrated"), 0o600); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	backupFileOnce(src)
	got, _ = os.ReadFile(dst)
	if string(got) != "original" {
		t.Errorf("backup overwritten = %q, want pristine %q", got, "original")
	}
}

func TestBackupFileOnce_MissingSourceIsNoop(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.db")
	backupFileOnce(src) // must not panic or create anything
	if _, err := os.Stat(src + ".pre-migrate.bak"); !os.IsNotExist(err) {
		t.Error("backup created for a missing source")
	}
}

func TestBackupFileOnce_InMemoryIsNoop(t *testing.T) {
	backupFileOnce(":memory:") // must not attempt a file copy
	if _, err := os.Stat(":memory:.pre-migrate.bak"); err == nil {
		t.Error(":memory: should never be backed up")
		_ = os.Remove(":memory:.pre-migrate.bak")
	}
}
