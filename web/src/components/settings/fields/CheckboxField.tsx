import * as React from 'react'
import { Switch } from '@heroui/react'
import type { CheckboxFieldProps } from '../../interfaces'

export const CheckboxField: React.FC<CheckboxFieldProps> = ({ label, checked, onChange, hint }) => {
  return (
    <div className="flex flex-col gap-1">
      {hint && <p className="text-xs text-muted">{hint}</p>}
      <div className="flex flex-1 items-center">
        <Switch isSelected={!!checked} onChange={onChange} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{label}</Switch.Content>
        </Switch>
      </div>
    </div>
  )
}
