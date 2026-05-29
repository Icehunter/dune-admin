import { useEffect, useState } from 'react'
import { ListBox, SearchField, Select, Button } from '@heroui/react'
import { Icon } from '../../dune-ui'

export type MarketFilters = {
  search: string
  category: string
  owner: '' | 'bot' | 'player'
}

type Props = {
  filters: MarketFilters
  onChange: (f: MarketFilters) => void
  onReset: () => void
}

export default function MarketSearch({ filters, onChange, onReset }: Props) {
  const [searchDraft, setSearchDraft] = useState(filters.search)

  // Sync draft when filters are reset externally.
  useEffect(() => {
    const t = setTimeout(() => setSearchDraft(filters.search), 0)
    return () => clearTimeout(t)
  }, [filters.search])

  // Debounce: commit search text 350ms after the user stops typing.
  useEffect(() => {
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
        aria-label="Search items"
        className="flex-1 min-w-[200px]"
        value={searchDraft}
        onChange={setSearchDraft}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder="Search items…" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

      <Select
        selectedKey={filters.owner || 'all'}
        onSelectionChange={k => set({ owner: k === 'all' ? '' : k as MarketFilters['owner'] })}
        className="w-36"
        aria-label="Filter by seller"
      >
        <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
        <Select.Popover>
          <ListBox>
            <ListBox.Item id="all" textValue="All sellers">All sellers<ListBox.ItemIndicator /></ListBox.Item>
            <ListBox.Item id="bot" textValue="Bot only">Bot only<ListBox.ItemIndicator /></ListBox.Item>
            <ListBox.Item id="player" textValue="Players only">Players only<ListBox.ItemIndicator /></ListBox.Item>
          </ListBox>
        </Select.Popover>
      </Select>

      {hasFilters && (
        <Button size="sm" variant="ghost" onPress={onReset}>
          <Icon name="x" /> Clear
        </Button>
      )}
    </div>
  )
}
