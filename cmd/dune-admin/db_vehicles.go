package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// vehiclesQuery lists one row per vehicle the player can reach. %[1]s is the
// account_id/character_id key column resolved by playerKeyFor.
//
// Backups are a LEFT JOIN flag, not a second UNION arm. A vehicle backup leaves
// the original actor in place, so the old UNION ALL emitted every backed-up
// vehicle twice — once "normal", once "Backup" (#313).
//
// Chassis durability comes from dune.vehicle_modules. The previous source,
// recovered_vehicles.chassis_durability, is only populated for recovered
// vehicles and was COALESCEd to 1.0, so every vehicle displayed as 100%.
const vehiclesQuery = `
	SELECT pa.actor_id,
	       a.class,
	       COALESCE(a.map, ''),
	       COALESCE(a.partition_id, 0),
	       COALESCE(a.dimension_index, 0),
	       COALESCE(pa.actor_name, ''),
	       par.rank,
	       COALESCE(ops.character_name, ''),
	       COALESCE(ch.cur, 0),
	       COALESCE(ch.maxd, 0),
	       (rv.vehicle_id IS NOT NULL) AS is_recovered,
	       (bv.vehicle_id IS NOT NULL) AS is_backup
	FROM dune.permission_actor pa
	JOIN dune.permission_actor_rank par
	  ON par.permission_actor_id = pa.actor_id AND par.player_id = $1
	JOIN dune.actors a ON a.id = pa.actor_id
	LEFT JOIN dune.recovered_vehicles rv ON rv.vehicle_id = pa.actor_id AND rv.%[1]s = $2
	LEFT JOIN dune.backup_vehicles   bv ON bv.vehicle_id = pa.actor_id AND bv.%[1]s = $2
	-- Exactly one rank-1 holder per vehicle: the owner.
	LEFT JOIN dune.permission_actor_rank opar
	  ON opar.permission_actor_id = pa.actor_id AND opar.rank = 1
	LEFT JOIN dune.player_state ops ON ops.player_controller_id = opar.player_id
	LEFT JOIN LATERAL (
	    SELECT (m.stats->'FVehicleModuleDurabilityStats'->1->>'CurrentDurability')::float8 AS cur,
	           (m.stats->'FVehicleModuleDurabilityStats'->1->>'DecayedMaxDurability')::float8 AS maxd
	    FROM dune.vehicle_modules m
	    WHERE m.vehicle_id = pa.actor_id AND m.template_id ILIKE '%%Chassis%%'
	    LIMIT 1
	) ch ON TRUE
	WHERE pa.actor_type = 2

	UNION ALL

	-- A backup whose vehicle actor is gone is the only remaining trace of it,
	-- so it still has to be listed.
	SELECT bv.vehicle_id, COALESCE(a.class, ''), '', 0, 0, '', 0, '', 0, 0, false, true
	FROM dune.backup_vehicles bv
	LEFT JOIN dune.actors a ON a.id = bv.vehicle_id
	WHERE bv.%[1]s = $2
	  AND NOT EXISTS (
	      SELECT 1 FROM dune.permission_actor pa2
	      WHERE pa2.actor_id = bv.vehicle_id AND pa2.actor_type = 2)

	ORDER BY 2`

// scanVehicleRow maps one result row into a vehicleRow, applying the display
// rules. A row that fails to scan is skipped rather than failing the request.
func scanVehicleRow(rows pgx.Rows) (vehicleRow, bool) {
	var r vehicleRow
	var rawName string
	if err := rows.Scan(
		&r.ID, &r.Class, &r.Map, &r.Partition, &r.Dimension, &rawName,
		&r.AccessRank, &r.OwnerName, &r.ChassisCurrent, &r.ChassisMax,
		&r.IsRecovered, &r.IsBackup,
	); err != nil {
		return vehicleRow{}, false
	}
	r.Class = vehicleClassLabel(r.Class)
	r.VehicleName = cleanVehicleName(rawName)
	r.Location = formatVehicleLocation(r.Map, r.Partition, r.Dimension)
	r.IsOwner = vehicleIsOwner(r.AccessRank)
	r.AccessLabel = vehicleAccessLabel(r.AccessRank)
	r.ChassisPct, r.HasChassisPct = chassisConditionPct(r.ChassisCurrent, r.ChassisMax)
	return r, true
}

// Vehicle display helpers. Kept pure and separate from the SQL in db.go so the
// presentation rules behind #313 are unit-testable.

// vehicleOwnerRank is the permission_actor_rank.rank value held by a vehicle's
// owner. Every vehicle has exactly one; ranks above it are granted access.
const vehicleOwnerRank = 1

// cleanVehicleName drops the game's placeholder names. An unnamed vehicle
// records '##<Type>' (e.g. ##MediumOrnithopter) or the literal 'None' on
// dune.permission_actor.actor_name; only a player-chosen name should surface.
// Mirrors the container-name handling in cmdFetchBases.
func cleanVehicleName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed == "None" || strings.HasPrefix(trimmed, "##") {
		return ""
	}
	return trimmed
}

// splitCamelCase inserts spaces at lower→upper boundaries only, so runs of
// capitals stay intact: "LightOrnithopter" becomes "Light Ornithopter" while
// "CHOAM" is left alone.
func splitCamelCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// vehicleClassLabel turns an Unreal class path into something readable.
//
// The path's object name repeats the package name
// (/Game/…/BP_Buggy_CHOAM.BP_Buggy_CHOAM_C), so shortClass alone yields
// "BP_Buggy_CHOAM.BP_Buggy_CHOAM" — long enough to be clipped in the table
// (#313). Collapse the duplicate, drop the BP_ prefix, and space out the words.
//
// Scoped to vehicles rather than changing shortClass, which three other call
// sites depend on.
func vehicleClassLabel(raw string) string {
	s := shortClass(raw)
	if pkg, obj, found := strings.Cut(s, "."); found && pkg == obj {
		s = obj
	}
	s = strings.TrimPrefix(s, "BP_")
	return splitCamelCase(strings.ReplaceAll(s, "_", " "))
}

// vehicleIsOwner reports whether a rank denotes ownership rather than a grant.
func vehicleIsOwner(rank int) bool { return rank == vehicleOwnerRank }

// vehicleAccessLabel describes how a player holds a vehicle.
func vehicleAccessLabel(rank int) string {
	switch {
	case rank == vehicleOwnerRank:
		return "owner"
	case rank > vehicleOwnerRank:
		return "granted"
	default:
		return "unknown"
	}
}

// formatVehicleLocation renders where a vehicle sits. Vehicles are spread
// across partitions and dimensions on a multi-sietch server, so the map name
// alone is ambiguous (#313). Missing pieces degrade rather than print zeros.
func formatVehicleLocation(mapName string, partition int64, dimension int) string {
	mapName = strings.TrimSpace(mapName)
	if partition <= 0 {
		return mapName
	}
	coords := fmt.Sprintf("p%d/d%d", partition, dimension)
	if mapName == "" {
		return coords
	}
	return mapName + " " + coords
}

// chassisConditionPct converts a module's durability into a percentage, but
// only when a real maximum is known.
//
// Most vehicle_modules rows carry CurrentDurability with no DecayedMaxDurability
// (the max only appears once a part has decayed), so there is usually no
// denominator. Reporting a percentage anyway is what produced the old
// always-100% column, which read from the empty recovered_vehicles table.
func chassisConditionPct(current, max float64) (float64, bool) {
	if max <= 0 {
		return 0, false
	}
	return current / max * 100, true
}
