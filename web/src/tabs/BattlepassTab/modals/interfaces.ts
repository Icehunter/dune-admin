import * as React from 'react'
import type { BattlepassTier } from '../../../api/client'

export interface FormSectionProps {
  children: React.ReactNode
  className?: string
}

export interface TierEditorModalProps {
  isOpen: boolean
  onClose: () => void
  /** Existing tier to edit, or null to open in create mode. */
  tier: BattlepassTier | null
  onSaved: () => void
}
