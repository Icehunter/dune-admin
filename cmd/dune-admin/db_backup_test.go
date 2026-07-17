package main

import (
	"errors"
	"strings"
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

func TestClassifyPgRestoreResult(t *testing.T) {
	t.Parallel()

	completedWithIgnored := `pg_restore: error: could not execute query: ERROR:  cannot drop inherited constraint "event_log_p9_pkey" of relation "event_log_p9"
Command was: ALTER TABLE IF EXISTS ONLY dune.event_log_p9 DROP CONSTRAINT IF EXISTS event_log_p9_pkey;
pg_restore: error: could not execute query: ERROR:  must be owner of extension pgcrypto
pg_restore: warning: errors ignored on restore: 38`

	tests := []struct {
		name        string
		out         string
		err         error
		wantIgnored int
		wantErr     bool
		wantInErr   string
	}{
		{
			name: "exit 0 clean success",
			out:  "", err: nil,
			wantIgnored: 0, wantErr: false,
		},
		{
			name: "exit 0 with summary line still parses count",
			out:  "pg_restore: warning: errors ignored on restore: 3", err: nil,
			wantIgnored: 3, wantErr: false,
		},
		{
			name: "exit 1 but ran to completion (summary line present) is success",
			out:  completedWithIgnored, err: errTestExit1,
			wantIgnored: 38, wantErr: false,
		},
		{
			name: "exit 1 without summary line is a real failure with output tail",
			out:  "pg_restore: error: could not connect to server: Connection refused", err: errTestExit1,
			wantErr: true, wantInErr: "Connection refused",
		},
		{
			name: "exit 1 with empty output is a real failure",
			out:  "", err: errTestExit1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ignored, err := classifyPgRestoreResult(tt.out, tt.err)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if tt.wantInErr != "" && !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantInErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ignored != tt.wantIgnored {
				t.Fatalf("ignored = %d, want %d", ignored, tt.wantIgnored)
			}
		})
	}
}

// TestClassifyPgRestoreResult_TailTruncation verifies a huge output is
// truncated to a bounded tail in the error message, keeping the most recent
// (most relevant) lines.
func TestClassifyPgRestoreResult_TailTruncation(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 5000) + "\nFINAL_LINE_MARKER"
	_, err := classifyPgRestoreResult(long, errTestExit1)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "FINAL_LINE_MARKER") {
		t.Fatal("error should keep the tail of the output")
	}
	if len(err.Error()) > 1000 {
		t.Fatalf("error message too long (%d chars) — tail not truncated", len(err.Error()))
	}
}

var errTestExit1 = errors.New("exit status 1")

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
