import * as React from 'react'
import { Button, ListBox, Select } from '@heroui/react'
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

      <Select
        aria-label={t('players.filter.faction')}
        selectionMode="multiple"
        value={[...factionFilter]}
        onChange={(keys) => onFactionFilterChange(new Set(keys as number[]))}
        placeholder={t('players.filter.factionAll')}
        className="w-full"
      >
        <Select.Trigger className="h-8 text-xs">
          <Select.Value />
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox selectionMode="multiple">
            {factionOptions.map((f) => (
              <ListBox.Item key={f.id} id={f.id} textValue={f.label}>
                {f.label}
                <ListBox.ItemIndicator />
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
    </div>
  )
}
