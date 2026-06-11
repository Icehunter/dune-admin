import type { EventDefinition } from '../../../api/client'

export interface EventEditorModalProps {
  isOpen: boolean
  onClose: () => void
  editing: EventDefinition | null
  onSaved: () => void
}
