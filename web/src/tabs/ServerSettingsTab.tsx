import { useState, useEffect, useCallback, useRef } from 'react'
import { Button, ListBox, Select, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { ServerSetting, ServerSettingUpdate, RawSection } from '../api/client'
import { PageHeader, Panel, SectionLabel, Icon } from '../dune-ui'

const CATEGORY_ORDER = [
  'Survival', 'Progression', 'Harvesting', 'Building', 'Inventory',
  'Guilds & Economy', 'Storm Cycle', 'PvP & Security', 'Spice', 'Taxation', 'Sandworm',
]

const SOURCE_FILE: Record<string, string> = {
  defaultGame:    'DefaultGame.ini',
  defaultEngine:  'DefaultEngine.ini',
  userGame:       'UserGame.ini',
  userEngine:     'UserEngine.ini',
  userOverrides:  'UserOverrides.ini',
}

const LAYER_STYLE: Record<string, { cls: string }> = {
  defaultGame:   { cls: 'text-muted/60' },
  defaultEngine: { cls: 'text-muted/60' },
  userEngine:    { cls: 'text-foreground/70' },
  userGame:      { cls: 'text-warning' },
  userOverrides: { cls: 'text-accent' },
}

const SOURCE_PRIORITY = ['defaultGame', 'defaultEngine', 'userGame', 'userEngine', 'userOverrides'] as const

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
  const file  = SOURCE_FILE[s]
  const style = LAYER_STYLE[s]
  if (!file || !style) return null
  return { text: file, cls: style.cls }
}

function shortSection(section: string) {
  // "/Script/DuneSandbox.BuildingSettings" → "BuildingSettings"
  const dot = section.lastIndexOf('.')
  return dot >= 0 ? section.slice(dot + 1) : section
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
  const dirty    = pending !== undefined && rawDisplay !== item.current
  const src      = sourceLabel(item.source)


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
              const style   = LAYER_STYLE[layer.source] ?? { cls: 'text-muted' }
              const isActive = i === item.layers.length - 1
              return (
                <span key={layer.source} className="flex items-center gap-1">
                  <span className={`text-xs font-mono px-1.5 py-0.5 rounded border border-border/30 bg-surface/60 ${style.cls} ${isActive ? 'font-semibold' : 'opacity-50'}`}>
                    {SOURCE_FILE[layer.source] ?? layer.source}: {trimFloat(layer.value)}{isActive ? ' ✓' : ''}
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
        {item.type === 'bool' ? (
          <Select selectedKey={display} onSelectionChange={k => onChange(String(k))} className="w-32">
            <Select.Trigger className="h-7 text-xs">
              <Select.Value /><Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                <ListBox.Item id="True"  textValue="True">True<ListBox.ItemIndicator /></ListBox.Item>
                <ListBox.Item id="False" textValue="False">False<ListBox.ItemIndicator /></ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        ) : item.type === 'string' ? (
          <input
            type="text"
            value={display}
            onChange={e => onChange(e.target.value)}
            className="w-40 bg-surface border border-border rounded px-2 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60"
          />
        ) : (
          <input
            type="number"
            step={item.type === 'float' ? '0.01' : '1'}
            value={display}
            onChange={e => onChange(e.target.value)}
            className="w-28 bg-surface border border-border rounded px-2 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 text-right"
          />
        )}
        {item.source === 'userOverrides' && (
          <button
            onClick={onDelete}
            title="Remove from UserOverrides.ini"
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
  return lines.map(l => `${l.prefix}${l.key}=${l.value}`).join('\n')
}

// Trim Go's 6-decimal float formatting: "500.000000" → "500", "0.300000" → "0.3"
function trimFloat(v: string): string {
  if (!v.includes('.')) return v
  const n = parseFloat(v)
  return isNaN(n) ? v : n.toString()
}

function groupLinesByKey(lines: RawSection['lines']) {
  const grouped: { key: string; lines: typeof lines }[] = []
  const seen = new Map<string, number>()
  for (const line of lines) {
    const idx = seen.get(line.key)
    if (idx !== undefined) {
      grouped[idx].lines.push(line)
    } else {
      seen.set(line.key, grouped.length)
      grouped.push({ key: line.key, lines: [line] })
    }
  }
  return grouped
}

// One panel per INI section name, merging all source files that contain it.
function RawSectionPanel({ sections, onSaved }: { sections: RawSection[]; onSaved: () => void }) {
  const sectionName    = sections[0].section
  const overridesSec   = sections.find(s => s.source === 'userOverrides')

  // Editing always targets UserOverrides.ini regardless of which sources are present.
  const [editing, setEditing] = useState(false)
  const [draft, setDraft]     = useState('')
  const [saving, setSaving]   = useState(false)
  const textareaRef           = useRef<HTMLTextAreaElement>(null)

  const startEdit = () => {
    setDraft(overridesSec ? linesToText(overridesSec.lines) : '')
    setEditing(true)
    setTimeout(() => textareaRef.current?.focus(), 0)
  }

  const cancel = () => setEditing(false)

  const save = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, 'userOverrides', draft)
      toast.success('Saved to UserOverrides.ini')
      setEditing(false)
      onSaved()
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  const deleteOverride = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, 'userOverrides', '')
      toast.success('Removed from UserOverrides.ini')
      onSaved()
    } catch (e: unknown) {
      toast.danger(`Delete failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
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
      <div className="flex items-center gap-2 mb-2 flex-wrap">
        <SectionLabel>{shortSection(sectionName)}</SectionLabel>
        {sorted.map(s => (
          <span key={s.source} className={`text-xs ${LAYER_STYLE[s.source]?.cls ?? 'text-muted'}`}>
            {SOURCE_FILE[s.source] ?? s.source}
          </span>
        ))}
        <div className="ml-auto flex items-center gap-1 min-w-[2rem]">
          {editing ? (
            <>
              <Button size="sm" variant="ghost" onPress={cancel} isDisabled={saving}>Cancel</Button>
              <Button size="sm" onPress={save} isDisabled={saving}>
                {saving ? <Spinner size="sm" color="current" /> : 'Save'}
              </Button>
            </>
          ) : (
            <>
              {overridesSec && (
                <button
                  onClick={deleteOverride}
                  title="Remove from UserOverrides.ini"
                  className="text-muted/50 hover:text-danger transition-colors"
                  disabled={saving}
                >
                  <Icon name="trash-2" className="w-3.5 h-3.5" />
                </button>
              )}
              <Button size="sm" variant="ghost" onPress={startEdit} isDisabled={saving}>
                <Icon name="pencil" className="w-3.5 h-3.5" />
              </Button>
            </>
          )}
        </div>
      </div>

      {editing ? (
        <textarea
          ref={textareaRef}
          value={draft}
          onChange={e => setDraft(e.target.value)}
          rows={Math.max(4, draft.split('\n').length + 1)}
          className="w-full bg-surface border border-border rounded px-3 py-2 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 resize-y"
          spellCheck={false}
          placeholder="Key=Value or +Key=Value for array entries"
        />
      ) : (
        <div className="flex flex-col gap-2">
          {sorted.map(sec => {
            const style   = LAYER_STYLE[sec.source] ?? { cls: 'text-muted' }
            const isActive = sec.source === sorted[sorted.length - 1].source
            return (
              <div
                key={sec.source}
                className={multiSource ? `pl-2 border-l-2 ${isActive ? 'border-accent/40' : 'border-border/30'}` : ''}
              >
                {multiSource && (
                  <span className={`text-xs ${style.cls} block mb-1`}>
                    {SOURCE_FILE[sec.source] ?? sec.source}{isActive ? ' ✓' : ''}
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
      )}
    </Panel>
  )
}

const USER_SOURCES = new Set(['userGame', 'userEngine', 'userOverrides'])

export default function ServerSettingsTab() {
  const [items, setItems]     = useState<ServerSetting[]>([])
  const [raw, setRaw]         = useState<RawSection[]>([])
  const [pending, setPending] = useState<Map<string, string>>(new Map())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving]   = useState(false)
  const [error, setError]     = useState<string | null>(null)
  const [showAll, setShowAll] = useState(() =>
    localStorage.getItem('serverSettings.showAll') === 'true'
  )

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await api.serverSettings.get()
      setItems(data.settings ?? [])
      setRaw(data.raw ?? [])
      setPending(new Map())
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const pendingKey = (item: ServerSetting) => `${item.section}|${item.key}`

  const handleChange = (item: ServerSetting, value: string) => {
    setPending(prev => { const n = new Map(prev); n.set(pendingKey(item), value); return n })
  }

  const handleDelete = async (item: ServerSetting) => {
    try {
      await api.serverSettings.update([{ section: item.section, key: item.key, value: '' }])
      toast.success('Removed from UserOverrides.ini')
      load()
    } catch (e: unknown) {
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
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
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

  const toggleShowAll = () => setShowAll(v => {
    localStorage.setItem('serverSettings.showAll', String(!v))
    return !v
  })

  // In "user settings" mode, show only items that have at least one value
  // from a user-controlled file (userGame / userEngine / userOverrides).
  const visibleItems = showAll
    ? items
    : items.filter(item => item.layers.some(l => USER_SOURCES.has(l.source)))

  const categories = groupByCategory(visibleItems)

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
  const visibleRawSections = showAll
    ? [...rawBySection.values()]
    : [...rawBySection.values()].filter(secs =>
        secs.some(s => USER_SOURCES.has(s.source))
      )

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title="Server Settings">
        <div className="flex items-center gap-2">
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
        Changes are written to <span className="font-mono">UserOverrides.ini</span>.
        A server restart is required for them to take effect.
      </p>

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6">

        {/* Typed / schema settings */}
        {categories.map(([cat, catItems]) => (
          <Panel key={cat}>
            <SectionLabel>{cat}</SectionLabel>
            <div>
              {catItems.map(item => (
                <SettingRow
                  key={`${item.section}|${item.key}`}
                  item={item}
                  pending={pending.get(pendingKey(item))}
                  onChange={v => handleChange(item, v)}
                  onDelete={() => handleDelete(item)}
                />
              ))}
            </div>
          </Panel>
        ))}

        {/* Raw sections — non-schema keys and array entries, one panel per INI section */}
        {visibleRawSections.map(sections => (
          <RawSectionPanel
            key={sections[0].section}
            sections={sections}
            onSaved={load}
          />
        ))}

      </div>
    </div>
  )
}
