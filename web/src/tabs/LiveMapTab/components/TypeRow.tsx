import * as React from 'react'
import { Checkbox } from '@heroui/react'
import { CAT_COLOR, ICON_POS } from '../constants'
import { SpriteIcon } from './SpriteIcon'
import type { TypeRowProps } from './types'

export const TypeRow: React.FC<TypeRowProps> = ({
  typeKey, label, count, category, filter, onToggle,
}): React.ReactElement => {
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
