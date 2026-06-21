export interface BasesTabProps {
  isSignedIn?: boolean
}

export interface BlueprintsTabProps {
  isSignedIn?: boolean
}

export interface ImportModalProps {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

export interface FieldProps {
  label: string
  value: string
}

export interface GuildsTabProps {
  isSignedIn?: boolean
}

export interface LogsTabProps {
  control?: string | undefined
}
