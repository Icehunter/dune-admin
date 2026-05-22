package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func handleListBlueprints(w http.ResponseWriter, r *http.Request) {
	msg, ok := cmdListBlueprints().(msgBlueprintList)
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

func handleExportBlueprint(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	bf, err := fetchBlueprintData(r.Context(), id)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="blueprint_%d.json"`, id))
	json.NewEncoder(w).Encode(bf)
}

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
	defer f.Close()

	var bf blueprintFile
	if err := json.NewDecoder(f).Decode(&bf); err != nil {
		jsonErr(w, fmt.Errorf("invalid blueprint JSON: %w", err), 400)
		return
	}
	if len(bf.Instances) == 0 && len(bf.Placeables) == 0 {
		jsonErr(w, fmt.Errorf("blueprint has no instances or placeables"), 400)
		return
	}

	msg, ok := importBlueprintData(r.Context(), playerID, bf).(msgMutate)
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

// fetchBlueprintData fetches blueprint instances, placeables, and pentashields
// from the DB and returns a blueprintFile ready for JSON serialization.
func fetchBlueprintData(ctx context.Context, blueprintID int64) (blueprintFile, error) {
	if globalDB == nil {
		return blueprintFile{}, fmt.Errorf("not connected")
	}

	// Fetch the blueprint name from the item stats.
	var name string
	_ = globalDB.QueryRow(ctx, `
		SELECT COALESCE(i.stats->'FBuildingBlueprintItemStats'->1->>'BuildingBlueprintName', '')
		FROM dune.building_blueprints bb
		JOIN dune.items i ON i.id = bb.item_id
		WHERE bb.id = $1`, blueprintID).Scan(&name)

	// Fetch instances.
	iRows, err := globalDB.Query(ctx, `
		SELECT building_type, transform
		FROM dune.building_blueprint_instances
		WHERE building_blueprint_id = $1
		ORDER BY instance_id`, blueprintID)
	if err != nil {
		return blueprintFile{}, fmt.Errorf("query instances: %w", err)
	}
	defer iRows.Close()

	var instances []blueprintInstance
	for iRows.Next() {
		var btype string
		var t []float32
		if err := iRows.Scan(&btype, &t); err != nil {
			continue
		}
		if len(t) < 4 {
			continue
		}
		instances = append(instances, blueprintInstance{
			BuildingType: btype,
			X:            float64(t[0]),
			Y:            float64(t[1]),
			Z:            float64(t[2]),
			Rotation:     float64(t[3]),
		})
	}
	if err := iRows.Err(); err != nil {
		return blueprintFile{}, fmt.Errorf("read instances: %w", err)
	}

	// Fetch placeables.
	pRows, err := globalDB.Query(ctx, `
		SELECT building_type, transform
		FROM dune.building_blueprint_placeables
		WHERE building_blueprint_id = $1
		ORDER BY placeable_id`, blueprintID)
	if err != nil {
		return blueprintFile{}, fmt.Errorf("query placeables: %w", err)
	}
	defer pRows.Close()

	var placeables []blueprintPlaceable
	for pRows.Next() {
		var btype string
		var t []float32
		if err := pRows.Scan(&btype, &t); err != nil {
			continue
		}
		if len(t) < 6 {
			continue
		}
		placeables = append(placeables, blueprintPlaceable{
			BuildingType: btype,
			X:            float64(t[0]),
			Y:            float64(t[1]),
			Z:            float64(t[2]),
			RX:           float64(t[3]),
			RY:           float64(t[4]),
			RZ:           float64(t[5]),
		})
	}
	if err := pRows.Err(); err != nil {
		return blueprintFile{}, fmt.Errorf("read placeables: %w", err)
	}

	// Fetch pentashield scale data.
	psRows, err := globalDB.Query(ctx, `
		SELECT placeable_id, scale
		FROM dune.building_blueprint_pentashields
		WHERE building_blueprint_id = $1
		ORDER BY placeable_id`, blueprintID)
	if err != nil {
		return blueprintFile{}, fmt.Errorf("query pentashields: %w", err)
	}
	defer psRows.Close()

	var pentashields []blueprintPentashield
	for psRows.Next() {
		var pid int
		var scale []int16
		if err := psRows.Scan(&pid, &scale); err != nil {
			continue
		}
		if len(scale) < 3 {
			continue
		}
		pentashields = append(pentashields, blueprintPentashield{
			PlaceableID: pid,
			Scale:       [3]int{int(scale[0]), int(scale[1]), int(scale[2])},
		})
	}
	if err := psRows.Err(); err != nil {
		return blueprintFile{}, fmt.Errorf("read pentashields: %w", err)
	}

	return blueprintFile{
		Name:         name,
		Instances:    instances,
		Placeables:   placeables,
		Pentashields: pentashields,
	}, nil
}

// importBlueprintData imports a blueprintFile into the DB for the given player pawn ID.
func importBlueprintData(ctx context.Context, playerPawnID int64, bf blueprintFile) Msg {
	if globalDB == nil {
		return msgMutate{err: fmt.Errorf("not connected")}
	}

	// Player must be offline.
	if err := checkPlayerOffline(ctx, playerPawnID); err != nil {
		return msgMutate{err: err}
	}

	tx, err := globalDB.Begin(ctx)
	if err != nil {
		return msgMutate{err: fmt.Errorf("begin tx: %w", err)}
	}
	defer tx.Rollback(ctx)

	// Get backpack inventory.
	var invID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM dune.inventories
		WHERE actor_id = $1 AND inventory_type = 0
		LIMIT 1`, playerPawnID).Scan(&invID)
	if err != nil {
		return msgMutate{err: fmt.Errorf("find inventory: %w", err)}
	}

	// Next free position index.
	var nextPos int64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position_index), -1) + 1
		FROM dune.items WHERE inventory_id = $1`, invID).Scan(&nextPos)

	// Placeholder stats — will be updated with real blueprint ID after insert.
	placeholderStats := `{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#0"}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`

	var itemID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO dune.items
			(inventory_id, stack_size, position_index, template_id, quality_level, stats)
		VALUES ($1, 1, $2, 'BuildingBlueprint_CopyDevice', 0, $3::jsonb)
		RETURNING id`,
		invID, nextPos, placeholderStats).Scan(&itemID)
	if err != nil {
		return msgMutate{err: fmt.Errorf("create item: %w", err)}
	}

	// Insert blueprint master record.
	var blueprintID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO dune.building_blueprints (item_id, player_id, building_blueprint_map)
		VALUES ($1, null, '')
		RETURNING id`, itemID).Scan(&blueprintID)
	if err != nil {
		return msgMutate{err: fmt.Errorf("create blueprint: %w", err)}
	}

	// Update item stats with real blueprint ID and name (no PlayerBaseBackupId — crashes the game).
	nameJSON := ""
	if bf.Name != "" {
		nameJSON = fmt.Sprintf(`,"BuildingBlueprintName":%q`, bf.Name)
	}
	fullStats := fmt.Sprintf(
		`{"FCustomizationStats":[[], {}],"FBuildingBlueprintItemStats":[[], {"PlayerBlueprintId":"!!bbp#%d"%s}],"FItemStackAndDurabilityStats":[[], {"DecayedMaxDurability":0.0}]}`,
		blueprintID, nameJSON)
	if _, err = tx.Exec(ctx, `UPDATE dune.items SET stats = $1::jsonb WHERE id = $2`,
		fullStats, itemID); err != nil {
		return msgMutate{err: fmt.Errorf("update item stats: %w", err)}
	}

	// Insert instances in batches of 50.
	const batchSize = 50
	for start := 0; start < len(bf.Instances); start += batchSize {
		end := start + batchSize
		if end > len(bf.Instances) {
			end = len(bf.Instances)
		}
		batch := &pgx.Batch{}
		for i, inst := range bf.Instances[start:end] {
			transform := fmt.Sprintf("[0:3]={%g,%g,%g,%g}",
				float32(inst.X), float32(inst.Y), float32(inst.Z), float32(inst.Rotation))
			batch.Queue(`
				INSERT INTO dune.building_blueprint_instances
					(building_blueprint_id, instance_id, building_type, transform, hologram, provides_stability, health)
				VALUES ($1, $2, $3, $4::real[], true, false, 0)`,
				blueprintID, start+i, inst.BuildingType, transform)
		}
		br := tx.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return msgMutate{err: fmt.Errorf("insert instance %d: %w", i, err)}
			}
		}
		br.Close()
	}

	// Insert placeables in batches of 50.
	for start := 0; start < len(bf.Placeables); start += batchSize {
		end := start + batchSize
		if end > len(bf.Placeables) {
			end = len(bf.Placeables)
		}
		batch := &pgx.Batch{}
		for i, pl := range bf.Placeables[start:end] {
			transform := fmt.Sprintf("[0:5]={%g,%g,%g,%g,%g,%g}",
				float32(pl.X), float32(pl.Y), float32(pl.Z),
				float32(pl.RX), float32(pl.RY), float32(pl.RZ))
			batch.Queue(`
				INSERT INTO dune.building_blueprint_placeables
					(building_blueprint_id, placeable_id, building_type, transform, hologram)
				VALUES ($1, $2, $3, $4::real[], true)`,
				blueprintID, start+i, pl.BuildingType, transform)
		}
		br := tx.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return msgMutate{err: fmt.Errorf("insert placeable %d: %w", i, err)}
			}
		}
		br.Close()
	}

	// Insert pentashield scale data.
	for _, ps := range bf.Pentashields {
		if _, err = tx.Exec(ctx, `
			INSERT INTO dune.building_blueprint_pentashields
				(building_blueprint_id, placeable_id, scale)
			VALUES ($1, $2, ARRAY[$3,$4,$5]::smallint[])`,
			blueprintID, ps.PlaceableID,
			int16(ps.Scale[0]), int16(ps.Scale[1]), int16(ps.Scale[2])); err != nil {
			return msgMutate{err: fmt.Errorf("insert pentashield %d: %w", ps.PlaceableID, err)}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return msgMutate{err: fmt.Errorf("commit: %w", err)}
	}

	return msgMutate{ok: fmt.Sprintf(
		"Imported %d pieces + %d placeables + %d pentashields → blueprint #%d (item %d) in player inventory",
		len(bf.Instances), len(bf.Placeables), len(bf.Pentashields), blueprintID, itemID)}
}
