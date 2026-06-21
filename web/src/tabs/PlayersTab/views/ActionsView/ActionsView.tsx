import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@heroui/react'
import { useAtom, useSetAtom } from 'jotai'
import { ConfirmDialog, SideNav } from '../../../../dune-ui'
import { ManageLocationsModal } from '../../modals/ManageLocationsModal'
import { MapCoordPickerModal } from '../../modals/MapCoordPickerModal'
import { ACTION_SECTIONS, type ActionSection } from '../../types'
import { api } from '../../../../api/client'
import { usePermissions } from '../../../../hooks/usePermissions'
import {
  playerAtom, partitionsAtom, allPlayersAtom, charXPCurrentAtom, confirmAtom,
} from './store'
import { ResourcesSection } from './sections/ResourcesSection'
import { SpecsSection } from './sections/SpecsSection'
import { ProgressionSection } from './sections/ProgressionSection'
import { ContractsSection } from './sections/ContractsSection'
import { JourneySection } from './sections/JourneySection'
import { AdminSection } from './sections/AdminSection'
import { TagsSection } from './sections/TagsSection'
import { HistorySection } from './sections/HistorySection'
import { ExperimentalSection } from './sections/ExperimentalSection'
import type { ActionsViewProps } from './interfaces'

export const ActionsView: React.FC<ActionsViewProps> = ({ player }) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canPlayersWrite = can('players:write')
  // Without players:write only the read-only History section and the Admin
  // section (which itself gates its panels, leaving only the export) remain.
  const visibleSections = ACTION_SECTIONS.filter(
    (s) => canPlayersWrite || s.key === 'admin' || s.key === 'history',
  )
  const defaultSection: ActionSection = canPlayersWrite ? 'resources' : 'admin'
  const [section, setSection] = React.useState<ActionSection>(defaultSection)

  const setPlayerAtom = useSetAtom(playerAtom(player.id))
  const setPartitions = useSetAtom(partitionsAtom(player.id))
  const setAllPlayers = useSetAtom(allPlayersAtom(player.id))
  const setCharXPCurrent = useSetAtom(charXPCurrentAtom(player.id))
  const [confirmPending, setConfirmPending] = useAtom(confirmAtom(player.id))

  const [showManageLocations, setShowManageLocations] = React.useState(false)
  const [showTeleportPicker, setShowTeleportPicker] = React.useState(false)
  const [showSpawnPicker, setShowSpawnPicker] = React.useState(false)
  const teleportPickerCb = React.useRef<(x: number, y: number, z: number) => void>(undefined)
  const spawnPickerCb = React.useRef<(x: number, y: number, z: number) => void>(undefined)

  React.useEffect(() => {
    setPlayerAtom(player)
  }, [player, setPlayerAtom])

  React.useEffect(() => {
    Promise.resolve().then(() => setSection(defaultSection))
  }, [player.id, defaultSection])

  React.useEffect(() => {
    Promise.resolve()
      .then(() => Promise.all([
        api.locations.list(),
        api.players.charXPCurrent(player.id),
        api.players.list(),
      ]))
      .then(([parts, xp, ps]) => {
        setPartitions(parts)
        setCharXPCurrent(xp)
        setAllPlayers(ps.filter((p) => p.id !== player.id))
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
  }, [player.id, player.faction_id, setPartitions, setCharXPCurrent, setAllPlayers])

  const renderSection = (): React.ReactNode => {
    switch (section) {
      case 'resources': return <ResourcesSection player={player} />
      case 'specs': return <SpecsSection player={player} />
      case 'progression': return <ProgressionSection player={player} />
      case 'contracts': return <ContractsSection player={player} />
      case 'journey': return <JourneySection player={player} />
      case 'admin': return (
        <AdminSection
          player={player}
          onManageLocations={() => setShowManageLocations(true)}
          onTeleportPicker={(cb) => {
            teleportPickerCb.current = cb
            setShowTeleportPicker(true)
          }}
          onSpawnPicker={(cb) => {
            spawnPickerCb.current = cb
            setShowSpawnPicker(true)
          }}
        />
      )
      case 'tags': return <TagsSection player={player} />
      case 'history': return <HistorySection player={player} />
      case 'experimental': return <ExperimentalSection player={player} />
    }
  }

  const renderManageLocationsModal = (): React.ReactNode => {
    if (!showManageLocations) return null
    return (
      <ManageLocationsModal
        onClose={(updated) => {
          if (updated) setPartitions(updated)
          setShowManageLocations(false)
        }}
      />
    )
  }

  const renderTeleportPicker = (): React.ReactNode => {
    if (!showTeleportPicker) return null
    return (
      <MapCoordPickerModal
        onPick={(x, y, z) => {
          teleportPickerCb.current?.(x, y, z)
          setShowTeleportPicker(false)
        }}
        onClose={() => setShowTeleportPicker(false)}
      />
    )
  }

  const renderSpawnPicker = (): React.ReactNode => {
    if (!showSpawnPicker) return null
    return (
      <MapCoordPickerModal
        onPick={(x, y, z) => {
          spawnPickerCb.current?.(x, y, z)
          setShowSpawnPicker(false)
        }}
        onClose={() => setShowSpawnPicker(false)}
      />
    )
  }

  return (
    <React.Fragment>
      <div className="flex flex-row h-full min-h-0 gap-3">
        {/* Vertical section nav (HeroUI Pro ListView via SideNav). */}
        <SideNav
          items={visibleSections.map((s) => ({ key: s.key, label: t(s.label as never) }))}
          active={section}
          onSelect={setSection}
          width="w-48"
        />

        <div className="flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
          {renderSection()}
        </div>
      </div>

      <ConfirmDialog
        open={confirmPending !== null}
        title={confirmPending?.title ?? ''}
        description={confirmPending?.description ?? ''}
        confirmLabel={confirmPending?.confirmLabel}
        onConfirm={() => {
          const action = confirmPending?.onConfirm
          setConfirmPending(null)
          action?.()
        }}
        onCancel={() => setConfirmPending(null)}
      />

      {renderManageLocationsModal()}
      {renderTeleportPicker()}
      {renderSpawnPicker()}
    </React.Fragment>
  )
}
