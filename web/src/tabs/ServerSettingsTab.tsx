import { useState, useEffect, useCallback, useRef, Fragment } from 'react'
import { Button, ListBox, SearchField, Select, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { ServerSetting, ServerSettingUpdate, RawSection } from '../api/client'
import { PageHeader, Panel, SectionLabel, Icon } from '../dune-ui'

const CATEGORY_ORDER = [
  'Survival', 'Progression', 'Harvesting', 'Building', 'Inventory',
  'Guilds & Economy', 'Storm Cycle', 'PvP & Security', 'Spice', 'Taxation', 'Sandworm',
]

// CATEGORY_ICONS picks a small set of lucide icon names that map to each
// category so the grid of category cards is scannable by glance. Unknown
// categories fall back to "sliders".
const CATEGORY_ICONS: Record<string, string> = {
  'Survival': 'heart-pulse',
  'Progression': 'trending-up',
  'Harvesting': 'pickaxe',
  'Building': 'home',
  'Inventory': 'package',
  'Guilds & Economy': 'coins',
  'Storm Cycle': 'wind',
  'PvP & Security': 'shield',
  'Spice': 'sparkles',
  'Taxation': 'receipt',
  'Sandworm': 'worm',
}

// COMMON_KEYS is the curated list of settings most admins want to touch
// regularly. Rendered at the top of the page so they don't need to drill
// into a category. Other settings live in the per-category grid below.
//
// Each entry is "section|key" — same shape as pendingKey().
const COMMON_KEYS = new Set([
  '/Script/DuneSandbox.DuneGameMode|m_GlobalXPMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalHealthMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalDamageToNpcsMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalDamageToPlayersMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalHarvestAmountMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_WaterConsumptionRate',
  '/Script/DuneSandbox.PvpPveSettings|bPvPEnabled',
  '/Script/DuneSandbox.PvpPveSettings|bServerPVE',
  '/Script/DuneSandbox.SandStormConfig|m_StormCycleDuration',
  '/Script/DuneSandbox.InventorySystemSettings|PlayerInventoryStartingSize',
  '/Script/DuneSandbox.BuildingSettings|m_MaxNumLandclaimSegments',
  '/Script/DuneSandbox.GuildSettings|m_MaxGuildMembersAllowed',
  '/DeteriorationSystem.ItemDeteriorationConstants|m_ItemDurabilityLossMultiplier',
  '/Script/DuneSandbox.SpiceHarvestingSystem|m_bSpawningActive',
])

const SOURCE_FILE: Record<string, string> = {
  defaultGame: 'DefaultGame.ini',
  defaultEngine: 'DefaultEngine.ini',
  userGame: 'UserGame.ini',
  userEngine: 'UserEngine.ini',
}

const LAYER_STYLE: Record<string, { cls: string }> = {
  defaultGame: { cls: 'text-muted/60' },
  defaultEngine: { cls: 'text-muted/60' },
  userEngine: { cls: 'text-foreground/70' },
  userGame: { cls: 'text-warning' },
}

const SOURCE_PRIORITY = ['defaultGame', 'defaultEngine', 'userEngine', 'userGame'] as const

function groupByCategory(items: ServerSetting[]) {
  const map = new Map<string, ServerSetting[]>()
  for (const item of items) {
    const arr = map.get(item.category) ?? []
    arr.push(item)
    map.set(item.category, arr)
  }
  const ordered: [string, ServerSetting[]][] = []
  for (const cat of CATEGORY_ORDER) {
    if (map.has(cat)) ordered.push([cat, map.get(cat)!])
  }
  for (const [cat, items] of map) {
    if (!CATEGORY_ORDER.includes(cat)) ordered.push([cat, items])
  }
  return ordered
}

function sourceLabel(s: string) {
  const file = SOURCE_FILE[s]
  const style = LAYER_STYLE[s]
  if (!file || !style) return null
  return { text: file, cls: style.cls }
}

function shortSection(section: string) {
  // "/Script/DuneSandbox.BuildingSettings" → "BuildingSettings"
  const dot = section.lastIndexOf('.')
  return dot >= 0 ? section.slice(dot + 1) : section
}

// matchesSetting returns true when the lower-cased query appears in any of the
// fields an admin would reasonably search by. Empty query matches everything.
function matchesSetting(item: ServerSetting, q: string): boolean {
  if (!q) return true
  return (
    item.label.toLowerCase().includes(q)
    || item.description.toLowerCase().includes(q)
    || item.key.toLowerCase().includes(q)
    || item.category.toLowerCase().includes(q)
    || shortSection(item.section).toLowerCase().includes(q)
  )
}

// matchesRawSection matches on the INI section name or any contained key/value.
function matchesRawSection(sections: RawSection[], q: string): boolean {
  if (!q) return true
  if (shortSection(sections[0].section).toLowerCase().includes(q)) return true
  return sections.some((sec) =>
    sec.lines.some((l) =>
      l.key.toLowerCase().includes(q) || l.value.toLowerCase().includes(q),
    ),
  )
}

function SettingRow({
  item, pending, onChange, onDelete,
}: {
  item: ServerSetting
  pending: string | undefined
  onChange: (value: string) => void
  onDelete: () => Promise<void>
}) {
  const rawDisplay = pending !== undefined ? pending : item.current
  const display = item.type === 'bool'
    ? (/^(true|1|yes)$/i.test(rawDisplay) ? 'True' : /^(false|0|no)$/i.test(rawDisplay) ? 'False' : rawDisplay)
    : rawDisplay
  const dirty = pending !== undefined && rawDisplay !== item.current
  const src = sourceLabel(item.source)

  return (
    <div className="flex items-start gap-3 py-2.5 border-b border-border/40 last:border-0">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium text-foreground">{item.label}</span>
          {src && <span className={`text-xs ${src.cls}`}>{src.text}</span>}
          {dirty && <span className="text-xs text-warning">unsaved</span>}
        </div>
        <p className="text-xs text-muted mt-0.5 leading-relaxed">{item.description}</p>
        {item.layers.length > 1 && (
          <div className="flex items-center gap-1 mt-1.5 flex-wrap">
            {item.layers.map((layer, i) => {
              const style = LAYER_STYLE[layer.source] ?? { cls: 'text-muted' }
              const isActive = i === item.layers.length - 1
              return (
                <span key={layer.source} className="flex items-center gap-1">
                  <span className={`text-xs font-mono px-1.5 py-0.5 rounded border border-border/30 bg-surface/60 ${style.cls} ${isActive ? 'font-semibold' : 'opacity-50'}`}>
                    {SOURCE_FILE[layer.source] ?? layer.source}
                    :
                    {trimFloat(layer.value)}
                    {isActive ? ' ✓' : ''}
                  </span>
                  {i < item.layers.length - 1 && (
                    <span className="text-muted/30 text-xs select-none">→</span>
                  )}
                </span>
              )
            })}
          </div>
        )}
      </div>

      <div className="flex items-center gap-1.5 shrink-0">
        {item.type === 'bool'
          ? (
              <Select selectedKey={display} onSelectionChange={(k) => onChange(String(k))} className="w-32">
                <Select.Trigger className="h-7 text-xs">
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    <ListBox.Item id="True" textValue="True">
                      True
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="False" textValue="False">
                      False
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  </ListBox>
                </Select.Popover>
              </Select>
            )
          : item.type === 'string'
            ? (
                <input
                  type="text"
                  value={display}
                  onChange={(e) => onChange(e.target.value)}
                  className="w-40 bg-surface border border-border rounded px-2 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60"
                />
              )
            : (
                <input
                  type="number"
                  step={item.type === 'float' ? '0.01' : '1'}
                  value={display}
                  onChange={(e) => onChange(e.target.value)}
                  className="w-28 bg-surface border border-border rounded px-2 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 text-right"
                />
              )}
        {(item.source === 'userGame' || item.source === 'userEngine') && (
          <button
            onClick={onDelete}
            title={`Remove from ${SOURCE_FILE[item.source]}`}
            className="text-muted/50 hover:text-danger transition-colors"
          >
            <Icon name="trash-2" className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    </div>
  )
}

function linesToText(lines: RawSection['lines']) {
  return lines.map((l) => `${l.prefix}${l.key}=${l.value}`).join('\n')
}

// Trim Go's 6-decimal float formatting: "500.000000" → "500", "0.300000" → "0.3"
function trimFloat(v: string): string {
  if (!v.includes('.')) return v
  const n = parseFloat(v)
  return isNaN(n) ? v : n.toString()
}

function groupLinesByKey(lines: RawSection['lines']) {
  const grouped: { key: string, lines: typeof lines }[] = []
  const seen = new Map<string, number>()
  for (const line of lines) {
    const idx = seen.get(line.key)
    if (idx !== undefined) {
      grouped[idx].lines.push(line)
    }
    else {
      seen.set(line.key, grouped.length)
      grouped.push({ key: line.key, lines: [line] })
    }
  }
  return grouped
}

// One panel per INI section name, merging all source files that contain it.
function RawSectionPanel({ sections, onSaved }: { sections: RawSection[], onSaved: () => void }) {
  const sectionName = sections[0].section
  // Find the active user-writable source for this section (userGame or userEngine).
  const userSec = sections.find((s) => s.source === 'userGame')
    ?? sections.find((s) => s.source === 'userEngine')

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [collapsed, setCollapsed] = useState(true)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const toggle = () => {
    // While editing, the header click stays a no-op so users don't lose
    // their draft by accident. Cancel first if they want to collapse.
    if (editing) return
    setCollapsed((v) => !v)
  }

  const startEdit = () => {
    setDraft(userSec ? linesToText(userSec.lines) : '')
    setEditing(true)
    setTimeout(() => textareaRef.current?.focus(), 0)
  }

  const cancel = () => setEditing(false)

  const save = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, draft)
      toast.success(`Saved to ${userSec ? SOURCE_FILE[userSec.source] : 'UserGame.ini'}`)
      setEditing(false)
      onSaved()
    }
    catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setSaving(false)
    }
  }

  const deleteUserEntry = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, '')
      toast.success(`Removed from ${userSec ? SOURCE_FILE[userSec.source] : 'UserGame.ini'}`)
      onSaved()
    }
    catch (e: unknown) {
      toast.danger(`Delete failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setSaving(false)
    }
  }

  // Sort sources low → high priority for display
  const sorted = [...sections].sort((a, b) => {
    const ai = SOURCE_PRIORITY.indexOf(a.source as typeof SOURCE_PRIORITY[number])
    const bi = SOURCE_PRIORITY.indexOf(b.source as typeof SOURCE_PRIORITY[number])
    return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi)
  })
  const multiSource = sorted.length > 1

  return (
    <Panel>
      <div
        className={`flex items-center gap-2 flex-wrap ${collapsed && !editing ? 'cursor-pointer select-none' : 'mb-2'}`}
        onClick={collapsed && !editing ? toggle : undefined}
      >
        <Icon
          name={collapsed && !editing ? 'chevron-right' : 'chevron-down'}
          className="w-4 h-4 shrink-0 text-muted/70"
        />
        <SectionLabel>{shortSection(sectionName)}</SectionLabel>
        {sorted.map((s) => (
          <span key={s.source} className={`text-xs ${LAYER_STYLE[s.source]?.cls ?? 'text-muted'}`}>
            {SOURCE_FILE[s.source] ?? s.source}
          </span>
        ))}
        {userSec && collapsed && !editing && (
          <span className="text-xs text-warning">· user override</span>
        )}
        <div
          className="ml-auto flex items-center gap-1 min-w-[2rem]"
          onClick={(e) => e.stopPropagation()}
        >
          {editing
            ? (
                <>
                  <Button size="sm" variant="ghost" onPress={cancel} isDisabled={saving}>Cancel</Button>
                  <Button size="sm" onPress={save} isDisabled={saving}>
                    {saving ? <Spinner size="sm" color="current" /> : 'Save'}
                  </Button>
                </>
              )
            : !collapsed && (
                <>
                  {userSec && (
                    <button
                      onClick={deleteUserEntry}
                      title={`Remove from ${SOURCE_FILE[userSec.source]}`}
                      className="text-muted/50 hover:text-danger transition-colors"
                      disabled={saving}
                    >
                      <Icon name="trash-2" className="w-3.5 h-3.5" />
                    </button>
                  )}
                  <Button size="sm" variant="ghost" onPress={startEdit} isDisabled={saving}>
                    <Icon name="pencil" className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => setCollapsed(true)}
                    aria-label="Collapse section"
                  >
                    <Icon name="x" className="w-3.5 h-3.5" />
                  </Button>
                </>
              )}
        </div>
      </div>

      {!collapsed && (editing
        ? (
            <textarea
              ref={textareaRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={Math.max(4, draft.split('\n').length + 1)}
              className="w-full bg-surface border border-border rounded px-3 py-2 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 resize-y"
              spellCheck={false}
              placeholder="Key=Value or +Key=Value for array entries"
            />
          )
        : (
            <div className="flex flex-col gap-2">
              {sorted.map((sec) => {
                const style = LAYER_STYLE[sec.source] ?? { cls: 'text-muted' }
                const isActive = sec.source === sorted[sorted.length - 1].source
                return (
                  <div
                    key={sec.source}
                    className={multiSource ? `pl-2 border-l-2 ${isActive ? 'border-accent/40' : 'border-border/30'}` : ''}
                  >
                    {multiSource && (
                      <span className={`text-xs ${style.cls} block mb-1`}>
                        {SOURCE_FILE[sec.source] ?? sec.source}
                        {isActive ? ' ✓' : ''}
                      </span>
                    )}
                    <div className="flex flex-col gap-0.5">
                      {groupLinesByKey(sec.lines).map(({ key, lines }) => (
                        <div key={key} className="py-1 border-b border-border/30 last:border-0">
                          <span className="text-xs font-mono text-muted">{key}</span>
                          {lines.map((l, i) => (
                            <div key={i} className="flex items-baseline gap-1.5 mt-0.5 ml-3">
                              {l.prefix && (
                                <span className={`text-xs font-mono w-3 shrink-0 ${l.prefix === '+' ? 'text-success' : 'text-danger'}`}>
                                  {l.prefix}
                                </span>
                              )}
                              <span className={`text-xs font-mono break-all ${isActive ? 'text-foreground/80' : 'text-muted/50'}`}>{l.value}</span>
                            </div>
                          ))}
                        </div>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
    </Panel>
  )
}

const USER_SOURCES = new Set(['userGame', 'userEngine'])

export default function ServerSettingsTab() {
  const [items, setItems] = useState<ServerSetting[]>([])
  const [raw, setRaw] = useState<RawSection[]>([])
  const [pending, setPending] = useState<Map<string, string>>(new Map())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [showAll, setShowAll] = useState(() =>
    localStorage.getItem('serverSettings.showAll') === 'true',
  )
  const [expandedCategory, setExpandedCategory] = useState<string | null>(() =>
    localStorage.getItem('serverSettings.expandedCategory') || null,
  )

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setError(null)
      })
      .then(() => api.serverSettings.get())
      .then((data) => {
        setItems(data.settings ?? [])
        setRaw(data.raw ?? [])
        setPending(new Map())
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const pendingKey = (item: ServerSetting) => `${item.section}|${item.key}`

  const handleChange = (item: ServerSetting, value: string) => {
    setPending((prev) => {
      const n = new Map(prev)
      n.set(pendingKey(item), value)
      return n
    })
  }

  const handleDelete = async (item: ServerSetting) => {
    try {
      await api.serverSettings.update([{ section: item.section, key: item.key, value: '' }])
      toast.success(`Removed from ${SOURCE_FILE[item.source] ?? item.source}`)
      load()
    }
    catch (e: unknown) {
      toast.danger(`Delete failed: ${e instanceof Error ? e.message : String(e)}`)
    }
  }

  const save = async () => {
    const updates: ServerSettingUpdate[] = []
    for (const [k, v] of pending) {
      const [section, key] = k.split('|')
      updates.push({ section, key, value: v })
    }
    if (updates.length === 0) return
    setSaving(true)
    try {
      const res = await api.serverSettings.update(updates)
      toast.success(res.ok)
      load()
    }
    catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setSaving(false)
    }
  }

  const dirtyCount = pending.size

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">Loading settings…</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col h-full gap-3">
        <PageHeader title="Server Settings" />
        <div className="rounded px-4 py-3 text-sm bg-danger/10 border border-danger/40 text-danger">
          {error.includes('server_ini_dir') || error.includes('ini dir')
            ? `Could not locate server INI files: ${error}. For kubectl, ensure the game server PVC is mounted. For docker/local, add server_ini_dir to ~/.dune-admin/config.yaml.`
            : error}
        </div>
      </div>
    )
  }

  const toggleShowAll = () => setShowAll((v) => {
    localStorage.setItem('serverSettings.showAll', String(!v))
    return !v
  })

  // In "user settings" mode, show only items that have at least one value
  // from a user-controlled file (userGame / userEngine).
  const visibleItems = showAll
    ? items
    : items.filter((item) => item.layers.some((l) => USER_SOURCES.has(l.source)))

  const q = search.trim().toLowerCase()
  const searching = q.length > 0

  // Partition into "common" (curated top-of-page list) vs "everything else"
  // (the categorised grid below). When the showAll toggle is off and a
  // common key has no user override, it's still surfaced — common settings
  // are interesting even before they've been touched. An active search query
  // filters all three groups (common / category / raw) down to matches.
  const commonItems = items
    .filter((item) => COMMON_KEYS.has(`${item.section}|${item.key}`))
    .filter((item) => matchesSetting(item, q))
  const advancedItems = visibleItems
    .filter((item) => !COMMON_KEYS.has(`${item.section}|${item.key}`))
    .filter((item) => matchesSetting(item, q))
  const categories = groupByCategory(advancedItems)

  const toggleCategory = (cat: string) => {
    setExpandedCategory((prev) => {
      const next = prev === cat ? null : cat
      if (next === null) localStorage.removeItem('serverSettings.expandedCategory')
      else localStorage.setItem('serverSettings.expandedCategory', next)
      return next
    })
  }

  // Group raw sections by INI section name, merging all source files.
  // Iteration in priority order ensures the Map key insertion order
  // matches the first-seen source (lowest priority first).
  const rawBySection = new Map<string, RawSection[]>()
  for (const src of SOURCE_PRIORITY) {
    for (const sec of raw) {
      if (sec.source !== src) continue
      const arr = rawBySection.get(sec.section) ?? []
      arr.push(sec)
      rawBySection.set(sec.section, arr)
    }
  }

  // In "user settings" mode, hide raw panels whose entries are only from default files.
  // An active search query further narrows to sections matching the query.
  const visibleRawSections = (showAll
    ? [...rawBySection.values()]
    : [...rawBySection.values()].filter((secs) =>
        secs.some((s) => USER_SOURCES.has(s.source)),
      )
  ).filter((secs) => matchesRawSection(secs, q))

  const hasResults
    = commonItems.length > 0 || categories.length > 0 || visibleRawSections.length > 0

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title="Server Settings">
        <div className="flex items-center gap-2">
          <SearchField
            aria-label="Search settings"
            className="w-56"
            value={search}
            onChange={setSearch}
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder="Search settings…" />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading || saving}>
            <Icon name="refresh-cw" />
          </Button>
          <Button
            size="sm"
            variant={showAll ? 'primary' : 'ghost'}
            onPress={toggleShowAll}
            aria-label={showAll ? 'Showing all settings — click to show user settings only' : 'Showing user settings only — click to show all'}
          >
            <Icon name={showAll ? 'eye' : 'eye-off'} className="w-3.5 h-3.5" />
            <span className="ml-1">{showAll ? 'All' : 'User'}</span>
          </Button>
          <Button size="sm" onPress={save} isDisabled={dirtyCount === 0 || saving}>
            {saving
              ? <Spinner size="sm" color="current" />
              : `Save${dirtyCount > 0 ? ` (${dirtyCount})` : ''}`}
          </Button>
        </div>
      </PageHeader>

      <p className="text-xs text-muted shrink-0">
        Changes are written to
        {' '}
        <span className="font-mono">UserGame.ini</span>
        {' '}
        or
        {' '}
        <span className="font-mono">UserEngine.ini</span>
        .
        A server restart is required for them to take effect.
      </p>

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6 pr-1">

        {searching && !hasResults && (
          <div className="text-sm text-muted py-8 text-center">
            No settings match “
            {search.trim()}
            ”.
          </div>
        )}

        {/* Common Settings — curated subset, always visible at top */}
        {commonItems.length > 0 && (
          <Panel>
            <SectionLabel>Common Settings</SectionLabel>
            <div className="text-xs text-muted mb-2">
              The dozen or so knobs admins reach for most often. Everything else lives in the categories below.
            </div>
            <div>
              {commonItems.map((item) => (
                <SettingRow
                  key={`common|${item.section}|${item.key}`}
                  item={item}
                  pending={pending.get(pendingKey(item))}
                  onChange={(v) => handleChange(item, v)}
                  onDelete={() => handleDelete(item)}
                />
              ))}
            </div>
          </Panel>
        )}

        {/* Category grid — clicking a card expands that category's settings
            inline, spanning the full grid row right below the clicked card.
            Subsequent cards reflow down. col-span-full on the expanded Panel
            is what makes the grid break cleanly at that point. */}
        {categories.length > 0 && (
          <div>
            <SectionLabel>Advanced — All Categories</SectionLabel>
            <div className="text-xs text-muted mb-2">
              Click a category to expand it in place. Click again or pick another to switch.
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2 mt-2">
              {categories.map(([cat, catItems]) => {
                const isOpen = searching || expandedCategory === cat
                const overrideCount = catItems.filter((i) =>
                  i.layers.some((l) => USER_SOURCES.has(l.source)),
                ).length
                return (
                  <Fragment key={cat}>
                    <button
                      onClick={() => toggleCategory(cat)}
                      className={`flex items-center gap-2 rounded border px-3 py-2.5 text-left transition-colors ${
                        isOpen
                          ? 'bg-accent/15 border-accent/60 text-foreground'
                          : 'bg-surface border-border/60 hover:bg-surface-secondary hover:border-border text-foreground/90'
                      }`}
                    >
                      <Icon
                        name={CATEGORY_ICONS[cat] ?? 'sliders'}
                        className={`w-4 h-4 shrink-0 ${isOpen ? 'text-accent' : 'text-muted'}`}
                      />
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium truncate">{cat}</div>
                        <div className="text-xs text-muted">
                          {catItems.length}
                          {' '}
                          {catItems.length === 1 ? 'setting' : 'settings'}
                          {overrideCount > 0 && (
                            <span className="ml-1 text-warning">
                              ·
                              {overrideCount}
                              {' '}
                              overridden
                            </span>
                          )}
                        </div>
                      </div>
                      <Icon
                        name={isOpen ? 'chevron-up' : 'chevron-down'}
                        className={`w-4 h-4 shrink-0 ${isOpen ? 'text-accent' : 'text-muted/50'}`}
                      />
                    </button>
                    {isOpen && (
                      <Panel className="col-span-full mt-1 mb-1">
                        <div className="flex items-center justify-between mb-2">
                          <SectionLabel>{cat}</SectionLabel>
                          {!searching && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onPress={() => toggleCategory(cat)}
                              aria-label="Collapse category"
                            >
                              <Icon name="x" className="w-3.5 h-3.5" />
                            </Button>
                          )}
                        </div>
                        <div>
                          {catItems.map((item) => (
                            <SettingRow
                              key={`${item.section}|${item.key}`}
                              item={item}
                              pending={pending.get(pendingKey(item))}
                              onChange={(v) => handleChange(item, v)}
                              onDelete={() => handleDelete(item)}
                            />
                          ))}
                        </div>
                      </Panel>
                    )}
                  </Fragment>
                )
              })}
            </div>
          </div>
        )}

        {/* Raw sections — non-schema keys and array entries, one panel per INI section */}
        {visibleRawSections.length > 0 && (
          <div>
            <SectionLabel>Raw INI Sections</SectionLabel>
            <div className="text-xs text-muted mb-2">
              Array entries (
              <code className="font-mono">+key=val</code>
              ) and unrecognised keys, grouped by INI section.
            </div>
            <div className="flex flex-col gap-3 mt-2">
              {visibleRawSections.map((sections) => (
                <RawSectionPanel
                  key={sections[0].section}
                  sections={sections}
                  onSaved={load}
                />
              ))}
            </div>
          </div>
        )}

      </div>
    </div>
  )
}
