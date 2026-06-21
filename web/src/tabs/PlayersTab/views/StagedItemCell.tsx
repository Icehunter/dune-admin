import * as React from 'react'
import { Chip } from '@heroui/react'
import { iconUrl, categoryColor } from '../../../utils/icons'
import type { StagedItemCellProps } from './interfaces'

// Sub-component exported so react-refresh treats it as a stable top-level component.
// Display-only (no picker semantics) — used in DataGrid template column.
export const StagedItemCell: React.FC<StagedItemCellProps> = ({ templateId, name, itemData }) => {
  const entry = itemData.items[templateId] ?? null
  const img = iconUrl(templateId, 'thumb')
  const rarity = entry?.rarity?.toLowerCase()

  return (
    <div className="flex items-center gap-2 py-0.5">
      {/* Thumbnail */}
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

      {/* Name + id */}
      <div className="flex-1 min-w-0">
        <div className="text-xs truncate text-foreground">{name || templateId}</div>
        {name && <div className="font-mono text-[10px] text-muted truncate">{templateId}</div>}
      </div>

      {/* Tier chip */}
      {!!entry?.tier && entry.tier > 0 && (
        <Chip size="sm" variant="soft" className="shrink-0">{`T${entry.tier}`}</Chip>
      )}

      {/* Rarity chip — inline color overrides Chip's internal text color */}
      {rarity && (
        <Chip size="sm" variant="soft" className="shrink-0 capitalize" style={{ color: `var(--rarity-${rarity})` }}>
          {rarity}
        </Chip>
      )}
    </div>
  )
}
