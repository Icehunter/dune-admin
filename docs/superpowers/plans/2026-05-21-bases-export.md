# Bases Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/v1/bases` and `GET /api/v1/bases/{id}/export` endpoints plus a Bases UI tab so admins can list live in-world bases and export any base as a solido-compatible blueprint JSON.

**Architecture:** Backend follows the existing `cmd*` / `handle*` / `msg*` pattern. Export converts absolute world transforms (7-float quaternion arrays for instances, PostgreSQL composite type for placeables) to centroid-relative positions with yaw/Euler angles, producing an identical `blueprintFile` to the existing blueprint export. Frontend mirrors `BlueprintsTab` — a sticky-header table with an export download link, registered as a new `isSignedIn`-gated tab.

**Tech Stack:** Go (pgx, net/http), React + TypeScript, HeroUI v3

---

## File Map

| File | Change |
|---|---|
| `model.go` | Add `baseRow`, `msgBaseList` |
| `db.go` | Add `cmdListBases()` |
| `handlers_bases.go` | **Create** — `handleListBases`, `handleExportBase`, transform helpers |
| `server.go` | Register 2 routes |
| `web/src/api/client.ts` | Add `BaseRow` type, `api.bases` |
| `web/src/tabs/BasesTab.tsx` | **Create** — bases list tab |
| `web/src/App.tsx` | Import and register Bases tab |

---

## Task 1: Add model types

**Files:** Modify `model.go`

- [ ] **Add `baseRow` and `msgBaseList` after the `blueprintRow` block (after line 85)**

```go
type baseRow struct {
 ID         int64  `json:"id"`
 Name       string `json:"name"`
 Pieces     int64  `json:"pieces"`
 Placeables int64  `json:"placeables"`
}
```

```go
type msgBaseList struct {
 rows []baseRow
 err  error
}
```

- [ ] **Verify the file compiles**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin && go build ./...
```

Expected: no output (success).

- [ ] **Commit**

```bash
git add model.go
git commit -m "feat: add baseRow and msgBaseList model types"
```

---

## Task 2: Add `cmdListBases` to db.go

**Files:** Modify `db.go`

- [ ] **Append `cmdListBases` at the end of db.go**

```go
func cmdListBases() Msg {
 if globalDB == nil {
  return msgBaseList{err: fmt.Errorf("not connected")}
 }
 rows, err := globalDB.Query(context.Background(), `
  SELECT b.id,
         COALESCE(pa.actor_name, '') AS name,
         COUNT(DISTINCT bi.instance_id) AS pieces,
         COUNT(DISTINCT p.id) AS placeables
  FROM dune.buildings b
  LEFT JOIN dune.building_instances bi ON bi.building_id = b.id
  LEFT JOIN dune.actor_fgl_entities afe ON afe.entity_id = bi.owner_entity_id
  LEFT JOIN dune.actors t ON t.id = afe.actor_id AND t.class ILIKE '%Totem%'
  LEFT JOIN dune.permission_actor pa ON pa.actor_id = t.id
  LEFT JOIN dune.placeables p ON p.owner_entity_id = bi.owner_entity_id
  GROUP BY b.id, pa.actor_name
  ORDER BY b.id`)
 if err != nil {
  return msgBaseList{err: err}
 }
 defer rows.Close()
 var out []baseRow
 for rows.Next() {
  var r baseRow
  if err := rows.Scan(&r.ID, &r.Name, &r.Pieces, &r.Placeables); err != nil {
   continue
  }
  out = append(out, r)
 }
 if err := rows.Err(); err != nil {
  return msgBaseList{err: err}
 }
 return msgBaseList{rows: out}
}
```

- [ ] **Verify the file compiles**

```bash
go build ./...
```

Expected: no output.

- [ ] **Commit**

```bash
git add db.go
git commit -m "feat: add cmdListBases DB query"
```

---

## Task 3: Create handlers_bases.go

**Files:** Create `handlers_bases.go`

- [ ] **Create the file with all handlers and helpers**

```go
package main

import (
 "context"
 "fmt"
 "math"
 "net/http"
 "strconv"
 "strings"
)

// ── transform helpers ─────────────────────────────────────────────────────────

// quatToYaw converts a quaternion to a yaw angle in degrees.
// Buildings only rotate around Z so QX and QY are always ~0.
func quatToYaw(qx, qy, qz, qw float64) float64 {
 return math.Atan2(2*(qw*qz+qx*qy), 1-2*(qy*qy+qz*qz)) * 180 / math.Pi
}

// quatToEuler converts a quaternion to Euler angles (roll, pitch, yaw) in degrees.
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

// parseVec3 parses the PostgreSQL composite text format "(x,y,z)".
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

// parseVec4 parses the PostgreSQL composite text format "(x,y,z,w)".
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

// ── handlers ──────────────────────────────────────────────────────────────────

func handleListBases(w http.ResponseWriter, r *http.Request) {
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

 // 1. Fetch instances, collect owner_entity_id from the first row.
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
  btype          string
  t              []float32
  ownerEntityID  int64
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

 // 2. Compute centroid.
 var sumX, sumY, sumZ float64
 for _, ri := range raws {
  sumX += float64(ri.t[0])
  sumY += float64(ri.t[1])
  sumZ += float64(ri.t[2])
 }
 n := float64(len(raws))
 cx, cy, cz := sumX/n, sumY/n, sumZ/n

 // 3. Convert instances to relative blueprint format.
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

 // 4. Fetch placeables via shared owner_entity_id.
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
  lx, ly, lz, locErr := parseVec3(locStr)
  qx, qy, qz, qw, rotErr := parseVec4(rotStr)
  if locErr != nil || rotErr != nil {
   continue
  }
  rx, ry, rz := quatToEuler(qx, qy, qz, qw)
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

  // 5. Detect pentashield placeables and extract scale.
  if strings.Contains(btype, "PentashieldSurface") {
   scale := [3]int{0, 0, 0}
   if props != nil {
    if inner, ok := props[strings.TrimSuffix(
     btype, "_Placeable")+"_C"].(map[string]any); ok {
     if sv, ok := inner["m_Scale"].([]any); ok && len(sv) >= 3 {
      for i := 0; i < 3; i++ {
       if f, ok := sv[i].(float64); ok {
        scale[i] = int(f)
       }
      }
     }
    }
   }
   pentashields = append(pentashields, blueprintPentashield{
    PlaceableID: idx,
    Scale:       scale,
   })
  }
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
```

- [ ] **Verify the file compiles**

```bash
go build ./...
```

Expected: no output.

- [ ] **Commit**

```bash
git add handlers_bases.go
git commit -m "feat: add base list and export handlers with transform math"
```

---

## Task 4: Register routes in server.go

**Files:** Modify `server.go`

- [ ] **Add two lines in the bases section, after the blueprints block (after line 94)**

```go
 // ── bases ─────────────────────────────────────────────────────────────────
 mux.HandleFunc("GET /api/v1/bases", handleListBases)
 mux.HandleFunc("GET /api/v1/bases/{id}/export", handleExportBase)
```

- [ ] **Build and smoke-test**

```bash
go build -o dune-admin . && echo "build ok"
```

Expected: `build ok`

```bash
curl -s http://localhost:8080/api/v1/bases | python3 -m json.tool
```

Expected: JSON array of base objects, e.g.:

```json
[{"id": 335, "name": "Air Crossroads PWR-SAVE", "pieces": 3554, "placeables": 308}]
```

```bash
curl -s http://localhost:8080/api/v1/bases/335/export | python3 -c "
import json,sys
d=json.load(sys.stdin)
print('instances:', len(d.get('instances',[])))
print('placeables:', len(d.get('placeables',[])))
print('pentashields:', len(d.get('pentashields',[])))
"
```

Expected:

```
instances: 3554
placeables: 308
pentashields: <number >= 0>
```

- [ ] **Commit**

```bash
git add server.go
git commit -m "feat: register GET /api/v1/bases and GET /api/v1/bases/{id}/export routes"
```

---

## Task 5: Add BaseRow type and api.bases to client.ts

**Files:** Modify `web/src/api/client.ts`

- [ ] **Add `BaseRow` type after the `BlueprintRow` type (line 43)**

```ts
export type BaseRow = { id: number; name: string; pieces: number; placeables: number }
```

- [ ] **Add `bases` section to the `api` object after the `blueprints` block (after line 151)**

```ts
  bases: {
    list: () => req<BaseRow[]>('GET', '/bases'),
    exportUrl: (id: number) => `${BASE}/bases/${id}/export`,
  },
```

- [ ] **Verify TypeScript compiles**

```bash
cd web && npm run build 2>&1 | tail -5
```

Expected: build succeeds with no type errors.

- [ ] **Commit**

```bash
cd ..
git add web/src/api/client.ts
git commit -m "feat: add BaseRow type and api.bases client methods"
```

---

## Task 6: Create BasesTab.tsx

**Files:** Create `web/src/tabs/BasesTab.tsx`

- [ ] **Create the file**

```tsx
import { useState, useEffect } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { BaseRow } from '../api/client'

export default function BasesTab() {
  const [bases, setBases] = useState<BaseRow[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const data = await api.bases.list()
      setBases(data)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.danger(`Failed to load bases: ${msg}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: '16px' }}>
      <div className="flex items-center justify-between shrink-0">
        <div>
          <h2 className="text-lg font-semibold" style={{ color: 'var(--color-primary)' }}>
            Bases
          </h2>
          <p className="text-sm" style={{ color: 'var(--color-text-dim)' }}>
            Live in-world player bases. Export any base as a solido-compatible blueprint.
          </p>
        </div>
        <Button variant="outline" size="sm" onPress={load} isDisabled={loading}>
          {loading ? <Spinner size="sm" color="current" /> : null}
          Refresh
        </Button>
      </div>

      {loading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : (
        <div className="rounded-lg" style={{ flex: 1, minHeight: 0, overflowY: 'auto', border: '1px solid #2a2418' }}>
          <table className="w-full text-sm">
            <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: '#1a1610' }}>
              <tr style={{ borderBottom: '1px solid #2a2418' }}>
                {['ID', 'Name', 'Pieces', 'Placeables', 'Actions'].map(h => (
                  <th key={h} className="text-left px-4 py-2 font-semibold text-xs uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {bases.map((base, i) => (
                <tr key={base.id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#111009' }}>
                  <td className="px-4 py-2 font-mono text-xs" style={{ color: 'var(--color-text)' }}>{base.id}</td>
                  <td className="px-4 py-2 text-xs" style={{ color: 'var(--color-text)' }}>{base.name || '—'}</td>
                  <td className="px-4 py-2 text-xs" style={{ color: 'var(--color-text-dim)' }}>{base.pieces}</td>
                  <td className="px-4 py-2 text-xs" style={{ color: 'var(--color-text-dim)' }}>{base.placeables}</td>
                  <td className="px-4 py-2">
                    <a
                      href={api.bases.exportUrl(base.id)}
                      download={base.name ? `${base.name}.json` : `base-${base.id}.json`}
                    >
                      <Button size="sm" variant="outline">Export</Button>
                    </a>
                  </td>
                </tr>
              ))}
              {bases.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-sm" style={{ color: 'var(--color-text-dim)' }}>
                    No bases found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Verify TypeScript compiles**

```bash
cd web && npm run build 2>&1 | tail -5
```

Expected: success.

- [ ] **Commit**

```bash
cd ..
git add web/src/tabs/BasesTab.tsx
git commit -m "feat: add BasesTab with list and export"
```

---

## Task 7: Register tab in App.tsx

**Files:** Modify `web/src/App.tsx`

- [ ] **Add import at the top with the other tab imports (after line 10)**

```ts
import BasesTab from './tabs/BasesTab'
```

- [ ] **Add the tab label inside `<Tabs.List>` alongside the Blueprints tab (after line 222)**

```tsx
{isSignedIn && <Tabs.Tab id="bases">Bases<Tabs.Indicator /></Tabs.Tab>}
```

- [ ] **Add the tab panel alongside the Blueprints panel (after the closing `}` of the blueprints panel block)**

```tsx
{isSignedIn && (
  <Tabs.Panel id="bases" className="flex-1 overflow-hidden flex flex-col p-4">
    <BasesTab />
  </Tabs.Panel>
)}
```

- [ ] **Verify TypeScript compiles**

```bash
cd web && npm run build 2>&1 | tail -5
```

Expected: success.

- [ ] **Commit**

```bash
cd ..
git add web/src/App.tsx
git commit -m "feat: register Bases tab in App"
```

---

## Task 8: End-to-end verification

- [ ] **Run the backend (if not already running)**

```bash
./dune-admin &
```

- [ ] **Verify list endpoint**

```bash
curl -s http://localhost:8080/api/v1/bases | python3 -m json.tool
```

Expected: array with at least `{"id": 335, "name": "Air Crossroads PWR-SAVE", "pieces": 3554, "placeables": 308}`.

- [ ] **Verify export produces importable JSON**

```bash
curl -s http://localhost:8080/api/v1/bases/335/export -o /tmp/base335.json
python3 -c "
import json
d = json.load(open('/tmp/base335.json'))
assert 'instances' in d and 'placeables' in d and 'pentashields' in d
assert len(d['instances']) > 0
assert all(k in d['instances'][0] for k in ['building_type','x','y','z','rotation'])
assert all(k in d['placeables'][0] for k in ['building_type','x','y','z','rx','ry','rz'])
print('export structure ok')
print(f'  instances:    {len(d[\"instances\"])}')
print(f'  placeables:   {len(d[\"placeables\"])}')
print(f'  pentashields: {len(d[\"pentashields\"])}')
"
```

Expected:

```
export structure ok
  instances:    3554
  placeables:   308
  pentashields: <number>
```

- [ ] **Open the web UI and verify the Bases tab appears and loads**

Open `http://localhost:5173` (or wherever the dev frontend runs), sign in, click **Bases** — should show the table with base 335 and an Export button.

- [ ] **Final commit**

```bash
git add -A
git status  # verify nothing unexpected staged
git commit -m "feat: bases export — list and solido-compatible export complete"
```
