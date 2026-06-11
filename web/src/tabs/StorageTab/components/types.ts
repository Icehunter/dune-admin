import type { Container } from '../types'

export type { Container }

export interface AddItemsModalProps {
  container: Container
  open: boolean
  onClose: () => void
  onSuccess: () => void
  onRefresh: () => void
}

export type AddResult = { given: string[], skipped: { template: string, reason: string }[] } | null
