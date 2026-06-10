import type React from 'react'
import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useAtom, useSetAtom } from 'jotai'
import { ConfirmDialog, Panel } from '../../../../dune-ui'
import { ManageLocationsModal } from '../../modals/ManageLocationsModal'
import { MapCoordPickerModal } from '../../modals/MapCoordPickerModal'
import { ACTION_SECTIONS, type ActionSection } from '../../types'
import { api } from '../../../../api/client'
import type { Player } from '../../../../api/client'
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

interface ActionsViewProps {
  player: Player
}

export const ActionsView: React.FC<ActionsViewProps> = ({ player }) => {
  const { t } = useTranslation()
  const [section, setSection] = useState<ActionSection>('resources')

  const setPlayerAtom = useSetAtom(playerAtom(player.id))
  const setPartitions = useSetAtom(partitionsAtom(player.id))
  const setAllPlayers = useSetAtom(allPlayersAtom(player.id))
  const setCharXPCurrent = useSetAtom(charXPCurrentAtom(player.id))
  const [confirmPending, setConfirmPending] = useAtom(confirmAtom(player.id))

  const [showManageLocations, setShowManageLocations] = useState(false)
  const [showTeleportPicker, setShowTeleportPicker] = useState(false)
  const [showSpawnPicker, setShowSpawnPicker] = useState(false)
  const teleportPickerCb = useRef<(x: number, y: number, z: number) => void>(undefined)
  const spawnPickerCb = useRef<(x: number, y: number, z: number) => void>(undefined)

  useEffect(() => {
    setPlayerAtom(player)
  }, [player, setPlayerAtom])

  useEffect(() => {
    Promise.resolve().then(() => setSection('resources'))
  }, [player.id])

  useEffect(() => {
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

  return (
    <>
      <div className="flex flex-row h-full min-h-0 gap-3">
        <Panel className="shrink-0 p-2 overflow-hidden">
          {/* Vertical section nav. The Segment base style is a horizontal inline
              row; flex-col + stretch stacks the items full-width to match the
              previous vertical Tabs layout. */}
          <Segment
            orientation="vertical"
            aria-label="Actions sections"
            selectedKey={section}
            onSelectionChange={(k) => setSection(k as ActionSection)}
            className="flex flex-col items-stretch w-44"
          >
            {ACTION_SECTIONS.map((s) => (
              <Segment.Item key={s.key} id={s.key} className="justify-start">
                {t(s.label as never)}
              </Segment.Item>
            ))}
          </Segment>
        </Panel>

        <div className="flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
          {section === 'resources' && <ResourcesSection player={player} />}
          {section === 'specs' && <SpecsSection player={player} />}
          {section === 'progression' && <ProgressionSection player={player} />}
          {section === 'contracts' && <ContractsSection player={player} />}
          {section === 'journey' && <JourneySection player={player} />}
          {section === 'admin' && (
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
          )}
          {section === 'tags' && <TagsSection player={player} />}
          {section === 'history' && <HistorySection player={player} />}
          {section === 'experimental' && <ExperimentalSection player={player} />}
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

      {showManageLocations && (
        <ManageLocationsModal
          onClose={(updated) => {
            if (updated) setPartitions(updated)
            setShowManageLocations(false)
          }}
        />
      )}
      {showTeleportPicker && (
        <MapCoordPickerModal
          onPick={(x, y, z) => {
            teleportPickerCb.current?.(x, y, z)
            setShowTeleportPicker(false)
          }}
          onClose={() => setShowTeleportPicker(false)}
        />
      )}
      {showSpawnPicker && (
        <MapCoordPickerModal
          onPick={(x, y, z) => {
            spawnPickerCb.current?.(x, y, z)
            setShowSpawnPicker(false)
          }}
          onClose={() => setShowSpawnPicker(false)}
        />
      )}
    </>
  )
}
