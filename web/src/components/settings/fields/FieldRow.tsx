import * as React from 'react'
import type { FieldProps } from '../../types'

// FieldLabelContext exposes the current field's label to nested inputs (TextInput
// reads it to derive an aria-label).
// eslint-disable-next-line react-refresh/only-export-components
export const FieldLabelContext = React.createContext('')

export const FieldRow: React.FC<FieldProps> = ({ label, hint, children }) => {
  return (
    <FieldLabelContext.Provider value={label}>
      <div className="flex flex-col gap-1">
        <span className="text-xs text-muted font-medium">
          {label}
          {hint && (
            <span className="opacity-60 font-normal">
              {' '}
              (
              {hint}
              )
            </span>
          )}
        </span>
        {children}
      </div>
    </FieldLabelContext.Provider>
  )
}
