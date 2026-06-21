import * as React from 'react'
import { Chip } from '@heroui/react'
import { iconUrl, categoryColor } from '../../../utils/icons'
import type { ModalStagedItemCellProps } from './interfaces'

// Sub-component exported for react-refresh. Display-only thumbnail + name cell for staged items.
export const ModalStagedItemCell: React.FC<ModalStagedItemCellProps> = ({ templateId, name, itemData }) => {
  const entry = itemData.items[templateId] ?? null
  const img = iconUrl(templateId, 'thumb')
  const rarity = entry?.rarity?.toLowerCase()

  return (
    <div className="flex items-center gap-2 py-0.5">
      <div
        className="w-6 h-6 shrink-0 rounded flex items-center justify-center overflow-hidden"
        style={{ background: categoryColor(entry?.category ?? '', entry?.rarity?.toLowerCase(), templateId) }}
      >
        <img
          src={img ?? undefined}
          alt=""
          className="w-full h-full object-contain"
          onError={(e) => {
            (e.currentTarget as HTMLImageElement).style.display = 'none'
          }}
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-xs truncate text-foreground">{name || templateId}</div>
        {name && <div className="font-mono text-[10px] text-muted truncate">{templateId}</div>}
      </div>
      {!!entry?.tier && entry.tier > 0 && (
        <Chip size="sm" variant="soft" className="shrink-0">{`T${entry.tier}`}</Chip>
      )}
      {rarity && (
        <Chip size="sm" variant="soft" className="shrink-0 capitalize" style={{ color: `var(--rarity-${rarity})` }}>
          {rarity}
        </Chip>
      )}
    </div>
  )
}
