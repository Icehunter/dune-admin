import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Checkbox, SearchField } from '@heroui/react'
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
      <Checkbox
        isSelected={isOn}
        onChange={() => onToggle(typeKey, isOn)}
        className="flex items-center gap-2 py-1.5 px-3 hover:bg-surface-secondary rounded-[var(--radius)] w-full max-w-none"
      >
        <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
        <SpriteIcon type={typeKey} size={18} />
        {!ICON_POS[typeKey] && (
          <span style={{ color: CAT_COLOR[category] }} className="shrink-0">●</span>
        )}
        <span className="flex-1 text-xs text-foreground truncate">{label}</span>
        <span className="text-xs text-muted tabular-nums shrink-0">{count.toLocaleString()}</span>
      </Checkbox>
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

    const open = isExpanded || !!q
    const totalCount = [...items.values()].reduce((s, v) => s + v.count, 0)

    return (
      <div className="rounded-[var(--radius)] border border-border bg-surface">
        <div className="flex items-center gap-2 px-3 py-2">
          <Checkbox
            isSelected={allOn}
            isIndeterminate={!allOn && anyOn}
            onChange={(v) => { [...items.keys()].forEach((k) => onToggle(k, !v)) }}
          >
            <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
          </Checkbox>
          <button
            type="button"
            className="flex-1 flex items-center gap-1.5 text-left min-w-0 focus:outline-none"
            onClick={() => { if (!q) setExpanded((e) => ({ ...e, [group.id]: !isExpanded })) }}
          >
            <span style={{ color: CAT_COLOR[group.id] }} className="text-xs shrink-0">●</span>
            <span className="text-xs font-medium text-muted uppercase tracking-wide">{t(group.labelKey as never)}</span>
            <span className="text-xs text-muted/60 ml-1">{totalCount.toLocaleString()}</span>
            {!q && (
              <Icon
                name={open ? 'chevron-up' : 'chevron-down'}
                className="ml-auto size-3 text-muted shrink-0"
              />
            )}
          </button>
        </div>
        {open && (
          <div className="border-t border-border px-1 py-1.5">
            {filteredItems.map(([key, { label, count }]) => (
              <TypeRow key={key} typeKey={key} label={label} count={count} category={group.id} />
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col w-[270px] shrink-0 min-h-0 overflow-hidden rounded-[var(--radius)] border border-border bg-background">
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
          className="text-xs text-muted hover:text-accent px-1 h-auto min-w-0"
          onPress={onClear}
        >
          {t('liveMap.clearFilters')}
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2 flex flex-col gap-2">
        {!search && (
          <Panel>
            <SectionLabel>{t('liveMap.filterLive')}</SectionLabel>
            {LIVE_TYPES.map((id) => (
              <Checkbox
                key={id}
                isSelected={filter[id] ?? false}
                onChange={() => onToggle(id, filter[id] ?? false)}
                className="flex items-center gap-2 py-1.5 px-1 hover:bg-surface-secondary rounded-[var(--radius)] w-full max-w-none"
              >
                <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
                <span style={{ color: CAT_COLOR[id] }} className="text-xs shrink-0">●</span>
                <span className="flex-1 text-xs text-foreground">{LIVE_LABELS[id]}</span>
              </Checkbox>
            ))}
          </Panel>
        )}

        {!search && HEATMAP_BOUNDS[mapKey] && (
          <Panel>
            <SectionLabel>{t('liveMap.filterDensity')}</SectionLabel>
            <Checkbox
              isSelected={heatmapMode}
              onChange={onHeatmapToggle}
              className="flex items-center gap-2 py-1.5 px-1 hover:bg-surface-secondary rounded-[var(--radius)] w-full max-w-none"
            >
              <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
              <Icon name="layers" className="text-accent shrink-0" />
              <span className="flex-1 text-xs text-foreground">{t('liveMap.densityOverlay')}</span>
            </Checkbox>
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
