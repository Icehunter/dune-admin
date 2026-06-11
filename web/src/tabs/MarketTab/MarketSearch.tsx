import * as React from 'react'
import { ListBox, SearchField, Select, Button } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../dune-ui'
import type { MarketFilters, MarketSearchProps } from './types'

export const MarketSearch: React.FC<MarketSearchProps> = ({ filters, onChange, onReset }) => {
  const { t } = useTranslation()
  const [searchDraft, setSearchDraft] = React.useState(filters.search)

  // Sync draft when filters are reset externally.
  React.useEffect(() => {
    const t = setTimeout(() => setSearchDraft(filters.search), 0)
    return () => clearTimeout(t)
  }, [filters.search])

  // Debounce: commit search text 350ms after the user stops typing.
  React.useEffect(() => {
    const t = setTimeout(() => {
      if (searchDraft !== filters.search) {
        onChange({ ...filters, search: searchDraft })
      }
    }, 350)
    return () => clearTimeout(t)
  }, [searchDraft]) // eslint-disable-line react-hooks/exhaustive-deps

  const set = (patch: Partial<MarketFilters>) => onChange({ ...filters, ...patch })
  const hasFilters = filters.search || filters.category || filters.owner

  return (
    <div className="flex flex-wrap items-center gap-2">
      <SearchField
        aria-label={t('market.search.ariaLabel')}
        className="flex-1 min-w-[200px]"
        value={searchDraft}
        onChange={setSearchDraft}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={t('market.search.searchPlaceholder')} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

      <Select
        selectedKey={filters.owner || 'all'}
        onSelectionChange={(k) => set({ owner: k === 'all' ? '' : k as MarketFilters['owner'] })}
        className="w-36"
        aria-label={t('market.search.sellerAriaLabel')}
      >
        <Select.Trigger>
          <Select.Value />
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            <ListBox.Item id="all" textValue={t('market.search.allSellers')}>
              {t('market.search.allSellers')}
              <ListBox.ItemIndicator />
            </ListBox.Item>
            <ListBox.Item id="bot" textValue={t('market.search.botOnly')}>
              {t('market.search.botOnly')}
              <ListBox.ItemIndicator />
            </ListBox.Item>
            <ListBox.Item id="player" textValue={t('market.search.playersOnly')}>
              {t('market.search.playersOnly')}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          </ListBox>
        </Select.Popover>
      </Select>

      {hasFilters && (
        <Button size="sm" variant="ghost" onPress={onReset}>
          <Icon name="x" />
          {' '}
          {t('market.search.clearFilters')}
        </Button>
      )}
    </div>
  )
}
