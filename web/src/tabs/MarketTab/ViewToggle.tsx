import * as React from 'react'
import { ToggleButtonGroup, ToggleButton } from '@heroui/react'
import { Icon } from '../../dune-ui'
import type { ViewToggleProps } from './types'

export const ViewToggle: React.FC<ViewToggleProps> = ({ view, onChange }) => {
  return (
    <ToggleButtonGroup
      selectionMode="single"
      disallowEmptySelection
      selectedKeys={[view]}
      onSelectionChange={(keys) => {
        const next = [...keys][0]
        if (next === 'grid' || next === 'table') onChange(next)
      }}
      className="shrink-0"
    >
      <ToggleButton id="grid" isIconOnly aria-label="Grid view">
        <Icon name="layout-grid" />
      </ToggleButton>
      <ToggleButton id="table" isIconOnly aria-label="Table view">
        <Icon name="list" />
      </ToggleButton>
    </ToggleButtonGroup>
  )
}
