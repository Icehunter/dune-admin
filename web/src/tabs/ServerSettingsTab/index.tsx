import { useEffect, useMemo, useState } from 'react'
import { Button, Chip, Input, Spinner, toast } from '@heroui/react'
import { api } from '../../api/client'
import type { ServerSetting, ServerSettingUpdate } from '../../api/client'
import { PageHeader, Panel, SectionLabel, Icon } from '../../dune-ui'

// Display order of categories in the page. Anything not listed here falls to
// the end alphabetically.
const CATEGORY_ORDER = [
  'Survival',
  'Inventory',
  'Building',
  'Progression',
  'Harvesting',
  'Storm',
  'Sandworm',
  'PvP & Security',
  'Spice',
  'Taxation',
  'Guilds',
]

export default function ServerSettingsTab() {
  const [items, setItems] = useState<ServerSetting[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  // Map key = "section|key", value = pending raw string. Empty string = clear override.
  const [pending, setPending] = useState<Map<string, string>>(new Map())

  const load = async () => {
    setLoading(true)
    try {
      const data = await api.serverSettings.get()
      setItems(data)
      setPending(new Map())
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const grouped = useMemo(() => {
    const byCat = new Map<string, ServerSetting[]>()
    for (const it of items) {
      const arr = byCat.get(it.category) ?? []
      arr.push(it)
      byCat.set(it.category, arr)
    }
    // Sort categories per CATEGORY_ORDER, unknown at end
    const sortedCats = Array.from(byCat.keys()).sort((a, b) => {
      const ia = CATEGORY_ORDER.indexOf(a)
      const ib = CATEGORY_ORDER.indexOf(b)
      if (ia < 0 && ib < 0) return a.localeCompare(b)
      if (ia < 0) return 1
      if (ib < 0) return -1
      return ia - ib
    })
    return sortedCats.map(cat => ({ category: cat, items: byCat.get(cat)! }))
  }, [items])

  const pendingKey = (s: ServerSetting) => `${s.section}|${s.key}`

  const valueFor = (s: ServerSetting): string => {
    const k = pendingKey(s)
    return pending.has(k) ? pending.get(k)! : s.current
  }

  const setValue = (s: ServerSetting, raw: string) => {
    const k = pendingKey(s)
    setPending(prev => {
      const next = new Map(prev)
      if (raw === s.current) {
        next.delete(k)
      } else {
        next.set(k, raw)
      }
      return next
    })
  }

  const resetToDefault = (s: ServerSetting) => {
    // "Clear override" = send empty string to the API
    setValue(s, '')
  }

  const save = async () => {
    if (pending.size === 0) return
    setSaving(true)
    try {
      const updates: ServerSettingUpdate[] = []
      for (const [k, value] of pending.entries()) {
        const [section, key] = k.split('|')
        updates.push({ section, key, value })
      }
      const res = await api.serverSettings.update(updates)
      toast.success(res.ok)
      await load() // reload to reflect server-side state
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title="Server Settings">
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading || saving}>
          {loading ? <Spinner size="sm" color="current" /> : <><Icon name="refresh-cw" /> Refresh</>}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          onPress={save}
          isDisabled={pending.size === 0 || saving}
        >
          {saving ? <Spinner size="sm" color="current" /> : `Save ${pending.size > 0 ? `(${pending.size})` : ''}`}
        </Button>
      </PageHeader>

      <div className="rounded-md px-3 py-2 text-xs flex items-start gap-2 bg-surface-secondary border border-border/40 shrink-0">
        <Icon name="info" />
        <div>
          <div>Settings flagged <Chip size="sm" color="accent" variant="soft">Dune Admin</Chip> are set in <code className="font-mono">UserOverrides.ini</code> by this tool. Settings flagged <Chip size="sm" color="default" variant="soft">AMP UI</Chip> are managed by AMP via <code className="font-mono">UserGame.ini</code>. Saving here writes to UserOverrides.ini, which takes precedence over the AMP-managed value at server start.</div>
          <div className="text-muted mt-1">Restart the Dune instance via the AMP UI to apply.</div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto flex flex-col gap-4 pr-1">
        {loading && items.length === 0 ? (
          <div className="flex justify-center py-12"><Spinner size="lg" /></div>
        ) : grouped.length === 0 ? (
          <div className="text-center text-muted py-8">No settings available.</div>
        ) : (
          grouped.map(group => (
            <Panel key={group.category}>
              <SectionLabel>{group.category}</SectionLabel>
              <div className="flex flex-col">
                {group.items.map(item => (
                  <SettingRow
                    key={`${item.section}|${item.key}`}
                    item={item}
                    value={valueFor(item)}
                    isDirty={pending.has(pendingKey(item))}
                    onChange={v => setValue(item, v)}
                    onReset={() => resetToDefault(item)}
                  />
                ))}
              </div>
            </Panel>
          ))
        )}
      </div>
    </div>
  )
}

type RowProps = {
  item: ServerSetting
  value: string         // current effective display value (may be pending)
  isDirty: boolean      // whether this row has an unsaved pending change
  onChange: (raw: string) => void
  onReset: () => void
}

function SettingRow({ item, value, isDirty, onChange, onReset }: RowProps) {
  const placeholder = `default: ${String(item.default)}`

  return (
    <div className="flex items-start gap-3 py-3 border-b border-border/40 last:border-0">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">{item.label}</span>
          {isDirty && <Chip size="sm" color="warning" variant="soft">unsaved</Chip>}
          {!isDirty && item.source === 'userOverrides' && (
            <Chip size="sm" color="accent" variant="soft">Dune Admin</Chip>
          )}
          {!isDirty && item.source === 'userGame' && (
            <Chip size="sm" color="default" variant="soft">AMP UI</Chip>
          )}
        </div>
        <div className="text-xs text-muted mt-0.5">{item.description}</div>
        <div className="text-xs text-muted/60 font-mono mt-0.5">{item.section} · {item.key}</div>
      </div>

      <div className="flex items-center gap-2 shrink-0">
        {item.type === 'bool' ? (
          <select
            value={value || ''}
            onChange={e => onChange(e.target.value)}
            className="px-2 py-1 text-xs rounded border border-border bg-background"
          >
            <option value="">(default)</option>
            <option value="True">True</option>
            <option value="False">False</option>
          </select>
        ) : (
          <Input
            type="number"
            step={item.type === 'float' ? 'any' : '1'}
            value={value}
            placeholder={placeholder}
            onChange={e => onChange(e.target.value)}
            className="w-32"
            aria-label={item.label}
          />
        )}

        {(item.is_overridden || isDirty) && (
          <Button size="sm" variant="ghost" onPress={onReset} aria-label="Reset to default">
            <Icon name="x" />
          </Button>
        )}
      </div>
    </div>
  )
}
