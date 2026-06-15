package main

import "testing"

func TestServersStore_InsertAssignsAutoincrementID(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	id1, err := s.insertServer(ServerConfig{Name: "One", Control: "local"}, 0)
	if err != nil {
		t.Fatalf("insertServer: %v", err)
	}
	id2, err := s.insertServer(ServerConfig{Name: "Two", Control: "amp"}, 1)
	if err != nil {
		t.Fatalf("insertServer: %v", err)
	}
	if id1 != 1 || id2 != 2 {
		t.Errorf("ids = %d, %d; want 1, 2 (autoincrement)", id1, id2)
	}
}

func TestServersStore_ListStampsIDAndOrders(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	// Insert out of position order; listServers must return them by position.
	idB, _ := s.insertServer(ServerConfig{Name: "Beta"}, 5)
	idA, _ := s.insertServer(ServerConfig{Name: "Alpha"}, 1)

	list, err := s.listServers()
	if err != nil {
		t.Fatalf("listServers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].Name != "Alpha" || list[0].ID != idA {
		t.Errorf("list[0] = %+v, want Alpha id=%d", list[0], idA)
	}
	if list[1].Name != "Beta" || list[1].ID != idB {
		t.Errorf("list[1] = %+v, want Beta id=%d", list[1], idB)
	}
	// LegacyID must be cleared on read — the numeric id is authoritative.
	if list[0].LegacyID != "" {
		t.Errorf("LegacyID = %q, want empty after list", list[0].LegacyID)
	}
}

func TestServersStore_GetUpdateDelete(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	id, _ := s.insertServer(ServerConfig{Name: "One", Control: "local", DBName: "dune"}, 0)

	got, ok, err := s.getServer(id)
	if err != nil || !ok {
		t.Fatalf("getServer: ok=%v err=%v", ok, err)
	}
	if got.ID != id || got.DBName != "dune" {
		t.Errorf("getServer = %+v, want id=%d DBName=dune", got, id)
	}

	got.DBName = "newdb"
	got.Name = "Renamed"
	if err := s.updateServer(got); err != nil {
		t.Fatalf("updateServer: %v", err)
	}
	after, _, _ := s.getServer(id)
	if after.DBName != "newdb" || after.Name != "Renamed" {
		t.Errorf("after update = %+v, want DBName=newdb Name=Renamed", after)
	}

	if err := s.deleteServer(id); err != nil {
		t.Fatalf("deleteServer: %v", err)
	}
	if _, ok, _ := s.getServer(id); ok {
		t.Error("server still present after delete")
	}
}

func TestServersStore_HasAnyAndNextPosition(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	if has, _ := s.hasAnyServer(); has {
		t.Error("hasAnyServer = true on empty store")
	}
	if pos, _ := s.nextPosition(); pos != 0 {
		t.Errorf("nextPosition = %d on empty store, want 0", pos)
	}

	_, _ = s.insertServer(ServerConfig{Name: "One"}, 0)
	if has, _ := s.hasAnyServer(); !has {
		t.Error("hasAnyServer = false after insert")
	}
	if pos, _ := s.nextPosition(); pos != 1 {
		t.Errorf("nextPosition = %d after one insert, want 1", pos)
	}
}
