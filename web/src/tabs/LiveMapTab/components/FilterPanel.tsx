import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Checkbox, SearchField } from '@heroui/react'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import { LIVE_TYPES, CATEGORY_GROUPS, CAT_COLOR, HEATMAP_BOUNDS, HEATMAP_TYPES, HEATMAP_COLORS, TYPE_LABELS } from '../constants'
import { filterKey, heatmapFilterKey } from '../utils'
import { CategorySection } from './CategorySection'
import type { FilterPanelProps } from '../interfaces'

export const FilterPanel: React.FC<FilterPanelProps> = ({
  filter, onToggle, onClear, spawns, mapKey, heatmapMode, onHeatmapToggle,
}): React.ReactElement => {
  const { t } = useTranslation()
  const [search, setSearch] = React.useState('')
  const [expanded, setExpanded] = React.useState<Record<string, boolean>>({})

  const typesByCategory: Record<string, Map<string, { label: string, count: number }>> = {}
  spawns.forEach((s) => {
    const cat = s.category
    if (!typesByCategory[cat]) typesByCategory[cat] = new Map()
    const key = filterKey(s.type)
    const label = TYPE_LABELS[key] ?? s.label ?? s.type.replace(/_/g, ' ')
    const existing = typesByCategory[cat].get(key)
    typesByCategory[cat].set(key, { label, count: (existing?.count ?? 0) + 1 })
  })

  const LIVE_LABELS: Record<string, string> = {
    players: t('liveMap.players'),
    vehicles: t('liveMap.vehicles'),
    bases: t('liveMap.filterBases'),
  }

  const renderHeatmapLegend = (): React.ReactNode => {
    if (!heatmapMode) return null
    const active = (HEATMAP_TYPES[mapKey] ?? []).filter((type) => filter[heatmapFilterKey(type)] ?? false)
    if (!active.length) {
      return <p className="text-xs text-muted px-1 pb-1">{t('liveMap.densityNoneSelected')}</p>
    }
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
  }

  return (
    <div className="flex flex-col w-[294px] shrink-0 min-h-0 overflow-hidden rounded-[var(--radius)] border border-border bg-background">
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
            {renderHeatmapLegend()}
          </Panel>
        )}

        {CATEGORY_GROUPS.map((group) => (
          <CategorySection
            key={group.id}
            group={group}
            typesByCategory={typesByCategory}
            expanded={expanded}
            setExpanded={setExpanded}
            search={search}
            filter={filter}
            onToggle={onToggle}
          />
        ))}
      </div>
    </div>
  )
}
