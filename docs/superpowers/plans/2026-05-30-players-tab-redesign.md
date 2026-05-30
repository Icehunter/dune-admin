# Players Tab Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sidebar-driven Players tab with a single unified panel: a compact, searchable player list at the top (showing up to 3 filtered results) and a full player detail view below with stat panels, a Solaris-over-time line chart, and a session-history bar chart.

**Architecture:** The existing sidebar and its five aggregate views (Currency, Factions, Specs, Online, Players) are deleted. The new layout is a single scrollable column: compact player cards (click to select) → stat panels (Economy / Progression / Sessions) → recharts charts (Solaris history, Session history). Two new backend endpoints serve the chart data; chart data is fetched lazily when a player is selected.

**Tech Stack:** Go (pgx/v5, `database/sql` + modernc.org/sqlite already present), React 18, TypeScript strict, HeroUI v3, recharts 2.x, Tailwind via CSS custom properties.

---

## File Map

### Delete (dead code after redesign)

- `web/src/tabs/PlayersTab/Sidebar.tsx`
- `web/src/tabs/PlayersTab/views/CurrencyView.tsx`
- `web/src/tabs/PlayersTab/views/FactionsView.tsx`
- `web/src/tabs/PlayersTab/views/SpecsView.tsx`
- `web/src/tabs/PlayersTab/views/OnlineView.tsx`
- `web/src/tabs/PlayersTab/views/PlayersListView.tsx`
- `web/src/tabs/PlayersTab/modals/StatsModal.tsx`

### Create

| File | Responsibility |
|---|---|
| `cmd/dune-admin/handlers_charts.go` | `handleGetSolarisHistory`, `handleGetSessionHistory` HTTP handlers |
| `cmd/dune-admin/handlers_charts_test.go` | nil-guard tests for both handlers |
| `cmd/dune-admin/sessions_history_test.go` | `getSessionHistory` unit tests against in-memory SQLite |
| `web/src/tabs/PlayersTab/components/PlayerCard.tsx` | Compact clickable player row (name, status dot, action buttons) |
| `web/src/tabs/PlayersTab/components/SolarisChart.tsx` | recharts `LineChart` wrapping solaris history data |
| `web/src/tabs/PlayersTab/components/SessionChart.tsx` | recharts `BarChart` wrapping per-day playtime data |
| `web/src/tabs/PlayersTab/components/PlayerDetailPanel.tsx` | Stat panels + charts for the selected player |

### Modify

| File | Change |
|---|---|
| `cmd/dune-admin/db.go` | Add `cmdFetchSolarisHistory` |
| `cmd/dune-admin/sessions.go` | Add `getSessionHistory` |
| `cmd/dune-admin/server.go` | Register two new GET routes |
| `web/src/api/client.ts` | Add `SolarisPoint`, `SessionRecord` types; add `api.players.solarisHistory`, `api.players.sessionHistory` |
| `web/src/tabs/PlayersTab/types.ts` | Remove `Sidebar` type, `SIDEBAR_ITEMS` const |
| `web/src/tabs/PlayersTab/index.tsx` | Full rewrite — new layout, no sidebar |

---

## Task 1 — Backend: `getSessionHistory` (SQLite, fully unit-testable)

**Files:**

- Create: `cmd/dune-admin/sessions_history_test.go`
- Modify: `cmd/dune-admin/sessions.go`

### Step 1 — Write the failing test

```go
// cmd/dune-admin/sessions_history_test.go
package main

import (
 "context"
 "testing"
)

type sessionRecord struct {
 StartedAt    string `json:"started_at"`
 EndedAt      string `json:"ended_at"`
 DurationSecs int64  `json:"duration_secs"`
}

func TestGetSessionHistory_ReturnsSortedClosedSessions(t *testing.T) {
 t.Parallel()
 db := openTestSessionDB(t)
 ctx := context.Background()

 db.ExecContext(ctx, `INSERT INTO play_sessions(account_id,started_at,ended_at,duration_secs) VALUES(42,'2026-01-01T10:00:00Z','2026-01-01T11:00:00Z',3600)`)
 db.ExecContext(ctx, `INSERT INTO play_sessions(account_id,started_at,ended_at,duration_secs) VALUES(42,'2026-01-02T10:00:00Z','2026-01-02T10:30:00Z',1800)`)
 // open session — must NOT appear
 db.ExecContext(ctx, `INSERT INTO play_sessions(account_id,started_at) VALUES(42,'2026-01-03T10:00:00Z')`)

 recs, err := getSessionHistory(ctx, db, 42, 50)
 if err != nil {
  t.Fatalf("getSessionHistory: %v", err)
 }
 if len(recs) != 2 {
  t.Fatalf("expected 2 closed sessions, got %d", len(recs))
 }
 // ascending order
 if recs[0].DurationSecs != 3600 {
  t.Errorf("first session: want 3600s, got %d", recs[0].DurationSecs)
 }
 if recs[1].DurationSecs != 1800 {
  t.Errorf("second session: want 1800s, got %d", recs[1].DurationSecs)
 }
}

func TestGetSessionHistory_Empty(t *testing.T) {
 t.Parallel()
 db := openTestSessionDB(t)
 recs, err := getSessionHistory(context.Background(), db, 999, 50)
 if err != nil {
  t.Fatalf("unexpected error: %v", err)
 }
 if len(recs) != 0 {
  t.Fatalf("expected 0 records, got %d", len(recs))
 }
}
```

- [ ] **Step 2 — Run; confirm FAIL** (`go test ./cmd/dune-admin/ -run TestGetSessionHistory -v`)

### Step 3 — Implement `getSessionHistory` in `sessions.go`

Add the `sessionRecord` struct and function **before** `startSessionPoller`:

```go
type sessionRecord struct {
 StartedAt    string `json:"started_at"`
 EndedAt      string `json:"ended_at"`
 DurationSecs int64  `json:"duration_secs"`
}

func getSessionHistory(ctx context.Context, db *sql.DB, accountID int64, limit int) ([]sessionRecord, error) {
 rows, err := db.QueryContext(ctx, `
  SELECT started_at, ended_at, duration_secs
  FROM play_sessions
  WHERE account_id = ? AND ended_at IS NOT NULL
  ORDER BY started_at ASC
  LIMIT ?
 `, accountID, limit)
 if err != nil {
  return nil, fmt.Errorf("query session history for account %d: %w", accountID, err)
 }
 defer func() { _ = rows.Close() }()

 var out []sessionRecord
 for rows.Next() {
  var r sessionRecord
  if err := rows.Scan(&r.StartedAt, &r.EndedAt, &r.DurationSecs); err != nil {
   return nil, fmt.Errorf("scan session record: %w", err)
  }
  out = append(out, r)
 }
 if out == nil {
  out = []sessionRecord{}
 }
 return out, rows.Err()
}
```

- [ ] **Step 4 — Run; confirm PASS** (`go test ./cmd/dune-admin/ -run TestGetSessionHistory -v`)

---

## Task 2 — Backend: `cmdFetchSolarisHistory` (Postgres)

**Files:**

- Modify: `cmd/dune-admin/db.go`

No practical unit test for the Postgres query — it requires a live DB. The nil-guard is tested in Task 3.

### Step 1 — Add struct + function to end of `db.go`

```go
type solarisPoint struct {
 Time    string `json:"time"`
 Balance int64  `json:"balance"`
}

// cmdFetchSolarisHistory returns timestamped solaris balance snapshots for a
// player. The hex FLS entity ID is derived via balance-match (same approach as
// cmdFetchPlayerPgStats). Returns at most 500 points in ascending time order.
func cmdFetchSolarisHistory(ctx context.Context, pool *pgxpool.Pool, accountID int64) ([]solarisPoint, error) {
 rows, err := pool.Query(ctx, `
  WITH fls_map AS (
   SELECT DISTINCT ON (meta->>'fls_id')
    meta->>'fls_id'                                    AS fls_id,
    ROUND((meta->>'solaris_balance')::float)::bigint   AS last_bal
   FROM dune.event_log
   WHERE meta->>'solaris_balance' IS NOT NULL
   ORDER BY meta->>'fls_id', event_time DESC
  )
  SELECT
   to_char(el.event_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
   ROUND((el.meta->>'solaris_balance')::float)::bigint
  FROM dune.event_log el
  JOIN fls_map fm ON fm.fls_id = el.meta->>'fls_id'
  JOIN dune.player_virtual_currency_balances pvc
      ON pvc.currency_id = 0 AND pvc.balance = fm.last_bal
  JOIN dune.player_state ps ON ps.player_controller_id = pvc.player_controller_id
  WHERE ps.account_id = $1 AND el.meta->>'solaris_balance' IS NOT NULL
  ORDER BY el.event_time ASC
  LIMIT 500
 `, accountID)
 if err != nil {
  return nil, fmt.Errorf("fetch solaris history for account %d: %w", accountID, err)
 }
 defer rows.Close()

 var out []solarisPoint
 for rows.Next() {
  var p solarisPoint
  if err := rows.Scan(&p.Time, &p.Balance); err != nil {
   return nil, fmt.Errorf("scan solaris point: %w", err)
  }
  out = append(out, p)
 }
 if out == nil {
  out = []solarisPoint{}
 }
 return out, rows.Err()
}
```

- [ ] **Step 2 — Verify compile** (`go build ./cmd/dune-admin/`)

---

## Task 3 — Backend: Handlers + Routes

**Files:**

- Create: `cmd/dune-admin/handlers_charts.go`
- Create: `cmd/dune-admin/handlers_charts_test.go`
- Modify: `cmd/dune-admin/server.go`

### Step 1 — Write failing handler tests

```go
// cmd/dune-admin/handlers_charts_test.go
package main

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestHandleGetSolarisHistory_DBNil(t *testing.T) {
 orig := globalDB
 globalDB = nil
 defer func() { globalDB = orig }()

 req := httptest.NewRequest(http.MethodGet, "/", nil)
 req.SetPathValue("id", "42")
 rr := httptest.NewRecorder()
 handleGetSolarisHistory(rr, req)

 if rr.Code != http.StatusServiceUnavailable {
  t.Fatalf("want 503, got %d", rr.Code)
 }
}

func TestHandleGetSessionHistory_SessionDBNil(t *testing.T) {
 origDB := globalDB
 origSDB := globalSessionDB
 globalDB = nil
 globalSessionDB = nil
 defer func() {
  globalDB = origDB
  globalSessionDB = origSDB
 }()

 req := httptest.NewRequest(http.MethodGet, "/", nil)
 req.SetPathValue("id", "42")
 rr := httptest.NewRecorder()
 handleGetSessionHistory(rr, req)

 if rr.Code != http.StatusServiceUnavailable {
  t.Fatalf("want 503, got %d", rr.Code)
 }
}
```

- [ ] **Step 2 — Run; confirm FAIL** (`go test ./cmd/dune-admin/ -run TestHandle.*History -v`)

### Step 3 — Implement handlers

```go
// cmd/dune-admin/handlers_charts.go
package main

import (
 "fmt"
 "log"
 "net/http"
 "strconv"
)

func handleGetSolarisHistory(w http.ResponseWriter, r *http.Request) {
 if globalDB == nil {
  jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
  return
 }
 accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
 if err != nil {
  jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
  return
 }
 pts, err := cmdFetchSolarisHistory(r.Context(), globalDB, accountID)
 if err != nil {
  log.Printf("handleGetSolarisHistory: %v", err)
  jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
  return
 }
 jsonOK(w, pts)
}

func handleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
 if globalDB == nil {
  jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
  return
 }
 if globalSessionDB == nil {
  jsonErr(w, fmt.Errorf("session tracker not available"), http.StatusServiceUnavailable)
  return
 }
 accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
 if err != nil {
  jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
  return
 }
 recs, err := getSessionHistory(r.Context(), globalSessionDB, accountID, 200)
 if err != nil {
  log.Printf("handleGetSessionHistory: %v", err)
  jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
  return
 }
 jsonOK(w, recs)
}
```

### Step 4 — Register routes in `server.go`

After the existing `handleGetPlayerStats` line:

```go
mux.HandleFunc("GET /api/v1/players/{id}/stats", handleGetPlayerStats)
mux.HandleFunc("GET /api/v1/players/{id}/solaris-history", handleGetSolarisHistory)
mux.HandleFunc("GET /api/v1/players/{id}/session-history", handleGetSessionHistory)
```

- [ ] **Step 5 — Run; confirm PASS + verify** (`make verify`)

---

## Task 4 — Frontend: Install recharts + API types

**Files:**

- Modify: `web/src/api/client.ts`

### Step 1 — Install recharts

```bash
cd web && pnpm add recharts
```

Expected: recharts added to `package.json` dependencies.

### Step 2 — Add types and API methods to `client.ts`

After the existing `PlayerStats` type block, add:

```ts
export type SolarisPoint = {
  time: string      // ISO-8601 UTC string "2026-05-25T11:41:39Z"
  balance: number
}

export type SessionRecord = {
  started_at: string
  ended_at: string
  duration_secs: number
}
```

In the `api.players` namespace, after `stats`:

```ts
solarisHistory: (id: number) => req<SolarisPoint[]>('GET', `/players/${id}/solaris-history`),
sessionHistory: (id: number) => req<SessionRecord[]>('GET', `/players/${id}/session-history`),
```

- [ ] **Step 3 — Verify lint** (`pnpm lint`)

---

## Task 5 — Frontend: `PlayerCard` component

**Files:**

- Create: `web/src/tabs/PlayersTab/components/PlayerCard.tsx`

This is the compact clickable row in the top player list.

```tsx
// web/src/tabs/PlayersTab/components/PlayerCard.tsx
import { Button } from '@heroui/react'
import type { Player } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import { StatusDot } from './StatusDot'

interface Props {
  player: Player
  selected: boolean
  onSelect: (player: Player) => void
  onAction: (player: Player, action: 'inventory' | 'give' | 'actions') => void
}

export function PlayerCard({ player, selected, onSelect, onAction }: Props) {
  return (
    <button
      type="button"
      onClick={() => onSelect(player)}
      className={[
        'w-full text-left px-3 py-2 rounded-[var(--radius)] flex items-center gap-3',
        'border transition-colors cursor-pointer',
        selected
          ? 'bg-surface border-accent/60'
          : 'bg-surface-secondary border-border hover:border-border/80 hover:bg-surface',
      ].join(' ')}
    >
      <StatusDot status={player.online_status} />
      <div className="flex-1 min-w-0">
        <div className="font-semibold text-sm truncate">{player.name}</div>
        <div className="text-xs text-muted truncate">{player.class} · {player.map}</div>
      </div>
      <div className="flex gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'inventory')}>
          <Icon name="package" />
        </Button>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'give')}>
          <Icon name="gift" />
        </Button>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'actions')}>
          <Icon name="settings" />
        </Button>
      </div>
    </button>
  )
}
```

- [ ] **Step 2 — Verify lint** (`pnpm lint`)

---

## Task 6 — Frontend: `SolarisChart` component

**Files:**

- Create: `web/src/tabs/PlayersTab/components/SolarisChart.tsx`

```tsx
// web/src/tabs/PlayersTab/components/SolarisChart.tsx
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import type { SolarisPoint } from '../../../api/client'
import { SectionLabel } from '../../../dune-ui'

interface Props {
  data: SolarisPoint[]
}

function fmtBalance(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function SolarisChart({ data }: Props) {
  if (data.length === 0) {
    return (
      <div>
        <SectionLabel>Solaris History</SectionLabel>
        <p className="text-muted text-sm mt-2">No economy events recorded yet.</p>
      </div>
    )
  }

  return (
    <div>
      <SectionLabel>Solaris History</SectionLabel>
      <div className="mt-3 h-48">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <XAxis
              dataKey="time"
              tickFormatter={fmtTime}
              tick={{ fontSize: 11, fill: 'var(--color-muted)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={fmtBalance}
              tick={{ fontSize: 11, fill: 'var(--color-muted)' }}
              tickLine={false}
              axisLine={false}
              width={48}
            />
            <Tooltip
              formatter={(val: number) => [fmtBalance(val), 'Balance']}
              labelFormatter={fmtTime}
              contentStyle={{
                background: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius)',
                fontSize: 12,
              }}
            />
            <Line
              type="monotone"
              dataKey="balance"
              stroke="var(--color-accent)"
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
```

- [ ] **Step 2 — Verify lint** (`pnpm lint`)

---

## Task 7 — Frontend: `SessionChart` component

**Files:**

- Create: `web/src/tabs/PlayersTab/components/SessionChart.tsx`

Session data is aggregated by calendar date before charting (multiple sessions on one day are summed).

```tsx
// web/src/tabs/PlayersTab/components/SessionChart.tsx
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import type { SessionRecord } from '../../../api/client'
import { SectionLabel } from '../../../dune-ui'

interface Props {
  data: SessionRecord[]
}

type DayBucket = { date: string, minutes: number }

function aggregate(records: SessionRecord[]): DayBucket[] {
  const map = new Map<string, number>()
  for (const r of records) {
    const day = r.started_at.slice(0, 10)
    map.set(day, (map.get(day) ?? 0) + Math.round(r.duration_secs / 60))
  }
  return Array.from(map.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, minutes]) => ({ date, minutes }))
}

function fmtDate(d: string): string {
  return new Date(d + 'T12:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function SessionChart({ data }: Props) {
  const buckets = aggregate(data)

  if (buckets.length === 0) {
    return (
      <div>
        <SectionLabel>Session History</SectionLabel>
        <p className="text-muted text-sm mt-2">
          Session data accumulates as players log in. The tracker polls every 5 minutes.
        </p>
      </div>
    )
  }

  return (
    <div>
      <SectionLabel>Session History</SectionLabel>
      <div className="mt-3 h-40">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={buckets} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <XAxis
              dataKey="date"
              tickFormatter={fmtDate}
              tick={{ fontSize: 11, fill: 'var(--color-muted)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              unit="m"
              tick={{ fontSize: 11, fill: 'var(--color-muted)' }}
              tickLine={false}
              axisLine={false}
              width={36}
            />
            <Tooltip
              formatter={(val: number) => [`${val}m`, 'Playtime']}
              labelFormatter={fmtDate}
              contentStyle={{
                background: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius)',
                fontSize: 12,
              }}
            />
            <Bar dataKey="minutes" fill="var(--color-accent)" radius={[3, 3, 0, 0]} maxBarSize={32} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
```

- [ ] **Step 2 — Verify lint** (`pnpm lint`)

---

## Task 8 — Frontend: `PlayerDetailPanel` component

**Files:**

- Create: `web/src/tabs/PlayersTab/components/PlayerDetailPanel.tsx`

This is the full detail view. It fetches stats + chart data when `player` changes.

```tsx
// web/src/tabs/PlayersTab/components/PlayerDetailPanel.tsx
import { useEffect, useState } from 'react'
import { Spinner } from '@heroui/react'
import { api } from '../../../api/client'
import type { Player, PlayerStats, SolarisPoint, SessionRecord } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { SolarisChart } from './SolarisChart'
import { SessionChart } from './SessionChart'

interface Props {
  player: Player
}

function fmtSolaris(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtDuration(s: number): string {
  if (s <= 0) return '—'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

function StatRow({ label, value }: { label: string, value: string | number }) {
  return (
    <div className="flex items-center justify-between py-1 border-b border-border/30 last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  )
}

export function PlayerDetailPanel({ player }: Props) {
  const [stats, setStats] = useState<PlayerStats | null>(null)
  const [solaris, setSolaris] = useState<SolarisPoint[]>([])
  const [sessions, setSessions] = useState<SessionRecord[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([
        api.players.stats(player.account_id),
        api.players.solarisHistory(player.account_id),
        api.players.sessionHistory(player.account_id),
      ]))
      .then(([s, sol, sess]) => {
        setStats(s)
        setSolaris(sol)
        setSessions(sess)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [player.account_id])

  if (loading) {
    return <div className="flex justify-center py-12"><Spinner size="lg" /></div>
  }

  if (!stats) {
    return <p className="text-muted text-sm py-4 text-center">Failed to load stats.</p>
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Stat panels — three columns */}
      <div className="grid grid-cols-3 gap-3">
        <Panel>
          <SectionLabel>Economy</SectionLabel>
          <div className="mt-2">
            <StatRow label="Solaris" value={fmtSolaris(stats.solaris_balance)} />
            <StatRow label="Scrip" value={fmtSolaris(stats.scrip_balance)} />
            <StatRow label="Earned" value={stats.solaris_earned > 0 ? fmtSolaris(stats.solaris_earned) : '—'} />
            <StatRow label="Spent" value={stats.solaris_spent > 0 ? fmtSolaris(stats.solaris_spent) : '—'} />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>Progression</SectionLabel>
          <div className="mt-2">
            <StatRow label="Char XP" value={stats.char_xp > 0 ? stats.char_xp.toLocaleString() : '—'} />
            <StatRow label="Skill pts" value={stats.skill_points > 0 ? stats.skill_points : '—'} />
            <StatRow label="POIs" value={stats.pois_discovered > 0 ? stats.pois_discovered : '—'} />
            <StatRow label="Milestones" value={stats.story_milestones > 0 ? stats.story_milestones : '—'} />
            <StatRow
              label="Faction tier"
              value={stats.max_faction_tier > 0 ? `Tier ${stats.max_faction_tier}` : '—'}
            />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>Sessions</SectionLabel>
          <div className="mt-2">
            <StatRow label="Playtime" value={fmtDuration(stats.total_playtime_secs)} />
            <StatRow label="Count" value={stats.session_count > 0 ? stats.session_count : '—'} />
            <StatRow label="Avg" value={fmtDuration(stats.avg_session_secs)} />
            <StatRow
              label="Last seen"
              value={stats.last_seen ? new Date(stats.last_seen as string).toLocaleDateString() : '—'}
            />
          </div>
        </Panel>
      </div>

      {/* Charts */}
      <Panel>
        <SolarisChart data={solaris} />
      </Panel>

      <Panel>
        <SessionChart data={sessions} />
      </Panel>
    </div>
  )
}
```

- [ ] **Step 2 — Verify lint** (`pnpm lint`)

---

## Task 9 — Frontend: Rewrite `PlayersTab/index.tsx` + cleanup

**Files:**

- Modify: `web/src/tabs/PlayersTab/index.tsx` (full rewrite)
- Modify: `web/src/tabs/PlayersTab/types.ts` (remove sidebar items)
- Delete: `Sidebar.tsx`, `views/CurrencyView.tsx`, `views/FactionsView.tsx`, `views/SpecsView.tsx`, `views/OnlineView.tsx`, `views/PlayersListView.tsx`, `modals/StatsModal.tsx`

### Step 1 — Clean `types.ts`

Remove the `Sidebar` type, `SIDEBAR_ITEMS` const. Keep `ActionSection`, `ACTION_SECTIONS`, `PlayerSortKey`, `PLAYER_COLUMNS`, `PacksData`, `XP_TRACKS`, `FACTIONS`.

```ts
// Remove these two:
// export type Sidebar = 'players' | 'currency' | 'factions' | 'specs' | 'online'
// export const SIDEBAR_ITEMS = [...]
```

### Step 2 — Rewrite `index.tsx`

```tsx
// web/src/tabs/PlayersTab/index.tsx
import { useState, useEffect, useMemo, useCallback } from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { api } from '../../api/client'
import type { Player } from '../../api/client'

import { Icon, PageHeader } from '../../dune-ui'
import { PlayerCard } from './components/PlayerCard'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { StatusDot } from './components/StatusDot'
import { InventoryModal } from './modals/InventoryModal'
import { GiveItemsModal } from './modals/GiveItemsModal'
import { PlayerActionsModal } from './modals/PlayerActionsModal'

const MAX_VISIBLE = 5

export default function PlayersTab() {
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Player | null>(null)

  // Modal state
  const [showInventory, setShowInventory] = useState(false)
  const [showGive, setShowGive] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const [modalPlayer, setModalPlayer] = useState<Player | null>(null)

  const loadPlayers = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.list())
      .then((list) => {
        setPlayers(list)
        // Auto-select first player if none selected
        setSelected((prev) => prev ?? list[0] ?? null)
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { loadPlayers() }, [loadPlayers])

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    const list = q
      ? players.filter((p) =>
          p.name.toLowerCase().includes(q)
          || p.class.toLowerCase().includes(q)
          || p.map.toLowerCase().includes(q),
        )
      : players
    return list.slice(0, MAX_VISIBLE)
  }, [players, search])

  const onlineCount = useMemo(
    () => players.filter((p) => p.online_status === 'Online').length,
    [players],
  )

  const openModal = (player: Player, action: 'inventory' | 'give' | 'actions') => {
    setModalPlayer(player)
    if (action === 'inventory') setShowInventory(true)
    else if (action === 'give') setShowGive(true)
    else setShowActions(true)
  }

  return (
    <div className="flex flex-col gap-4 h-full min-h-0 overflow-y-auto">
      {/* Header */}
      <PageHeader title="Players" onRefresh={loadPlayers} loading={loading}>
        <div className="flex items-center gap-2 text-sm text-muted">
          <StatusDot status={onlineCount > 0 ? 'Online' : 'Offline'} />
          {onlineCount}
          {' '}
          online
        </div>
      </PageHeader>

      {/* Compact player list */}
      <div className="flex flex-col gap-2 shrink-0">
        <SearchField
          aria-label="Search players"
          className="w-full"
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder="Filter players..." />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>

        {loading && players.length === 0
          ? <div className="flex justify-center py-4"><Spinner size="sm" /></div>
          : filtered.length === 0
            ? <p className="text-muted text-sm text-center py-2">No players found</p>
            : filtered.map((p) => (
                <PlayerCard
                  key={p.id}
                  player={p}
                  selected={selected?.id === p.id}
                  onSelect={setSelected}
                  onAction={openModal}
                />
              ))}

        {players.length > MAX_VISIBLE && !search && (
          <p className="text-muted text-xs text-center">
            Showing
            {' '}
            {MAX_VISIBLE}
            {' of '}
            {players.length}
            {' — use search to filter'}
          </p>
        )}
      </div>

      {/* Detail panel */}
      {selected
        ? (
            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-2 mb-1">
                <span className="font-semibold text-accent">{selected.name}</span>
                <StatusDot status={selected.online_status} />
                <span className="text-muted text-xs">{selected.online_status}</span>
              </div>
              <PlayerDetailPanel player={selected} />
            </div>
          )
        : (
            <p className="text-muted text-sm text-center py-8">Select a player above</p>
          )}

      {/* Modals */}
      {modalPlayer && (
        <>
          <InventoryModal player={modalPlayer} open={showInventory} onClose={() => setShowInventory(false)} />
          <GiveItemsModal player={modalPlayer} open={showGive} onClose={() => setShowGive(false)} />
          <PlayerActionsModal player={modalPlayer} open={showActions} onClose={() => setShowActions(false)} />
        </>
      )}
    </div>
  )
}
```

### Step 3 — Delete dead files

```bash
cd web/src/tabs/PlayersTab
rm Sidebar.tsx
rm views/CurrencyView.tsx views/FactionsView.tsx views/SpecsView.tsx views/OnlineView.tsx views/PlayersListView.tsx
rm modals/StatsModal.tsx
```

### Step 4 — Verify

```bash
cd web && pnpm lint
make verify  # from project root
```

Expected: all checks pass, no references to deleted files remain.

---

## Self-Review

**Spec coverage:**

- ✅ Remove sidebar aggregate views → Tasks 9 deletes them
- ✅ Compact player list at top with search, top N shown → Task 9 index.tsx
- ✅ Click player to select → PlayerCard `onSelect`, index.tsx state
- ✅ Online count badge → `onlineCount` in header  
- ✅ Action buttons per player (Inventory, Give, Actions) → PlayerCard + openModal
- ✅ Stat panels (Economy / Progression / Sessions) → PlayerDetailPanel 3-column grid
- ✅ Solaris history chart → Tasks 2, 3, 6, 8
- ✅ Session history chart → Tasks 1, 3, 7, 8
- ✅ recharts added → Task 4
- ✅ StatsModal removed (inline now) → Task 9 delete step
- ✅ `make verify` + `pnpm lint` gating every backend/frontend task

**Placeholder scan:** None found — all code blocks are complete.

**Type consistency:**

- `SolarisPoint` defined in Task 4, used in Tasks 6, 8 ✅
- `SessionRecord` defined in Task 4, used in Tasks 7, 8 ✅
- `sessionRecord` (Go) defined in Task 1, returned by `getSessionHistory`, serialized by handler Task 3 ✅
- `solarisPoint` (Go) defined in Task 2, returned by `cmdFetchSolarisHistory`, serialized in Task 3 ✅
- `PlayerDetailPanel` expects `player: Player` (has `account_id`) ✅ — all chart/stats calls use `player.account_id`
