package main

// Phase 4 — Red tests for cmd* pool parameterisation.
// Each test calls a cmd* function with an explicit nil pool and verifies that
// it returns an error Msg rather than panicking. These tests fail to compile
// until the corresponding pool param is added to each function.

import "testing"

// ── Players-tab commands ──────────────────────────────────────────────────────

func TestCmdFetchPlayers_NilPool(t *testing.T) {
	msg, ok := cmdFetchPlayers(nil).(msgPlayers)
	if !ok {
		t.Fatal("expected msgPlayers")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchInventory_NilPool(t *testing.T) {
	msg, ok := cmdFetchInventory(nil, 1)().(msgInventory)
	if !ok {
		t.Fatal("expected msgInventory")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchCurrency_NilPool(t *testing.T) {
	msg, ok := cmdFetchCurrency(nil).(msgCurrency)
	if !ok {
		t.Fatal("expected msgCurrency")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchFactions_NilPool(t *testing.T) {
	msg, ok := cmdFetchFactions(nil).(msgFactions)
	if !ok {
		t.Fatal("expected msgFactions")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchSpecs_NilPool(t *testing.T) {
	msg, ok := cmdFetchSpecs(nil).(msgSpecs)
	if !ok {
		t.Fatal("expected msgSpecs")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGiveItem_NilPool(t *testing.T) {
	msg, ok := cmdGiveItem(nil, 1, "template", 1, 0)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGrantAllKeystones_NilPool(t *testing.T) {
	msg, ok := cmdGrantAllKeystones(nil, 1)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdResetAllKeystones_NilPool(t *testing.T) {
	msg, ok := cmdResetAllKeystones(nil, 1)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchPlayerKeystones_NilPool(t *testing.T) {
	msg, ok := cmdFetchPlayerKeystones(nil, 1)().(msgKeystones)
	if !ok {
		t.Fatal("expected msgKeystones")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGetPlayerVehicles_NilPool(t *testing.T) {
	msg, ok := cmdGetPlayerVehicles(nil, 1)().(msgVehicles)
	if !ok {
		t.Fatal("expected msgVehicles")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdRepairItem_NilPool(t *testing.T) {
	msg, ok := cmdRepairItem(nil, 1)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdRepairPlayerGear_NilPool(t *testing.T) {
	msg, ok := cmdRepairPlayerGear(nil, 1)().(msgRepairGear)
	if !ok {
		t.Fatal("expected msgRepairGear")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdListPartitions_NilPool(t *testing.T) {
	msg, ok := cmdListPartitions(nil)().(msgPartitions)
	if !ok {
		t.Fatal("expected msgPartitions")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdTeleportPlayer_NilPool(t *testing.T) {
	msg, ok := cmdTeleportPlayer(nil, "player", "spawn")().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGetPlayerPosition_NilPool(t *testing.T) {
	msg, ok := cmdGetPlayerPosition(nil, 1)().(msgPlayerPosition)
	if !ok {
		t.Fatal("expected msgPlayerPosition")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdTeleportPlayerToCoords_NilPool(t *testing.T) {
	msg, ok := cmdTeleportPlayerToCoords(nil, "fls", 1, 0, 0, 0)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchEventLog_NilPool(t *testing.T) {
	msg, ok := cmdFetchEventLog(nil, 1)().(msgEvents)
	if !ok {
		t.Fatal("expected msgEvents")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchPlayerDungeons_NilPool(t *testing.T) {
	msg, ok := cmdFetchPlayerDungeons(nil, 1)().(msgDungeons)
	if !ok {
		t.Fatal("expected msgDungeons")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchCheatLog_NilPool(t *testing.T) {
	msg, ok := cmdFetchCheatLog(nil)().(msgCheatLog)
	if !ok {
		t.Fatal("expected msgCheatLog")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Database-tab commands ─────────────────────────────────────────────────────

func TestCmdFetchTables_NilPool(t *testing.T) {
	msg, ok := cmdFetchTables(nil).(msgTables)
	if !ok {
		t.Fatal("expected msgTables")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdDescribeTable_NilPool(t *testing.T) {
	msg, ok := cmdDescribeTable(nil, "foos")().(msgDescribe)
	if !ok {
		t.Fatal("expected msgDescribe")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdSampleTable_NilPool(t *testing.T) {
	msg, ok := cmdSampleTable(nil, "foos", 10)().(msgSample)
	if !ok {
		t.Fatal("expected msgSample")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdSearchColumns_NilPool(t *testing.T) {
	msg, ok := cmdSearchColumns(nil, "foo")().(msgSearchCols)
	if !ok {
		t.Fatal("expected msgSearchCols")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdRunSQL_NilPool(t *testing.T) {
	msg, ok := cmdRunSQL(nil, "SELECT 1")().(msgSQL)
	if !ok {
		t.Fatal("expected msgSQL")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Storage-tab commands ──────────────────────────────────────────────────────

func TestCmdListStorageContainers_NilPool(t *testing.T) {
	msg, ok := cmdListStorageContainers(nil).(msgStorageContainers)
	if !ok {
		t.Fatal("expected msgStorageContainers")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGetContainerInventory_NilPool(t *testing.T) {
	msg, ok := cmdGetContainerInventory(nil, 1)().(msgContainerInventory)
	if !ok {
		t.Fatal("expected msgContainerInventory")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdGiveItemToContainer_NilPool(t *testing.T) {
	msg, ok := cmdGiveItemToContainer(nil, 1, "t", 1, 0)().(msgMutate)
	if !ok {
		t.Fatal("expected msgMutate")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Bases-tab commands ────────────────────────────────────────────────────────

func TestCmdListBases_NilPool(t *testing.T) {
	msg, ok := cmdListBases(nil).(msgBaseList)
	if !ok {
		t.Fatal("expected msgBaseList")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Blueprints-tab commands ───────────────────────────────────────────────────

func TestCmdListBlueprints_NilPool(t *testing.T) {
	msg, ok := cmdListBlueprints(nil).(msgBlueprintList)
	if !ok {
		t.Fatal("expected msgBlueprintList")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Market-tab commands ───────────────────────────────────────────────────────

func TestCmdFetchMarketItems_NilPool(t *testing.T) {
	msg, ok := cmdFetchMarketItems(nil).(msgMarketItems)
	if !ok {
		t.Fatal("expected msgMarketItems")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchMarketListings_NilPool(t *testing.T) {
	msg, ok := cmdFetchMarketListings(nil, "tmpl").(msgMarketListings)
	if !ok {
		t.Fatal("expected msgMarketListings")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchMarketSales_NilPool(t *testing.T) {
	msg, ok := cmdFetchMarketSales(nil).(msgMarketSales)
	if !ok {
		t.Fatal("expected msgMarketSales")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestCmdFetchMarketStats_NilPool(t *testing.T) {
	msg, ok := cmdFetchMarketStats(nil).(msgMarketStats)
	if !ok {
		t.Fatal("expected msgMarketStats")
	}
	if msg.err == nil {
		t.Error("expected error for nil pool")
	}
}
