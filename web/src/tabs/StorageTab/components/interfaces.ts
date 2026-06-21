import type { Container } from '../types'

export interface AddItemsModalProps {
  container: Container
  open: boolean
  onClose: () => void
  onSuccess: () => void
  onRefresh: () => void
}
