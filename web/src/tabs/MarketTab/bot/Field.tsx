import * as React from 'react'
import type { FieldProps } from './interfaces'

export const Field: React.FC<FieldProps> = ({ label, hint, children }) => {
  return (
    <div className="flex flex-col gap-0.5">
      <label className="text-xs text-muted">
        {label}
        {hint && (
          <span className="text-muted/60 ml-1">
            (
            {hint}
            )
          </span>
        )}
      </label>
      {children}
    </div>
  )
}
