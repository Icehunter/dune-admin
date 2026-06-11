import type { BackupFile } from '../../../api/client'
import type { ActionDef } from '../types'

export type CommandOutputModalProps = {
  runningCmd: string | null
  cmdOutput: string | null
  cmdDone: boolean
  lastBackupFile: string | null
  onClose: () => void
}

export type ConfirmDialogProps = {
  action: ActionDef | null
  onConfirm: (a: ActionDef) => void
  onClose: () => void
}

export type RestoreModalProps = {
  open: boolean
  backupFiles: BackupFile[]
  backupFilesLoading: boolean
  setBackupFiles: (files: BackupFile[]) => void
  onClose: () => void
  onRestoreComplete: (output: string) => void
}
