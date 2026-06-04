import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, SearchField } from '@heroui/react'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import { LIVE_TYPES, CATEGORY_GROUPS, CAT_COLOR, TYPE_LABELS, ICON_POS, HEATMAP_BOUNDS, HEATMAP_TYPES, HEATMAP_COLORS } from '../constants'
import { filterKey, heatmapFilterKey } from '../utils'
import { SpriteIcon } from './SpriteIcon'
import type { FilterPanelProps } from '../types'

export function FilterPanel({
  filter, onToggle, onClear, spawns, mapKey, heatmapMode, onHeatmapToggle,
}: FilterPanelProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  const typesByCategory = useMemo(() => {
    const map: Record<string, Map<string, { label: string, count: number }>> = {}
    spawns.forEach((s) => {
      const cat = s.category
      if (!map[cat]) map[cat] = new Map()
      const key = filterKey(s.type)
      const label = TYPE_LABELS[key] ?? s.label ?? s.type.replace(/_/g, ' ')
      const existing = map[cat].get(key)
      map[cat].set(key, { label, count: (existing?.count ?? 0) + 1 })
    })
    return map
  }, [spawns])

  const LIVE_LABELS: Record<string, string> = {
    players: t('liveMap.players'),
    vehicles: t('liveMap.vehicles'),
    bases: t('liveMap.filterBases'),
  }

  type TypeRowProps = { typeKey: string, label: string, count: number, category: string }
  function TypeRow({ typeKey, label, count, category }: TypeRowProps) {
    const isOn = filter[typeKey] ?? false
    return (
      <label className="flex items-center gap-2 py-1.5 px-3 cursor-pointer hover:bg-surface-secondary rounded-[var(--radius)] select-none">
        <input
          type="checkbox"
          checked={isOn}
          onChange={() => onToggle(typeKey, isOn)}
          className="h-3.5 w-3.5 accent-accent shrink-0"
        />
        <SpriteIcon type={typeKey} size={18} />
        {!ICON_POS[typeKey] && (
          <span style={{ color: CAT_COLOR[category] }} className="shrink-0">●</span>
        )}
        <span className="flex-1 text-xs text-foreground truncate">{label}</span>
        <span className="text-xs text-muted tabular-nums shrink-0">{count.toLocaleString()}</span>
      </label>
    )
  }

  type CategorySectionProps = { group: (typeof CATEGORY_GROUPS)[number] }
  function CategorySection({ group }: CategorySectionProps) {
    const items = typesByCategory[group.id]
    if (!items?.size) return null
    const isExpanded = expanded[group.id] ?? false
    const allOn = [...items.keys()].every((k) => filter[k] ?? false)
    const anyOn = [...items.keys()].some((k) => filter[k] ?? false)
    const q = search.toLowerCase()
    const filteredItems = q
      ? [...items.entries()].filter(([k, v]) => v.label.toLowerCase().includes(q) || k.toLowerCase().includes(q))
      : [...items.entries()]

    if (q && filteredItems.length === 0) return null

    return (
      <div className="mb-1">
        <div className="flex items-center gap-1 px-2 py-1.5">
          <input
            type="checkbox"
            checked={allOn}
            ref={(el) => { if (el) el.indeterminate = !allOn && anyOn }}
            onChange={(e) => {
              const target = e.target.checked
              ;[...items.keys()].forEach((k) => onToggle(k, !target))
            }}
            className="h-3.5 w-3.5 accent-accent shrink-0"
          />
          <button
            type="button"
            className="flex-1 flex items-center gap-1.5 text-left"
            onClick={() => setExpanded((e) => ({ ...e, [group.id]: !e[group.id] }))}
          >
            <span style={{ color: CAT_COLOR[group.id] }} className="text-xs shrink-0">●</span>
            <span className="text-xs font-medium text-muted uppercase tracking-wide">{t(group.labelKey as never)}</span>
            <span className="text-xs text-muted/60 ml-1">
              {[...items.values()].reduce((s, v) => s + v.count, 0).toLocaleString()}
            </span>
            <Icon
              name={isExpanded || q ? 'chevron-down' : 'chevron-right'}
              className="size-3 text-muted ml-auto"
            />
          </button>
        </div>
        {(isExpanded || !!q) && (
          <div className="ml-1">
            {filteredItems.map(([key, { label, count }]) => (
              <TypeRow key={key} typeKey={key} label={label} count={count} category={group.id} />
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col w-60 shrink-0 min-h-0 overflow-hidden border-r border-border bg-background">
      <div className="px-2 pt-2 pb-1 shrink-0">
        <SearchField
          aria-label={t('liveMap.filter')}
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder={t('liveMap.filterSearch')} />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
      </div>
      <div className="px-2 pb-1 shrink-0 flex justify-end">
        <Button
          variant="ghost"
          size="sm"
          className="text-xs text-muted hover:text-accent px-1 h-auto min-w-0"
          onPress={onClear}
        >
          {t('liveMap.clearFilters')}
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {!search && (
          <Panel className="mb-2 mt-1">
            <SectionLabel>{t('liveMap.filterLive')}</SectionLabel>
            {LIVE_TYPES.map((id) => (
              <label key={id} className="flex items-center gap-2 py-1.5 cursor-pointer hover:bg-surface-secondary rounded-[var(--radius)] select-none px-1">
                <input
                  type="checkbox"
                  checked={filter[id] ?? false}
                  onChange={() => onToggle(id, filter[id] ?? false)}
                  className="h-3.5 w-3.5 accent-accent shrink-0"
                />
                <span style={{ color: CAT_COLOR[id] }} className="text-xs shrink-0">●</span>
                <span className="flex-1 text-xs text-foreground">{LIVE_LABELS[id]}</span>
              </label>
            ))}
          </Panel>
        )}

        {!search && HEATMAP_BOUNDS[mapKey] && (
          <Panel className="mb-2">
            <SectionLabel>{t('liveMap.filterDensity')}</SectionLabel>
            <label className="flex items-center gap-2 py-1.5 cursor-pointer hover:bg-surface-secondary rounded-[var(--radius)] select-none px-1">
              <input
                type="checkbox"
                checked={heatmapMode}
                onChange={onHeatmapToggle}
                className="h-3.5 w-3.5 accent-accent shrink-0"
              />
              <Icon name="layers" className="text-accent shrink-0" />
              <span className="flex-1 text-xs text-foreground">{t('liveMap.densityOverlay')}</span>
            </label>
            {heatmapMode && (() => {
              const active = (HEATMAP_TYPES[mapKey] ?? []).filter((type) => filter[heatmapFilterKey(type)] ?? false)
              if (!active.length) return (
                <p className="text-xs text-muted px-1 pb-1">{t('liveMap.densityNoneSelected')}</p>
              )
              return (
                <div className="px-1 pb-1 flex flex-col gap-0.5">
                  {active.map((type) => (
                    <div key={type} className="flex items-center gap-1.5">
                      <span className="w-3 h-3 rounded-sm shrink-0 opacity-80" style={{ background: HEATMAP_COLORS[type] ?? '#888' }} />
                      <span className="text-xs text-muted truncate">{TYPE_LABELS[type] ?? type.replace(/_/g, ' ')}</span>
                    </div>
                  ))}
                </div>
              )
            })()}
          </Panel>
        )}

        {CATEGORY_GROUPS.map((group) => (
          <CategorySection key={group.id} group={group} />
        ))}
      </div>
    </div>
  )
}
