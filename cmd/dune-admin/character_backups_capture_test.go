package main

import (
	"errors"
	"strings"
	"testing"
)

// TestProcessCaptureCharacterBackup exercises the native-transfer capture
// orchestration with injected deps — no DB or filesystem needed.
func TestProcessCaptureCharacterBackup(t *testing.T) {
	t.Parallel()

	t.Run("happy path: resolves FLS id, exports, writes file, records metadata", func(t *testing.T) {
		t.Parallel()
		var gotFLSID string
		var gotPath string
		var gotRecord characterBackup
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:     42,
			characterName: "Paul Atreides",
			action:        "manual",
			reason:        "pre-transfer safety net",
			resolveFLSID:  func(id int64) (string, error) { return "fls|1234", nil },
			exportCharacter: func(flsID string) (string, error) {
				gotFLSID = flsID
				return `{"_patches_checksum":"abc123","player_controller_id":99,"entries":[]}`, nil
			},
			writeFile: func(contents string) (string, error) {
				gotPath = "/backups/character-transfers/42-20260716-150405.json"
				return gotPath, nil
			},
			createRecord: func(b characterBackup) error {
				gotRecord = b
				return nil
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotFLSID != "fls|1234" {
			t.Fatalf("exportCharacter called with flsID=%q, want fls|1234", gotFLSID)
		}
		if gotRecord.AccountID != 42 || gotRecord.FLSID != "fls|1234" || gotRecord.CharacterName != "Paul Atreides" ||
			gotRecord.Action != "manual" || gotRecord.Reason != "pre-transfer safety net" ||
			gotRecord.FilePath != gotPath || gotRecord.PatchesChecksum != "abc123" {
			t.Fatalf("createRecord got %+v", gotRecord)
		}
	})

	t.Run("zero account id rejected", func(t *testing.T) {
		t.Parallel()
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       0,
			resolveFLSID:    func(int64) (string, error) { t.Error("resolveFLSID must not be called"); return "", nil },
			exportCharacter: func(string) (string, error) { t.Error("exportCharacter must not be called"); return "", nil },
			writeFile:       func(string) (string, error) { t.Error("writeFile must not be called"); return "", nil },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if err == nil {
			t.Fatal("expected error for zero account id")
		}
	})

	t.Run("resolveFLSID error propagates before export", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("no such account")
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "", boom },
			exportCharacter: func(string) (string, error) { t.Error("exportCharacter must not be called"); return "", nil },
			writeFile:       func(string) (string, error) { t.Error("writeFile must not be called"); return "", nil },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("empty resolved FLS id rejected", func(t *testing.T) {
		t.Parallel()
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "", nil },
			exportCharacter: func(string) (string, error) { t.Error("exportCharacter must not be called"); return "", nil },
			writeFile:       func(string) (string, error) { t.Error("writeFile must not be called"); return "", nil },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if err == nil {
			t.Fatal("expected error for empty resolved FLS id")
		}
	})

	t.Run("exportCharacter error propagates (e.g. player must be offline)", func(t *testing.T) {
		t.Parallel()
		boom := errors.New(`sbRF3$ - player_online: Player fls|1234 must be Offline`)
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "fls|1234", nil },
			exportCharacter: func(string) (string, error) { return "", boom },
			writeFile:       func(string) (string, error) { t.Error("writeFile must not be called"); return "", nil },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("malformed export JSON rejected before writing a file", func(t *testing.T) {
		t.Parallel()
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "fls|1234", nil },
			exportCharacter: func(string) (string, error) { return "not json", nil },
			writeFile:       func(string) (string, error) { t.Error("writeFile must not be called"); return "", nil },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if err == nil {
			t.Fatal("expected error for malformed export JSON")
		}
	})

	t.Run("writeFile error propagates before recording metadata", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("disk full")
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "fls|1234", nil },
			exportCharacter: func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			writeFile:       func(string) (string, error) { return "", boom },
			createRecord:    func(characterBackup) error { t.Error("createRecord must not be called"); return nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("createRecord error propagates", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("sqlite busy")
		err := processCaptureCharacterBackup(captureCharacterBackupParams{
			accountID:       42,
			resolveFLSID:    func(int64) (string, error) { return "fls|1234", nil },
			exportCharacter: func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			writeFile:       func(string) (string, error) { return "/path.json", nil },
			createRecord:    func(characterBackup) error { return boom },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})
}

// TestProcessRestoreCharacterBackup exercises the native-transfer restore
// orchestration with injected deps — no DB or filesystem needed.
func TestProcessRestoreCharacterBackup(t *testing.T) {
	t.Parallel()

	t.Run("happy path: loads backup, reads file, imports, invalidates cache", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", CharacterName: "Paul Atreides", FilePath: "/backups/42.json"}
		var gotPath string
		var gotData, gotFLSID, gotName string
		invalidated := false
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:  7,
			getBackup: func(id int64) (*characterBackup, error) { return backup, nil },
			readFile: func(path string) (string, error) {
				gotPath = path
				return `{"_patches_checksum":"abc"}`, nil
			},
			importCharacter: func(data, flsID, name string) (int64, error) {
				gotData, gotFLSID, gotName = data, flsID, name
				return 555, nil
			},
			invalidateCache: func() { invalidated = true },
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
		if gotPath != "/backups/42.json" {
			t.Fatalf("readFile called with path=%q", gotPath)
		}
		if gotData != `{"_patches_checksum":"abc"}` || gotFLSID != "fls|1234" || gotName != "Paul Atreides" {
			t.Fatalf("importCharacter called with data=%q flsID=%q name=%q", gotData, gotFLSID, gotName)
		}
		if !invalidated {
			t.Fatal("expected invalidateCache to be called on success")
		}
	})

	t.Run("getBackup error propagates before reading a file", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("not found")
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:  7,
			getBackup: func(int64) (*characterBackup, error) { return nil, boom },
			readFile:  func(string) (string, error) { t.Error("readFile must not be called"); return "", nil },
			importCharacter: func(string, string, string) (int64, error) {
				t.Error("importCharacter must not be called")
				return 0, nil
			},
			invalidateCache: func() { t.Error("invalidateCache must not be called") },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("readFile error propagates before import", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("file missing")
		backup := &characterBackup{ID: 7, FilePath: "/backups/42.json"}
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:  7,
			getBackup: func(int64) (*characterBackup, error) { return backup, nil },
			readFile:  func(string) (string, error) { return "", boom },
			importCharacter: func(string, string, string) (int64, error) {
				t.Error("importCharacter must not be called")
				return 0, nil
			},
			invalidateCache: func() { t.Error("invalidateCache must not be called") },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("importCharacter error propagates, cache not invalidated (e.g. checksum mismatch)", func(t *testing.T) {
		t.Parallel()
		boom := errors.New(`sb9R2$ - Patches checksum mismatch`)
		backup := &characterBackup{ID: 7, FilePath: "/backups/42.json"}
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:        7,
			getBackup:       func(int64) (*characterBackup, error) { return backup, nil },
			readFile:        func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			importCharacter: func(string, string, string) (int64, error) { return 0, boom },
			invalidateCache: func() { t.Error("invalidateCache must not be called") },
			cleanupOrphans:  func() error { t.Error("cleanupOrphans must not be called"); return nil },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("cleanupOrphans called after a successful import, before cache invalidation", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FilePath: "/backups/42.json"}
		var order []string
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:        7,
			getBackup:       func(int64) (*characterBackup, error) { return backup, nil },
			readFile:        func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			importCharacter: func(string, string, string) (int64, error) { order = append(order, "import"); return 555, nil },
			cleanupOrphans:  func() error { order = append(order, "cleanup"); return nil },
			invalidateCache: func() { order = append(order, "invalidate") },
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
		want := []string{"import", "cleanup", "invalidate"}
		if len(order) != len(want) {
			t.Fatalf("call order = %v, want %v", order, want)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Fatalf("call order = %v, want %v", order, want)
			}
		}
	})

	t.Run("cleanupOrphans error propagates, cache not invalidated", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("db down")
		backup := &characterBackup{ID: 7, FilePath: "/backups/42.json"}
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:        7,
			getBackup:       func(int64) (*characterBackup, error) { return backup, nil },
			readFile:        func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			importCharacter: func(string, string, string) (int64, error) { return 555, nil },
			cleanupOrphans:  func() error { return boom },
			invalidateCache: func() { t.Error("invalidateCache must not be called") },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("nil cleanupOrphans is fine", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FilePath: "/backups/42.json"}
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:        7,
			getBackup:       func(int64) (*characterBackup, error) { return backup, nil },
			readFile:        func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			importCharacter: func(string, string, string) (int64, error) { return 555, nil },
			invalidateCache: func() {},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
	})

	t.Run("resolveOldAccountID runs before import, cleanupOrphanActors runs after with that id", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", FilePath: "/backups/42.json"}
		var order []string
		var gotOldAccountID int64
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:  7,
			getBackup: func(int64) (*characterBackup, error) { return backup, nil },
			readFile:  func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			resolveOldAccountID: func(flsID string) (int64, error) {
				if flsID != "fls|1234" {
					t.Fatalf("resolveOldAccountID called with flsID=%q", flsID)
				}
				order = append(order, "resolve")
				return 31, nil
			},
			importCharacter: func(string, string, string) (int64, error) { order = append(order, "import"); return 555, nil },
			cleanupOrphanActors: func(oldAccountID int64) error {
				order = append(order, "cleanupActors")
				gotOldAccountID = oldAccountID
				return nil
			},
			invalidateCache: func() { order = append(order, "invalidate") },
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
		if gotOldAccountID != 31 {
			t.Fatalf("cleanupOrphanActors called with oldAccountID=%d, want 31", gotOldAccountID)
		}
		want := []string{"resolve", "import", "cleanupActors", "invalidate"}
		if len(order) != len(want) {
			t.Fatalf("call order = %v, want %v", order, want)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Fatalf("call order = %v, want %v", order, want)
			}
		}
	})

	t.Run("resolveOldAccountID error aborts before import", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("db down")
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", FilePath: "/backups/42.json"}
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:            7,
			getBackup:           func(int64) (*characterBackup, error) { return backup, nil },
			readFile:            func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			resolveOldAccountID: func(string) (int64, error) { return 0, boom },
			importCharacter: func(string, string, string) (int64, error) {
				t.Error("importCharacter must not be called")
				return 0, nil
			},
			cleanupOrphanActors: func(int64) error { t.Error("cleanupOrphanActors must not be called"); return nil },
			invalidateCache:     func() { t.Error("invalidateCache must not be called") },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("zero old account id (no prior character) skips cleanupOrphanActors", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", FilePath: "/backups/42.json"}
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:            7,
			getBackup:           func(int64) (*characterBackup, error) { return backup, nil },
			readFile:            func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			resolveOldAccountID: func(string) (int64, error) { return 0, nil },
			importCharacter:     func(string, string, string) (int64, error) { return 555, nil },
			cleanupOrphanActors: func(int64) error {
				t.Error("cleanupOrphanActors must not be called for a zero old account id")
				return nil
			},
			invalidateCache: func() {},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
	})

	t.Run("cleanupOrphanActors error propagates, cache not invalidated", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("actor delete failed")
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", FilePath: "/backups/42.json"}
		_, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:            7,
			getBackup:           func(int64) (*characterBackup, error) { return backup, nil },
			readFile:            func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			resolveOldAccountID: func(string) (int64, error) { return 31, nil },
			importCharacter:     func(string, string, string) (int64, error) { return 555, nil },
			cleanupOrphanActors: func(int64) error { return boom },
			invalidateCache:     func() { t.Error("invalidateCache must not be called") },
		})
		if !errors.Is(err, boom) {
			t.Fatalf("want boom, got %v", err)
		}
	})

	t.Run("nil resolveOldAccountID and nil cleanupOrphanActors are fine", func(t *testing.T) {
		t.Parallel()
		backup := &characterBackup{ID: 7, FLSID: "fls|1234", FilePath: "/backups/42.json"}
		newID, err := processRestoreCharacterBackup(restoreCharacterBackupParams{
			backupID:        7,
			getBackup:       func(int64) (*characterBackup, error) { return backup, nil },
			readFile:        func(string) (string, error) { return `{"_patches_checksum":"abc"}`, nil },
			importCharacter: func(string, string, string) (int64, error) { return 555, nil },
			invalidateCache: func() {},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if newID != 555 {
			t.Fatalf("newID = %d, want 555", newID)
		}
	})
}

// TestOrphanPlayerActorsDeleteSQL_ScopedToPlayerActorClasses pins the safety
// property of the post-restore orphan-actor cleanup: it may only ever delete
// the leaked player actor trio (character/controller/player-state), NEVER the
// owner's surviving property. dune.delete_account strips ownership/permission
// ranks from bases, storage, totems, and vehicles but leaves their actor rows
// alive with a dangling owner_account_id — an unscoped delete keyed on
// owner_account_id alone would destroy all of it, inventories included.
func TestOrphanPlayerActorsDeleteSQL_ScopedToPlayerActorClasses(t *testing.T) {
	t.Parallel()
	for _, want := range []string{
		"owner_account_id = $1",
		"PlayerCharacter",
		"PlayerController",
		"PlayerState",
		"class ILIKE",
	} {
		if !strings.Contains(orphanPlayerActorsDeleteSQL, want) {
			t.Fatalf("orphanPlayerActorsDeleteSQL missing %q:\n%s", want, orphanPlayerActorsDeleteSQL)
		}
	}
}

func TestCharacterTransferChecksum(t *testing.T) {
	t.Parallel()

	t.Run("extracts checksum field", func(t *testing.T) {
		t.Parallel()
		got, err := characterTransferChecksum(`{"_patches_checksum":"abc123","entries":[]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "abc123" {
			t.Fatalf("checksum = %q, want abc123", got)
		}
	})

	t.Run("malformed JSON returns an error", func(t *testing.T) {
		t.Parallel()
		if _, err := characterTransferChecksum("not json"); err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("missing checksum field returns empty string, no error", func(t *testing.T) {
		t.Parallel()
		got, err := characterTransferChecksum(`{"entries":[]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("checksum = %q, want empty", got)
		}
	})
}
