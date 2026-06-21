export interface DeleteCharacterModalProps {
  open: boolean
  playerName: string
  online: boolean
  busy: boolean
  onCancel: () => void
  onConfirm: (reason: string) => void
}
