import type { KeystoneRow } from '../../../../../api/client'

export interface AddTagsPanelProps {
  tags: string[]
  pendingTags: string[]
  onAdd: (tag: string) => void
}

export interface DebouncedSearchFieldProps {
  onSearch: (q: string) => void
  placeholder?: string
  className?: string
}

export interface KeystonesToggleProps {
  keystones: KeystoneRow[]
}
