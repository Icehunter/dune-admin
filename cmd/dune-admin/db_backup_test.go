package main

import (
	"testing"
	"time"
)

func TestValidateBackupName(t *testing.T) {
	t.Parallel()
	good := []string{"dune-20260608-221700.dump", "a.dump", "BG_1.backup.dump"}
	for _, n := range good {
		if err := validateBackupName(n); err != nil {
			t.Errorf("validateBackupName(%q) = %v, want nil", n, err)
		}
	}
	bad := []string{
		"",                   // empty
		"foo.txt",            // wrong ext
		"foo.dump.exe",       // wrong ext
		"../etc/passwd.dump", // traversal
		"a/b.dump",           // path sep
		"a\\b.dump",          // win path sep
		"foo .dump",          // space
		"foo;rm.dump",        // shell metachar
		".dump",              // no stem
	}
	for _, n := range bad {
		if err := validateBackupName(n); err == nil {
			t.Errorf("validateBackupName(%q) = nil, want error", n)
		}
	}
}

func TestBackupsToPrune(t *testing.T) {
	t.Parallel()
	names := []string{"d5.dump", "d4.dump", "d3.dump", "d2.dump", "d1.dump"} // newest-first

	tests := []struct {
		name  string
		keepN int
		want  []string
	}{
		{"keep 3 prunes oldest 2", 3, []string{"d2.dump", "d1.dump"}},
		{"keep more than present prunes none", 10, nil},
		{"keep exactly present prunes none", 5, nil},
		{"keep 0 disables pruning", 0, nil},
		{"negative disables pruning", -1, nil},
		{"keep 1 prunes rest", 1, []string{"d4.dump", "d3.dump", "d2.dump", "d1.dump"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backupsToPrune(names, tt.keepN)
			if len(got) != len(tt.want) {
				t.Fatalf("backupsToPrune(keepN=%d) = %v, want %v", tt.keepN, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("backupsToPrune(keepN=%d) = %v, want %v", tt.keepN, got, tt.want)
				}
			}
		})
	}
}

func TestDBBackupFilename(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 8, 22, 17, 5, 0, time.UTC)
	got := dbBackupFilename(ts)
	want := "dune-20260608-221705.dump"
	if got != want {
		t.Fatalf("dbBackupFilename = %q, want %q", got, want)
	}
	if err := validateBackupName(got); err != nil {
		t.Fatalf("generated name failed validation: %v", err)
	}
}
