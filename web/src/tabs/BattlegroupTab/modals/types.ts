import type { BackupFile } from '../../../api/client'
import type { ActionDef, ServerRow } from '../types'

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

export type PartitionRestartConfirmProps = {
  server: ServerRow | null
  onConfirm: (server: ServerRow) => void
  onClose: () => void
}

export type RestoreModalProps = {
  open: boolean
  backupFiles: BackupFile[]
  backupFilesLoading: boolean
  setBackupFiles: (files: BackupFile[]) => void
  onClose: () => void
  /** Called once the backend restore job is accepted — the caller opens the
   * shared step-progress dialog, which polls the job to completion. */
  onRestoreStarted: () => void
}
