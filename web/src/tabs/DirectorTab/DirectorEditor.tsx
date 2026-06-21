import * as React from 'react'
import { FieldInput, FieldSelect } from '../../dune-ui'
import type { DirectorEditorProps } from './interfaces'

export const DirectorEditor: React.FC<DirectorEditorProps> = ({ kind, value, ariaLabel, onChange }) => {
  if (kind.kind === 'bool') {
    return (
      <FieldSelect
        className="w-full"
        value={value.trim().toLowerCase()}
        onChange={onChange}
        options={['true', 'false']}
        ariaLabel={ariaLabel}
      />
    )
  }
  if (kind.kind === 'enum') {
    // Keep the current value selectable even if it isn't in the derived option set.
    const opts = kind.options.includes(value) ? kind.options : [value, ...kind.options]
    return (
      <FieldSelect
        className="w-full"
        value={value}
        onChange={onChange}
        options={opts}
        ariaLabel={ariaLabel}
      />
    )
  }
  if (kind.kind === 'number') {
    return <FieldInput type="number" className="w-full" value={value} onChange={onChange} />
  }
  return <FieldInput className="w-full" value={value} onChange={onChange} />
}
