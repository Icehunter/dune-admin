import * as React from 'react'
import { Button, Checkbox, ListBox, Select } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../../dune-ui'
import { PLAYER_COLUMNS } from '../types'
import type { PlayerSortKey, PlayerStatusFilter } from '../types'
import type { PlayerListControlsProps } from './interfaces'

// SORT and FILTER are two orthogonal controls (#281): sort picks one axis +
// direction; filters (status, faction) narrow the population and combine
// freely with each other and with free-text search (handled by the caller).
export const PlayerListControls: React.FC<PlayerListControlsProps> = ({
  sortKey,
  onSortKeyChange,
  sortDir,
  onToggleSortDir,
  statusFilter,
  onStatusFilterChange,
  factionFilter,
  onFactionFilterChange,
  factionOptions,
}): React.ReactElement => {
  const { t } = useTranslation()

  const toggleFaction = (id: number, isSelected: boolean): void => {
    const next = new Set(factionFilter)
    if (isSelected) next.add(id)
    else next.delete(id)
    onFactionFilterChange(next)
  }

  // Faction facet is a Checkbox group (frontend.md: "Checkbox (filter/option)
  // — use Checkbox"), not a Select — RAC's Select is the wrong primitive for
  // a filter facet and every other selectionMode="multiple" usage in this
  // codebase is on DataTable/DataGrid row selection, not a dropdown. Renders
  // nothing until the player list has loaded at least one faction_id.
  const renderFactionFilter = (): React.ReactNode => {
    if (factionOptions.length === 0) return null
    return (
      <div className="flex flex-col gap-1">
        <span className="text-[10px] uppercase tracking-wide text-muted">
          {t('players.filter.faction')}
        </span>
        <div className="flex flex-wrap gap-x-3 gap-y-1">
          {factionOptions.map((f) => (
            <Checkbox
              key={f.id}
              isSelected={factionFilter.has(f.id)}
              onChange={(isSelected) => toggleFaction(f.id, isSelected)}
            >
              {f.label}
            </Checkbox>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1.5">
        <Select
          aria-label={t('players.sort.label')}
          selectedKey={sortKey}
          onSelectionChange={(key) => onSortKeyChange(key as PlayerSortKey)}
          className="flex-1 min-w-0"
        >
          <Select.Trigger className="h-8 text-xs">
            <Icon name="arrow-up-down" className="size-3.5 text-muted shrink-0 mr-1" />
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {PLAYER_COLUMNS.map((col) => (
                <ListBox.Item key={col.key} id={col.key} textValue={t(col.label as never)}>
                  {t(col.label as never)}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={sortDir === 'asc' ? t('players.sort.ascLabel') : t('players.sort.descLabel')}
          onPress={onToggleSortDir}
        >
          <Icon name={sortDir === 'asc' ? 'arrow-up' : 'arrow-down'} />
        </Button>
      </div>

      <Segment
        aria-label={t('players.filter.label')}
        variant="ghost"
        size="sm"
        className="w-full"
        selectedKey={statusFilter}
        onSelectionChange={(key) => onStatusFilterChange(key as PlayerStatusFilter)}
      >
        <Segment.Item id="all">
          <Segment.Separator />
          {t('players.filter.all')}
        </Segment.Item>
        <Segment.Item id="online">
          <Segment.Separator />
          {t('players.filter.online')}
        </Segment.Item>
        <Segment.Item id="offline">
          <Segment.Separator />
          {t('players.filter.offline')}
        </Segment.Item>
      </Segment>

      {renderFactionFilter()}
    </div>
  )
}
