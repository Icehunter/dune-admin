package main

import (
	"archive/zip"
	"bytes"
	"testing"
)

type testMultipartFile struct {
	*bytes.Reader
}

func (f testMultipartFile) Close() error { return nil }

func TestAllowedBackupArchiveEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entryName string
		wantName  string
		wantOK    bool
	}{
		{name: "backup", entryName: "save.backup", wantName: "save.backup", wantOK: true},
		{name: "backup-yaml", entryName: "save.backup.yaml", wantName: "save.backup.yaml", wantOK: true},
		{name: "nested-path", entryName: "dir/sub/save.backup", wantName: "save.backup", wantOK: true},
		{name: "non-backup", entryName: "notes.txt", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotName, gotOK := allowedBackupArchiveEntry(tt.entryName)
			if gotOK != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, gotOK)
			}
			if gotName != tt.wantName {
				t.Fatalf("expected name %q, got %q", tt.wantName, gotName)
			}
		})
	}
}

func TestIsDirectBackupUpload(t *testing.T) {
	t.Parallel()

	if !isDirectBackupUpload("save.backup") {
		t.Fatal("expected plain .backup filename to be accepted")
	}
	if isDirectBackupUpload("save.zip") {
		t.Fatal("expected .zip to be rejected for direct backup upload")
	}
	if isDirectBackupUpload("../save.backup") {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestUploadBackupArchive_InvalidZip(t *testing.T) {
	t.Parallel()

	file := testMultipartFile{Reader: bytes.NewReader([]byte("not a zip"))}
	_, status, err := uploadBackupArchive("/unused", file)
	if err == nil {
		t.Fatal("expected invalid zip error")
	}
	if status != 400 {
		t.Fatalf("expected status 400, got %d", status)
	}
}

func TestUploadBackupArchive_NoBackupFile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("notes.txt")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	file := testMultipartFile{Reader: bytes.NewReader(buf.Bytes())}
	_, status, err := uploadBackupArchive("/unused", file)
	if err == nil || err.Error() != "zip contains no .backup file" {
		t.Fatalf("expected no-backup error, got %v", err)
	}
	if status != 400 {
		t.Fatalf("expected status 400, got %d", status)
	}
}
