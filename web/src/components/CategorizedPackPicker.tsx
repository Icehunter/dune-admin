import * as React from 'react'
import { Header, ListBox, Select, Separator } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { PackOption, CategorizedPackPickerProps } from './interfaces'

/** Group packs by category, sort each group by tier, and return category
 *  entries sorted by localeCompare. Pure helper — mirrors the original
 *  inline grouping in GiveItemsModal. */
// Pure grouping helper co-located with its picker; imported by tests.
// eslint-disable-next-line react-refresh/only-export-components
export function groupPacksByCategory(
  packs: PackOption[],
): [string, PackOption[]][] {
  const groups: Record<string, PackOption[]> = {}
  for (const pack of packs) {
    if (!groups[pack.category]) groups[pack.category] = []
    groups[pack.category].push(pack)
  }
  for (const cat of Object.keys(groups)) {
    groups[cat].sort((a, b) => a.tier - b.tier)
  }
  return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
}

/** Pack picker rendered as a categorized Select → Popover → ListBox, with one
 *  ListBox.Section per category (header = category name, dashes → spaces).
 *  Selection is fire-and-forget: the Select never holds a value, so the same
 *  pack can be picked repeatedly. */
export const CategorizedPackPicker: React.FC<CategorizedPackPickerProps> = ({
  packs, onSelectPack, className,
}) => {
  const { t } = useTranslation()
  const grouped = groupPacksByCategory(packs)

  return (
    <Select
      aria-label={t('players.give.loadPack')}
      placeholder={t('players.give.loadPack')}
      selectedKey={null}
      onSelectionChange={(k) => { if (k) onSelectPack(String(k)) }}
      {...(className !== undefined ? { className } : {})}
    >
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {grouped.map(([cat, catPacks], i, arr) => (
            <ListBox.Section key={cat}>
              <Header>{cat.replace(/-/g, ' ')}</Header>
              {catPacks.map((p) => (
                <ListBox.Item key={p.id} id={p.id} textValue={p.name}>
                  {p.name}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
              {i < arr.length - 1 && <Separator />}
            </ListBox.Section>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
