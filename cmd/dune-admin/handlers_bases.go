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

func handleExportBase(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), 400)
		return
	}
	if globalDB == nil {
		jsonErr(w, fmt.Errorf("not connected"), 500)
		return
	}
	ctx := context.Background()

	iRows, err := globalDB.Query(ctx, `
		SELECT building_type, transform, owner_entity_id
		FROM dune.building_instances
		WHERE building_id = $1`, id)
	if err != nil {
		jsonErr(w, fmt.Errorf("query instances: %w", err), 500)
		return
	}
	defer iRows.Close()

	type rawInstance struct {
		btype         string
		t             []float32
		ownerEntityID int64
	}
	var raws []rawInstance
	for iRows.Next() {
		var ri rawInstance
		if err := iRows.Scan(&ri.btype, &ri.t, &ri.ownerEntityID); err != nil {
			continue
		}
		if len(ri.t) < 7 {
			continue
		}
		raws = append(raws, ri)
	}
	if err := iRows.Err(); err != nil {
		jsonErr(w, fmt.Errorf("read instances: %w", err), 500)
		return
	}
	if len(raws) == 0 {
		jsonErr(w, fmt.Errorf("building %d not found or empty", id), 404)
		return
	}

	ownerEntityID := raws[0].ownerEntityID

	var sumX, sumY, sumZ float64
	for _, ri := range raws {
		sumX += float64(ri.t[0])
		sumY += float64(ri.t[1])
		sumZ += float64(ri.t[2])
	}
	n := float64(len(raws))
	cx, cy, cz := sumX/n, sumY/n, sumZ/n

	instances := make([]blueprintInstance, 0, len(raws))
	for _, ri := range raws {
		qx, qy, qz, qw := float64(ri.t[3]), float64(ri.t[4]), float64(ri.t[5]), float64(ri.t[6])
		instances = append(instances, blueprintInstance{
			BuildingType: ri.btype,
			X:            float64(ri.t[0]) - cx,
			Y:            float64(ri.t[1]) - cy,
			Z:            float64(ri.t[2]) - cz,
			Rotation:     quatToYaw(qx, qy, qz, qw),
		})
	}

	pRows, err := globalDB.Query(ctx, `
		SELECT p.building_type,
		       (a.transform).location::text,
		       (a.transform).rotation::text,
		       a.properties
		FROM dune.placeables p
		JOIN dune.actors a ON a.id = p.id
		WHERE p.owner_entity_id = $1`, ownerEntityID)
	if err != nil {
		jsonErr(w, fmt.Errorf("query placeables: %w", err), 500)
		return
	}
	defer pRows.Close()

	var placeables []blueprintPlaceable
	var pentashields []blueprintPentashield

	for pRows.Next() {
		var btype, locStr, rotStr string
		var props map[string]any
		if err := pRows.Scan(&btype, &locStr, &rotStr, &props); err != nil {
			continue
		}
		// Totem is base-specific (land claim anchor) — never export it.
		if btype == "Totem_Placeable" {
			continue
		}
		lx, ly, lz, locErr := parseVec3(locStr)
		qx, qy, qz, qw, rotErr := parseVec4(rotStr)
		if locErr != nil || rotErr != nil {
			continue
		}
		rx, ry, rz := quatToEuler(qx, qy, qz, qw)

		if strings.Contains(btype, "PentashieldSurface") {
			// Only include pentashield placeables if scale data is present;
			// a zero scale means the server has no data and would crash on import.
			scale := [3]int{0, 0, 0}
			found := false
			if props != nil {
				if inner, ok := props[strings.TrimSuffix(btype, "_Placeable")+"_C"].(map[string]any); ok {
					if sv, ok := inner["m_Scale"].([]any); ok && len(sv) >= 3 {
						for i := range 3 {
							if f, ok := sv[i].(float64); ok {
								scale[i] = int(f)
							}
						}
						found = true
					}
				}
			}
			if !found {
				continue
			}
			idx := len(placeables)
			placeables = append(placeables, blueprintPlaceable{
				BuildingType: btype,
				X:            lx - cx,
				Y:            ly - cy,
				Z:            lz - cz,
				RX:           rx,
				RY:           ry,
				RZ:           rz,
			})
			pentashields = append(pentashields, blueprintPentashield{
				PlaceableID: idx,
				Scale:       scale,
			})
			continue
		}

		placeables = append(placeables, blueprintPlaceable{
			BuildingType: btype,
			X:            lx - cx,
			Y:            ly - cy,
			Z:            lz - cz,
			RX:           rx,
			RY:           ry,
			RZ:           rz,
		})
	}
	if err := pRows.Err(); err != nil {
		jsonErr(w, fmt.Errorf("read placeables: %w", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="base_%d.json"`, id))
	jsonOK(w, blueprintFile{
		Instances:    instances,
		Placeables:   placeables,
		Pentashields: pentashields,
	})
}
