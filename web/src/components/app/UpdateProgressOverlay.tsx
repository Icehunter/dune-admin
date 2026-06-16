import * as React from 'react'
import { useAtom, useAtomValue } from 'jotai'
import { UpdateProgressModal } from '../UpdateProgressModal'
import {
  updateApplyingAtom,
  updateErrorAtom,
  updatePhaseAtom,
} from '../../atoms/app'

// Update progress overlay — shown while downloading, restarting, and waiting for
// the server. Reads the update-flow atoms driven by useAppUpdate.
export const UpdateProgressOverlay: React.FC = () => {
  const [updateApplying, setUpdateApplying] = useAtom(updateApplyingAtom)
  const updatePhase = useAtomValue(updatePhaseAtom)
  const updateError = useAtomValue(updateErrorAtom)

  return (
    <UpdateProgressModal
      isOpen={updateApplying}
      phase={updatePhase}
      errorMessage={updateError}
      onDismiss={() => setUpdateApplying(false)}
    />
  )
}
