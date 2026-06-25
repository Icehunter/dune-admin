import * as React from 'react'
import { Checkbox, Chip } from '@heroui/react'
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
    >
      <Checkbox.Content className="w-full max-w-none gap-2 py-1.5 px-3 hover:bg-surface-secondary rounded-[var(--radius)] cursor-pointer">
        <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
        <SpriteIcon type={typeKey} size={18} />
        {!ICON_POS[typeKey] && (
          <span style={{ color: CAT_COLOR[category] }} className="shrink-0">●</span>
        )}
        <span className="flex-1 text-xs text-foreground truncate">{label}</span>
        <Chip size="sm" variant="soft">{count.toLocaleString()}</Chip>
      </Checkbox.Content>
    </Checkbox>
  )
}
