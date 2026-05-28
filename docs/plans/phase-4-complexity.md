# Phase 4 — Cognitive Complexity Refactor

**Status:** Pending (blocked on Phase 3 merge)  
**Branch:** `refactor/phase-4-complexity` (to be created from Phase 3 branch)  
**PR base:** Phase 3 PR

---

## Scope

Resolve all 34 open cognitive-complexity issues (#18–51) by extracting helper functions until each violating function passes `gocognit -over 15`.

---

## Current violations (as of Phase 0 branch)

The following entries remain in `.gocognit-ignore` after Phase 0 removed `capture.go`:

| Function | File | Score | Issue |
|----------|------|-------|-------|
| `cmdGiveItem` | db.go | 109 | #18 |
| `handleExportBase` | handlers_bases.go | 53 | #19 |
| `cmdReverseContracts` | db.go | 51 | #20 |
| `cmdRepairPlayerGear` | db.go | 46 | #21 |
| `handleGetServerSettings` | handlers_server_settings.go | 41 | #22 |
| `importBlueprintData` | handlers_blueprints.go | 41 | #23 |
| `cmdCompleteContracts` | db.go | 38 | #24 |
| `handleBGBackupUpload` | handlers_battlegroup.go | 37 | #25 |
| `checkInventoryCapacity` | db.go | 37 | #26 |
| `handleMarketItems` | handlers_market.go | 34 | #27 |
| `cmdAwardCharXP` | db.go | 33 | #28 |
| `cmdProgressionUnlock` | db.go | 32 | #29 |
| `cmdReverseProgressionUnlock` | db.go | 32 | #30 |
| `main` | main.go | 30 | #31 |
| `handleUpdateServerSettings` | handlers_server_settings.go | 26 | #33 |
| `discoverDefaultINI` | handlers_server_settings.go | 26 | #34 |
| `cmdSetStarterClass` | db.go | 25 | #35 |
| `runKubectlSetup` | setup.go | 25 | #36 |
| `parseINILines` | handlers_server_settings.go | 24 | #37 |
| `cmdRepairVehicle` | db.go | 24 | #39 |
| `stripEmptySections` | handlers_server_settings.go | 22 | #40 |
| `handleGiveItems` | handlers_players.go | 22 | #41 |
| `fetchBlueprintData` | handlers_blueprints.go | 22 | #42 |
| `cmdApplyProgressionPreset` | progression_presets.go | 19 | #43 |
| `connectAll` | connection.go | 18 | #44 |
| `cmdRepairItem` | db.go | 18 | #45 |
| `listGameProcesses` | control_amp.go | 17 | #46 |
| `cmdRunSQL` | db.go | 17 | #47 |
| `stripKeysFromContent` | handlers_server_settings.go | 17 | #48 |
| `handleSaveConfig` | handlers_config.go | 16 | #49 |
| `cmdSampleTable` | db.go | 16 | #50 |
| `cmdGrantAllKeystones` | db.go | 16 | #51 |

---

## Execution plan

Run `make gocognit` first to confirm the list (line numbers shift after Phase 1 moved files).

Use the `/batch` skill to dispatch parallel agents grouped by file:

| Agent | Files | Issues |
|-------|-------|--------|
| 1 | `cmd/dune-admin/db.go` | #18, #20, #21, #24, #26, #28, #29, #30, #35, #39, #45, #47, #50, #51 |
| 2 | `cmd/dune-admin/handlers_server_settings.go` | #22, #33, #34, #37, #40, #48 |
| 3 | `cmd/dune-admin/handlers_*.go` (non-settings) | #19, #23, #25, #27, #41, #42, #49 |
| 4 | `cmd/dune-admin/main.go`, `connection.go`, `setup.go`, `control_amp.go`, `handlers_market.go` | #31, #36, #43, #44, #46 |

Each agent should:

1. Read the function body
2. Extract cohesive sub-operations as named helpers
3. Verify `make gocognit` passes for their group
4. Run `make test-race` to confirm no regressions

After all agents complete: `make verify` must pass clean, then remove resolved entries from `.gocognit-ignore`.

---

## Checklist

- [ ] `make gocognit` passes with empty output
- [ ] `.gocognit-ignore` is empty (or contains only intentional exceptions)
- [ ] `make verify` passes clean
- [ ] All #18–51 GitHub issues closed with PR reference
