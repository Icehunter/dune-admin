package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// @Summary List all building blueprints
// @Tags blueprints
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /api/v1/blueprints [get]
func handleListBlueprints(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListBlueprints(dbFromCtx(r)).(msgBlueprintList)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	rows := msg.rows
	if rows == nil {
		rows = []blueprintRow{}
	}
	jsonOK(w, rows)
}

// @Summary Export a blueprint as a downloadable JSON file
// @Tags blueprints
// @Produce application/octet-stream
// @Param id path int true "Blueprint ID"
// @Success 200 {file} string "Blueprint JSON file"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/blueprints/{id}/export [get]
func handleExportBlueprint(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	bf, err := fetchBlueprintData(r.Context(), dbFromCtx(r), id)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, blueprintFilename(bf.Name, id)))
	_ = json.NewEncoder(w).Encode(bf)
}

// blueprintFilename returns the suggested download filename: the in-game name
// if present (sanitized), otherwise blueprint_<id>.json.
func blueprintFilename(name string, id int64) string {
	clean := sanitizeFilename(name)
	if clean == "" {
		return fmt.Sprintf("blueprint_%d.json", id)
	}
	return clean + ".json"
}

// sanitizeFilename strips characters that are unsafe in filenames or
// Content-Disposition values across common filesystems.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20, r == 0x7f:
			// drop control chars
		case r == '/', r == '\\', r == ':', r == '*', r == '?', r == '"', r == '<', r == '>', r == '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// @Summary Import a blueprint JSON file into a player's inventory
// @Tags blueprints
// @Accept multipart/form-data
// @Produce json
// @Param blueprint formData file true "Blueprint JSON file"
// @Param player_id formData int true "Player pawn ID to receive the blueprint"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/blueprints/import [post]
func handleImportBlueprint(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonErr(w, err, 400)
		return
	}
	playerIDStr := r.FormValue("player_id")
	playerID, err := strconv.ParseInt(playerIDStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid player_id"), 400)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, fmt.Errorf("file required"), 400)
		return
	}
	defer func() { _ = f.Close() }()

	var bf blueprintFile
	if err := json.NewDecoder(f).Decode(&bf); err != nil {
		jsonErr(w, fmt.Errorf("invalid blueprint JSON: %w", err), 400)
		return
	}
	if len(bf.Instances) == 0 && len(bf.Placeables) == 0 {
		jsonErr(w, fmt.Errorf("blueprint has no instances or placeables"), 400)
		return
	}

	msg, ok := importBlueprintData(r.Context(), dbFromCtx(r), playerID, bf).(msgMutate)
	if !ok {
		jsonErr(w, fmt.Errorf("internal error"), 500)
		return
	}
	if msg.err != nil {
		jsonErr(w, msg.err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": msg.ok})
}

// structuralBuildingTypes lists building_type values that game-saved blueprints
// commonly mark with provides_stability=true (foundations, pillars, columns).
// Used only as a fallback when importing legacy JSON that doesn't carry the
// per-instance flag; the game's structural solver actually picks a subset of
// these per build, so re-exported files always carry the exact bool.
var structuralBuildingTypes = map[string]bool{
	"Atreides_Outpost_Column":                  true,
	"Atreides_Outpost_Column_Corner":           true,
	"Atreides_Outpost_Foundation":              true,
	"Atreides_Outpost_Foundation_Round_Corner": true,
	"Atreides_Outpost_Foundation_Wedge":        true,
	"Atreides_Outpost_Pillar_Bottom":           true,
	"Atreides_Outpost_Pillar_Middle":           true,
	"Atreides_Outpost_Pillar_Top":              true,
	"Choam_Level2_Column":                      true,
	"Choam_Level2_Foundation":                  true,
	"Choam_Level2_Pillar_Bottom":               true,
	"Choam_Shelter_Column_Corner_New":          true,
	"Choam_Shelter_Column_New":                 true,
	"Harkonnen_Outpost_Column":                 true,
	"Harkonnen_Outpost_Foundation":             true,
	"MTX_Neut_DesertMechanic_Center_Column":    true,
	"MTX_Neut_DesertMechanic_Corner_Column":    true,
	"MTX_Neut_DesertMechanic_Foundation":       true,
	"MTX_Smug_Foundation":                      true,
}

func isStructuralBuilding(buildingType string) bool {
	return structuralBuildingTypes[buildingType]
}

func fetchBlueprintName(ctx context.Context, db *pgxpool.Pool, blueprintID int64) string {
	var name string
	_ = db.QueryRow(ctx, `
		SELECT COALESCE(i.stats->'FBuildingBlueprintItemStats'->1->>'BuildingBlueprintName', '')
		FROM dune.building_blueprints bb
		JOIN dune.items i ON i.id = bb.item_id
		WHERE bb.id = $1`, blueprintID).Scan(&name)
	return name
}

func buildBlueprintInstance(iid int, buildingType string, transform []float32, stability bool) (blueprintInstance, bool) {
	if len(transform) < 4 {
		return blueprintInstance{}, false
	}
	return blueprintInstance{
		InstanceID:        &iid,
		BuildingType:      buildingType,
		X:                 float64(transform[0]),
		Y:                 float64(transform[1]),
		Z:                 float64(transform[2]),
		Rotation:          float64(transform[3]),
		ProvidesStability: &stability,
	}, true
}

func fetchBlueprintInstances(ctx context.Context, db *pgxpool.Pool, blueprintID int64) ([]blueprintInstance, error) {
	rows, err := db.Query(ctx, `
		SELECT instance_id, building_type, transform, provides_stability
		FROM dune.building_blueprint_instances
		WHERE building_blueprint_id = $1
		ORDER BY instance_id`, blueprintID)
	if err != nil {
		return nil, fmt.Errorf("query instances: %w", err)
	}
	defer rows.Close()

	var instances []blueprintInstance
	for rows.Next() {
		var iid int
		var buildingType string
		var transform []float32
		var stability bool
		if err := rows.Scan(&iid, &buildingType, &transform, &stability); err != nil {
			continue
		}
		instance, ok := buildBlueprintInstance(iid, buildingType, transform, stability)
		if !ok {
			continue
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read instances: %w", err)
	}
	return instances, nil
}

func buildBlueprintPlaceable(pid int, buildingType string, transform []float32) (blueprintPlaceable, bool) {
	if len(transform) < 6 {
		return blueprintPlaceable{}, false
	}
	return blueprintPlaceable{
		PlaceableID:  &pid,
		BuildingType: buildingType,
		X:            float64(transform[0]),
		Y:            float64(transform[1]),
		Z:            float64(transform[2]),
		RX:           float64(transform[3]),
		RY:           float64(transform[4]),
		RZ:           float64(transform[5]),
	}, true
}

func fetchBlueprintPlaceables(ctx context.Context, db *pgxpool.Pool, blueprintID int64) ([]blueprintPlaceable, error) {
	rows, err := db.Query(ctx, `
		SELECT placeable_id, building_type, transform
		FROM dune.building_blueprint_placeables
		WHERE building_blueprint_id = $1
		ORDER BY placeable_id`, blueprintID)
	if err != nil {
		return nil, fmt.Errorf("query placeables: %w", err)
	}
	defer rows.Close()

	var placeables []blueprintPlaceable
	for rows.Next() {
		var pid int
		var buildingType string
		var transform []float32
		if err := rows.Scan(&pid, &buildingType, &transform); err != nil {
			continue
		}
		placeable, ok := buildBlueprintPlaceable(pid, buildingType, transform)
		if !ok {
			continue
		}
		placeables = append(placeables, placeable)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read placeables: %w", err)
	}
	return placeables, nil
}

func buildBlueprintPentashield(pid int, scale []int16) (blueprintPentashield, bool) {
	if len(scale) < 3 {
		return blueprintPentashield{}, false
	}
	return blueprintPentashield{
		PlaceableID: pid,
		Scale:       [3]int{int(scale[0]), int(scale[1]), int(scale[2])},
	}, true
}

func fetchBlueprintPentashields(ctx context.Context, db *pgxpool.Pool, blueprintID int64) ([]blueprintPentashield, error) {
	rows, err := db.Query(ctx, `
		SELECT placeable_id, scale
		FROM dune.building_blueprint_pentashields
		WHERE building_blueprint_id = $1
		ORDER BY placeable_id`, blueprintID)
	if err != nil {
		return nil, fmt.Errorf("query pentashields: %w", err)
	}
	defer rows.Close()

	var pentashields []blueprintPentashield
	for rows.Next() {
		var pid int
		var scale []int16
		if err := rows.Scan(&pid, &scale); err != nil {
			continue
		}
		pentashield, ok := buildBlueprintPentashield(pid, scale)
		if !ok {
			continue
		}
		pentashields = append(pentashields, pentashield)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pentashields: %w", err)
	}
	return pentashields, nil
}

// fetchBlueprintData fetches blueprint instances, placeables, and pentashields
// from the DB and returns a blueprintFile ready for JSON serialization.
func fetchBlueprintData(ctx context.Context, db *pgxpool.Pool, blueprintID int64) (blueprintFile, error) {
	if db == nil {
		return blueprintFile{}, fmt.Errorf("not connected")
	}

	name := fetchBlueprintName(ctx, db, blueprintID)
	instances, err := fetchBlueprintInstances(ctx, db, blueprintID)
	if err != nil {
		return blueprintFile{}, err
	}
	placeables, err := fetchBlueprintPlaceables(ctx, db, blueprintID)
	if err != nil {
		return blueprintFile{}, err
	}
	pentashields, err := fetchBlueprintPentashields(ctx, db, blueprintID)
	if err != nil {
		return blueprintFile{}, err
	}

	return blueprintFile{
		Name:         name,
		Instances:    instances,
		Placeables:   placeables,
		Pentashields: pentashields,
	}, nil
}

const blueprintImportBatchSize = 50

const blueprintPlaceholderStats = `{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#0"}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`

func findBackpackInventoryID(ctx context.Context, tx pgx.Tx, playerPawnID int64) (int64, error) {
	var invID int64
	err := tx.QueryRow(ctx, `
		SELECT id FROM dune.inventories
		WHERE actor_id = $1 AND inventory_type = 0
		LIMIT 1`, playerPawnID).Scan(&invID)
	if err != nil {
		return 0, fmt.Errorf("find inventory: %w", err)
	}
	return invID, nil
}

func nextInventoryPosition(ctx context.Context, tx pgx.Tx, inventoryID int64) int64 {
	var nextPos int64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position_index), -1) + 1
		FROM dune.items WHERE inventory_id = $1`, inventoryID).Scan(&nextPos)
	return nextPos
}

func createBlueprintItem(ctx context.Context, tx pgx.Tx, inventoryID, position int64) (int64, error) {
	var itemID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO dune.items
			(inventory_id, stack_size, position_index, template_id, quality_level, stats)
		VALUES ($1, 1, $2, 'BuildingBlueprint_CopyDevice', 0, $3::jsonb)
		RETURNING id`,
		inventoryID, position, blueprintPlaceholderStats).Scan(&itemID)
	if err != nil {
		return 0, fmt.Errorf("create item: %w", err)
	}
	return itemID, nil
}

func createBlueprintRecord(ctx context.Context, tx pgx.Tx, itemID int64) (int64, error) {
	var blueprintID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO dune.building_blueprints (item_id, player_id, building_blueprint_map)
		VALUES ($1, null, '')
		RETURNING id`, itemID).Scan(&blueprintID)
	if err != nil {
		return 0, fmt.Errorf("create blueprint: %w", err)
	}
	return blueprintID, nil
}

func blueprintItemStatsJSON(blueprintID int64, name string) string {
	nameJSON := ""
	if name != "" {
		nameJSON = fmt.Sprintf(`,"BuildingBlueprintName":%q`, name)
	}
	return fmt.Sprintf(
		`{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#%d"%s}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`,
		blueprintID, nameJSON)
}

func updateBlueprintItemStats(ctx context.Context, tx pgx.Tx, itemID, blueprintID int64, name string) error {
	if _, err := tx.Exec(ctx, `UPDATE dune.items SET stats = $1::jsonb WHERE id = $2`,
		blueprintItemStatsJSON(blueprintID, name), itemID); err != nil {
		return fmt.Errorf("update item stats: %w", err)
	}
	return nil
}

func resolveBlueprintImportInstance(start, offset int, inst blueprintInstance) (instanceID int, transform string, stability bool) {
	transform = fmt.Sprintf("{%g,%g,%g,%g}",
		float32(inst.X), float32(inst.Y), float32(inst.Z), float32(inst.Rotation))
	instanceID = start + offset + 1
	if inst.InstanceID != nil {
		instanceID = *inst.InstanceID
	}
	stability = isStructuralBuilding(inst.BuildingType)
	if inst.ProvidesStability != nil {
		stability = *inst.ProvidesStability
	}
	return instanceID, transform, stability
}

func insertBlueprintInstances(ctx context.Context, tx pgx.Tx, blueprintID int64, instances []blueprintInstance) error {
	for start := 0; start < len(instances); start += blueprintImportBatchSize {
		end := start + blueprintImportBatchSize
		if end > len(instances) {
			end = len(instances)
		}
		batch := &pgx.Batch{}
		for i, inst := range instances[start:end] {
			instanceID, transform, stability := resolveBlueprintImportInstance(start, i, inst)
			batch.Queue(`
				INSERT INTO dune.building_blueprint_instances
					(building_blueprint_id, instance_id, building_type, transform, hologram, provides_stability, health)
				VALUES ($1, $2, $3, $4::real[], true, $5, 0)`,
				blueprintID, instanceID, inst.BuildingType, transform, stability)
		}
		br := tx.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return fmt.Errorf("insert instance %d: %w", i, err)
			}
		}
		_ = br.Close()
	}
	return nil
}

func resolveBlueprintImportPlaceable(start, offset int, pl blueprintPlaceable) (placeableID int, transform string) {
	transform = fmt.Sprintf("{%g,%g,%g,%g,%g,%g}",
		float32(pl.X), float32(pl.Y), float32(pl.Z),
		float32(pl.RX), float32(pl.RY), float32(pl.RZ))
	placeableID = start + offset + 1
	if pl.PlaceableID != nil {
		placeableID = *pl.PlaceableID
	}
	return placeableID, transform
}

func insertBlueprintPlaceables(ctx context.Context, tx pgx.Tx, blueprintID int64, placeables []blueprintPlaceable) error {
	for start := 0; start < len(placeables); start += blueprintImportBatchSize {
		end := start + blueprintImportBatchSize
		if end > len(placeables) {
			end = len(placeables)
		}
		batch := &pgx.Batch{}
		for i, pl := range placeables[start:end] {
			placeableID, transform := resolveBlueprintImportPlaceable(start, i, pl)
			batch.Queue(`
				INSERT INTO dune.building_blueprint_placeables
					(building_blueprint_id, placeable_id, building_type, transform, hologram)
				VALUES ($1, $2, $3, $4::real[], true)`,
				blueprintID, placeableID, pl.BuildingType, transform)
		}
		br := tx.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return fmt.Errorf("insert placeable %d: %w", i, err)
			}
		}
		_ = br.Close()
	}
	return nil
}

func insertBlueprintPentashields(ctx context.Context, tx pgx.Tx, blueprintID int64, pentashields []blueprintPentashield) error {
	for _, ps := range pentashields {
		if _, err := tx.Exec(ctx, `
			INSERT INTO dune.building_blueprint_pentashields
				(building_blueprint_id, placeable_id, scale)
			VALUES ($1, $2, ARRAY[$3,$4,$5]::smallint[])`,
			blueprintID, ps.PlaceableID,
			int16(ps.Scale[0]), int16(ps.Scale[1]), int16(ps.Scale[2])); err != nil {
			return fmt.Errorf("insert pentashield %d: %w", ps.PlaceableID, err)
		}
	}
	return nil
}

// importBlueprintData imports a blueprintFile into the DB for the given player pawn ID.
func importBlueprintData(ctx context.Context, db *pgxpool.Pool, playerPawnID int64, bf blueprintFile) Msg {
	if db == nil {
		return msgMutate{err: fmt.Errorf("not connected")}
	}

	// Player must be offline.
	if err := checkPlayerOfflinePool(ctx, db, playerPawnID); err != nil {
		return msgMutate{err: err}
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return msgMutate{err: fmt.Errorf("begin tx: %w", err)}
	}
	defer func() { _ = tx.Rollback(ctx) }()

	invID, err := findBackpackInventoryID(ctx, tx, playerPawnID)
	if err != nil {
		return msgMutate{err: err}
	}

	itemID, err := createBlueprintItem(ctx, tx, invID, nextInventoryPosition(ctx, tx, invID))
	if err != nil {
		return msgMutate{err: err}
	}

	blueprintID, err := createBlueprintRecord(ctx, tx, itemID)
	if err != nil {
		return msgMutate{err: err}
	}

	// Update item stats with real blueprint ID and name (no PlayerBaseBackupId — crashes the game).
	if err := updateBlueprintItemStats(ctx, tx, itemID, blueprintID, bf.Name); err != nil {
		return msgMutate{err: err}
	}

	// Insert instances in batches of 50.
	// Per-row instance_id and provides_stability come from the JSON when present
	// (fresh exports always include them). Legacy files without these fields fall
	// back to 1-based sequential ids and a structural-type stability lookup —
	// matching the indexing scheme used by every existing blueprint in the DB
	// that the source pentashield placeable_id references assume.
	if err := insertBlueprintInstances(ctx, tx, blueprintID, bf.Instances); err != nil {
		return msgMutate{err: err}
	}

	// Insert placeables in batches of 50.
	if err := insertBlueprintPlaceables(ctx, tx, blueprintID, bf.Placeables); err != nil {
		return msgMutate{err: err}
	}

	// Insert pentashield scale data.
	if err := insertBlueprintPentashields(ctx, tx, blueprintID, bf.Pentashields); err != nil {
		return msgMutate{err: err}
	}

	if err := tx.Commit(ctx); err != nil {
		return msgMutate{err: fmt.Errorf("commit: %w", err)}
	}

	return msgMutate{ok: fmt.Sprintf(
		"Imported %d pieces + %d placeables + %d pentashields → blueprint #%d (item %d) in player inventory",
		len(bf.Instances), len(bf.Placeables), len(bf.Pentashields), blueprintID, itemID)}
}
