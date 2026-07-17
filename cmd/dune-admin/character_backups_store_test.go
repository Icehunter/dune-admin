package main

import "testing"

func testCharacterBackupsStore(t *testing.T) *characterBackupsStore {
	t.Helper()
	s, err := openCharacterBackupsStore(":memory:")
	if err != nil {
		t.Fatalf("openCharacterBackupsStore: %v", err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}

func TestCharacterBackupsStore_CreateAndGet(t *testing.T) {
	s := testCharacterBackupsStore(t)

	created, err := s.create(characterBackup{
		AccountID:       42,
		FLSID:           "fls|1234",
		CharacterName:   "Paul Atreides",
		Action:          "delete_character",
		Reason:          "ban evasion",
		FilePath:        "/backups/character-transfers/42-20260716-150405.json",
		PatchesChecksum: "abc123",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id after create")
	}
	if created.CreatedAt == "" {
		t.Error("expected created_at to be stamped")
	}

	got, err := s.get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccountID != 42 || got.FLSID != "fls|1234" || got.CharacterName != "Paul Atreides" ||
		got.Action != "delete_character" || got.Reason != "ban evasion" ||
		got.FilePath != "/backups/character-transfers/42-20260716-150405.json" || got.PatchesChecksum != "abc123" {
		t.Errorf("get returned %+v, want the created row", got)
	}
}

func TestCharacterBackupsStore_GetMissingReturnsErrNotFound(t *testing.T) {
	s := testCharacterBackupsStore(t)
	_, err := s.get(999)
	if err != errNotFound {
		t.Fatalf("get missing = %v, want errNotFound", err)
	}
}

func TestCharacterBackupsStore_ListOrdersNewestFirst(t *testing.T) {
	s := testCharacterBackupsStore(t)

	first, err := s.create(characterBackup{AccountID: 1, Action: "delete_character", FilePath: "/a"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := s.create(characterBackup{AccountID: 2, Action: "manual", FilePath: "/b"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := s.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("list order = [%d, %d], want newest-first [%d, %d]", list[0].ID, list[1].ID, second.ID, first.ID)
	}
}

func TestCharacterBackupsStore_ListForAccountFiltersAndOrdersNewestFirst(t *testing.T) {
	s := testCharacterBackupsStore(t)

	if _, err := s.create(characterBackup{AccountID: 1, Action: "delete_character", FilePath: "/a"}); err != nil {
		t.Fatalf("create for account 1: %v", err)
	}
	firstForTwo, err := s.create(characterBackup{AccountID: 2, Action: "manual", FilePath: "/b"})
	if err != nil {
		t.Fatalf("create first for account 2: %v", err)
	}
	secondForTwo, err := s.create(characterBackup{AccountID: 2, Action: "manual", FilePath: "/c"})
	if err != nil {
		t.Fatalf("create second for account 2: %v", err)
	}

	list, err := s.listForAccount(2)
	if err != nil {
		t.Fatalf("listForAccount: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("listForAccount len = %d, want 2", len(list))
	}
	if list[0].ID != secondForTwo.ID || list[1].ID != firstForTwo.ID {
		t.Errorf("listForAccount order = [%d, %d], want newest-first [%d, %d]", list[0].ID, list[1].ID, secondForTwo.ID, firstForTwo.ID)
	}
	for _, b := range list {
		if b.AccountID != 2 {
			t.Errorf("listForAccount returned a row for account %d, want only account 2", b.AccountID)
		}
	}
}

func TestCharacterBackupsStore_WithScopeIsolatesServers(t *testing.T) {
	s := testCharacterBackupsStore(t)
	serverA := s.withScope(1)
	serverB := s.withScope(2)

	if _, err := serverA.create(characterBackup{AccountID: 1, Action: "delete_character", FilePath: "/a"}); err != nil {
		t.Fatalf("create on server A: %v", err)
	}

	listA, err := serverA.list()
	if err != nil {
		t.Fatalf("list server A: %v", err)
	}
	if len(listA) != 1 {
		t.Fatalf("server A list len = %d, want 1", len(listA))
	}

	listB, err := serverB.list()
	if err != nil {
		t.Fatalf("list server B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("server B list len = %d, want 0 (must not see server A's backups)", len(listB))
	}
}

func TestCharacterBackupsStore_ListEmptyIsEmptySliceNotNil(t *testing.T) {
	s := testCharacterBackupsStore(t)
	list, err := s.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list == nil {
		t.Error("list() returned nil, want an empty (non-nil) slice")
	}
	if len(list) != 0 {
		t.Errorf("list len = %d, want 0", len(list))
	}
}

func TestCharacterBackupsStore_DeleteRemovesRow(t *testing.T) {
	s := testCharacterBackupsStore(t)
	created, err := s.create(characterBackup{AccountID: 1, Action: "manual", FilePath: "/a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := s.get(created.ID); err != errNotFound {
		t.Fatalf("get after delete = %v, want errNotFound", err)
	}
}

func TestCharacterBackupsStore_DeleteMissingReturnsErrNotFound(t *testing.T) {
	s := testCharacterBackupsStore(t)
	if err := s.delete(999); err != errNotFound {
		t.Fatalf("delete missing = %v, want errNotFound", err)
	}
}

func TestCharacterBackupsStore_DeleteScopedToServer(t *testing.T) {
	s := testCharacterBackupsStore(t)
	serverA := s.withScope(1)
	serverB := s.withScope(2)

	created, err := serverA.create(characterBackup{AccountID: 1, Action: "manual", FilePath: "/a"})
	if err != nil {
		t.Fatalf("create on server A: %v", err)
	}

	if err := serverB.delete(created.ID); err != errNotFound {
		t.Fatalf("delete from server B = %v, want errNotFound (must not delete server A's row)", err)
	}
	if _, err := serverA.get(created.ID); err != nil {
		t.Fatalf("server A row should still exist: %v", err)
	}
}
