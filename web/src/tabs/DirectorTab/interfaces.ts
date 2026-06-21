import type { FieldKind } from '../types'

export interface DirectorEditorProps {
  kind: FieldKind
  value: string
  ariaLabel: string
  onChange: (v: string) => void
}
