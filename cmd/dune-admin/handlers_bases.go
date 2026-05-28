package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

func quatToYaw(qx, qy, qz, qw float64) float64 {
	return math.Atan2(2*(qw*qz+qx*qy), 1-2*(qy*qy+qz*qz)) * 180 / math.Pi
}

func quatToEuler(qx, qy, qz, qw float64) (rx, ry, rz float64) {
	rx = math.Atan2(2*(qw*qx+qy*qz), 1-2*(qx*qx+qy*qy)) * 180 / math.Pi
	sinp := 2 * (qw*qy - qz*qx)
	if sinp >= 1 {
		ry = 90
	} else if sinp <= -1 {
		ry = -90
	} else {
		ry = math.Asin(sinp) * 180 / math.Pi
	}
	rz = math.Atan2(2*(qw*qz+qx*qy), 1-2*(qy*qy+qz*qz)) * 180 / math.Pi
	return
}

func parseVec3(s string) (x, y, z float64, err error) {
	s = strings.Trim(strings.TrimSpace(s), "()")
	parts := strings.SplitN(s, ",", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("expected 3 components in %q", s)
	}
	if x, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err != nil {
		return
	}
	if y, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err != nil {
		return
	}
	z, err = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	return
}

func parseVec4(s string) (x, y, z, w float64, err error) {
	s = strings.Trim(strings.TrimSpace(s), "()")
	parts := strings.SplitN(s, ",", 4)
	if len(parts) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("expected 4 components in %q", s)
	}
	if x, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err != nil {
		return
	}
	if y, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err != nil {
		return
	}
	if z, err = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err != nil {
		return
	}
	w, err = strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	return
}

func handleListBases(w http.ResponseWriter, _ *http.Request) {
	msg, ok := cmdListBases().(msgBaseList)
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
		rows = []baseRow{}
	}
	jsonOK(w, rows)
}

type rawBaseInstance struct {
	buildingType  string
	transform     []float32
	ownerEntityID int64
}

type rawBasePlaceable struct {
	buildingType string
	location     string
	rotation     string
	properties   map[string]any
}

func parseBasePathID(id string) (int64, error) {
	parsedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return parsedID, nil
}

func queryBaseExportInstances(ctx context.Context, id int64) ([]rawBaseInstance, error) {
	rows, err := globalDB.Query(ctx, `
		SELECT building_type, transform, owner_entity_id
		FROM dune.building_instances
		WHERE building_id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("query instances: %w", err)
	}
	defer rows.Close()

	raws := make([]rawBaseInstance, 0, 32)
	for rows.Next() {
		var ri rawBaseInstance
		if err := rows.Scan(&ri.buildingType, &ri.transform, &ri.ownerEntityID); err != nil {
			continue
		}
		if len(ri.transform) < 7 {
			continue
		}
		raws = append(raws, ri)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read instances: %w", err)
	}
	return raws, nil
}

func queryBaseExportPlaceables(ctx context.Context, ownerEntityID int64) ([]rawBasePlaceable, error) {
	rows, err := globalDB.Query(ctx, `
		SELECT p.building_type,
		       (a.transform).location::text,
		       (a.transform).rotation::text,
		       a.properties
		FROM dune.placeables p
		JOIN dune.actors a ON a.id = p.id
		WHERE p.owner_entity_id = $1`, ownerEntityID)
	if err != nil {
		return nil, fmt.Errorf("query placeables: %w", err)
	}
	defer rows.Close()

	raws := make([]rawBasePlaceable, 0, 32)
	for rows.Next() {
		var rp rawBasePlaceable
		if err := rows.Scan(&rp.buildingType, &rp.location, &rp.rotation, &rp.properties); err != nil {
			continue
		}
		raws = append(raws, rp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read placeables: %w", err)
	}
	return raws, nil
}

func calculateBaseCentroid(raws []rawBaseInstance) (float64, float64, float64) {
	var sumX, sumY, sumZ float64
	for _, ri := range raws {
		sumX += float64(ri.transform[0])
		sumY += float64(ri.transform[1])
		sumZ += float64(ri.transform[2])
	}
	n := float64(len(raws))
	return sumX / n, sumY / n, sumZ / n
}

func buildBlueprintInstances(raws []rawBaseInstance, cx, cy, cz float64) []blueprintInstance {
	instances := make([]blueprintInstance, 0, len(raws))
	for _, ri := range raws {
		qx, qy, qz, qw := float64(ri.transform[3]), float64(ri.transform[4]), float64(ri.transform[5]), float64(ri.transform[6])
		instances = append(instances, blueprintInstance{
			BuildingType: ri.buildingType,
			X:            float64(ri.transform[0]) - cx,
			Y:            float64(ri.transform[1]) - cy,
			Z:            float64(ri.transform[2]) - cz,
			Rotation:     quatToYaw(qx, qy, qz, qw),
		})
	}
	return instances
}

func extractPentashieldScale(buildingType string, props map[string]any) ([3]int, bool) {
	var scale [3]int
	if props == nil {
		return scale, false
	}
	inner, ok := props[strings.TrimSuffix(buildingType, "_Placeable")+"_C"].(map[string]any)
	if !ok {
		return scale, false
	}
	scaleValues, ok := inner["m_Scale"].([]any)
	if !ok || len(scaleValues) < 3 {
		return scale, false
	}
	for i := 0; i < 3; i++ {
		value, ok := scaleValues[i].(float64)
		if !ok {
			return scale, false
		}
		scale[i] = int(value)
	}
	return scale, true
}

func convertExportPlaceable(raw rawBasePlaceable, cx, cy, cz float64, placeableID int) (blueprintPlaceable, *blueprintPentashield, bool) {
	if raw.buildingType == "Totem_Placeable" {
		return blueprintPlaceable{}, nil, false
	}
	lx, ly, lz, locErr := parseVec3(raw.location)
	qx, qy, qz, qw, rotErr := parseVec4(raw.rotation)
	if locErr != nil || rotErr != nil {
		return blueprintPlaceable{}, nil, false
	}
	rx, ry, rz := quatToEuler(qx, qy, qz, qw)
	placeable := blueprintPlaceable{
		BuildingType: raw.buildingType,
		X:            lx - cx,
		Y:            ly - cy,
		Z:            lz - cz,
		RX:           rx,
		RY:           ry,
		RZ:           rz,
	}
	if !strings.Contains(raw.buildingType, "PentashieldSurface") {
		return placeable, nil, true
	}
	scale, ok := extractPentashieldScale(raw.buildingType, raw.properties)
	if !ok {
		return blueprintPlaceable{}, nil, false
	}
	return placeable, &blueprintPentashield{PlaceableID: placeableID, Scale: scale}, true
}

func buildBlueprintPlaceables(raws []rawBasePlaceable, cx, cy, cz float64) ([]blueprintPlaceable, []blueprintPentashield) {
	placeables := make([]blueprintPlaceable, 0, len(raws))
	pentashields := make([]blueprintPentashield, 0, len(raws))
	for _, raw := range raws {
		nextID := len(placeables)
		placeable, pentashield, ok := convertExportPlaceable(raw, cx, cy, cz, nextID)
		if !ok {
			continue
		}
		placeables = append(placeables, placeable)
		if pentashield != nil {
			pentashields = append(pentashields, *pentashield)
		}
	}
	return placeables, pentashields
}

func writeExportBaseResponse(
	w http.ResponseWriter,
	id int64,
	instances []blueprintInstance,
	placeables []blueprintPlaceable,
	pentashields []blueprintPentashield,
) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="base_%d.json"`, id))
	jsonOK(w, blueprintFile{
		Instances:    instances,
		Placeables:   placeables,
		Pentashields: pentashields,
	})
}

func handleExportBase(w http.ResponseWriter, r *http.Request) {
	id, err := parseBasePathID(r.PathValue("id"))
	if err != nil {
		jsonErr(w, err, 400)
		return
	}
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("not connected"), 500)
		return
	}
	ctx := context.Background()

	rawInstances, err := queryBaseExportInstances(ctx, id)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	if len(rawInstances) == 0 {
		jsonErr(w, fmt.Errorf("building %d not found or empty", id), 404)
		return
	}

	cx, cy, cz := calculateBaseCentroid(rawInstances)
	instances := buildBlueprintInstances(rawInstances, cx, cy, cz)

	rawPlaceables, err := queryBaseExportPlaceables(ctx, rawInstances[0].ownerEntityID)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	placeables, pentashields := buildBlueprintPlaceables(rawPlaceables, cx, cy, cz)
	writeExportBaseResponse(w, id, instances, placeables, pentashields)
}
