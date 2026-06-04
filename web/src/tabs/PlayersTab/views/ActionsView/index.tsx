import type React from 'react'
import { useState, useEffect, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, toast } from '@heroui/react'
import { ConfirmDialog } from '../../../../dune-ui'
import { ManageLocationsModal } from '../../modals/ManageLocationsModal'
import { MapCoordPickerModal } from '../../modals/MapCoordPickerModal'
import { ACTION_SECTIONS, type ActionSection } from '../../types'
import type {
  Player,
  JourneyNode,
  SpecTrack,
  KeystoneRow,
  TeleportLocation,
  GameEvent,
  DungeonRecord,
  ProgressionPreset,
} from '../../../../api/client'
import { api } from '../../../../api/client'
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
  const [busy, setBusy] = useState(false)
  const [mounted, setMounted] = useState<Set<ActionSection>>(() => new Set<ActionSection>(['resources']))

  const [currency, setCurrency] = useState(100)
  const [scrip, setScrip] = useState(100)
  const [intel, setIntel] = useState(100)

  const [charXP, setCharXP] = useState(1000)
  const [charXPCurrent, setCharXPCurrent] = useState<{ xp: number, level: number } | null>(null)

  const [factionId, setFactionId] = useState(player.faction_id || 0)
  const [repDelta, setRepDelta] = useState(100)

  const [playerSpecs, setPlayerSpecs] = useState<SpecTrack[]>([])
  const [playerKeystones, setPlayerKeystones] = useState<KeystoneRow[]>([])
  const [specsLoaded, setSpecsLoaded] = useState(false)
  const [specsLoading, setSpecsLoading] = useState(false)

  const [nodes, setNodes] = useState<JourneyNode[]>([])
  const [nodesLoaded, setNodesLoaded] = useState(false)
  const [nodesLoading, setNodesLoading] = useState(false)
  const [nodeSearch, setNodeSearch] = useState('')
  const [unlockFaction, setUnlockFaction] = useState('atreides')
  const [unlockPreset, setUnlockPreset] = useState('ch3_start')

  const [customScriptName, setCustomScriptName] = useState('')

  const [selectedTrainer, setSelectedTrainer] = useState<'BeneGesserit' | 'Mentat' | 'Planetologist' | 'Swordmaster' | 'Trooper'>('BeneGesserit')
  const [selectedMQ, setSelectedMQ] = useState<string>('DA_MQ_ANewBeginning')

  const [presets, setPresets] = useState<ProgressionPreset[]>([])
  const [presetsLoaded, setPresetsLoaded] = useState(false)

  const [contractCatalog, setContractCatalog] = useState<{ id: string, alias: string, tag_count: number }[]>([])
  const [contractCatalogLoaded, setContractCatalogLoaded] = useState(false)
  const [contractCatalogError, setContractCatalogError] = useState('')
  const [contractSearch, setContractSearch] = useState('')
  const [selectedContracts, setSelectedContracts] = useState<string[]>([])

  const [tags, setTags] = useState<string[]>([])
  const [tagsLoaded, setTagsLoaded] = useState(false)
  const [tagsLoading, setTagsLoading] = useState(false)
  const [pendingTags, setPendingTags] = useState<string[]>([])
  const [tagRemoveSearch, setTagRemoveSearch] = useState('')

  const handleAddTag = useCallback((tag: string) => {
    setPendingTags((prev) => [...prev, tag])
  }, [])

  const filteredActiveTags = useMemo(() => {
    const q = tagRemoveSearch.toLowerCase()
    return q ? tags.filter((t) => t.toLowerCase().includes(q)) : tags
  }, [tags, tagRemoveSearch])

  const [skillPointsAmount, setSkillPointsAmount] = useState(10)
  const [skillModule, setSkillModule] = useState('')
  const [skillModuleLevel, setSkillModuleLevel] = useState(1)
  const [confirmPending, setConfirmPending] = useState<{
    title: string
    description: string
    confirmLabel: string
    onConfirm: () => void
  } | null>(null)

  const [partitions, setPartitions] = useState<TeleportLocation[]>([])
  const [selectedPartition, setSelectedPartition] = useState('')
  const [teleportX, setTeleportX] = useState('')
  const [teleportY, setTeleportY] = useState('')
  const [teleportZ, setTeleportZ] = useState('')
  const [showManageLocations, setShowManageLocations] = useState(false)
  const [showTeleportMapPicker, setShowTeleportMapPicker] = useState(false)
  const [allPlayers, setAllPlayers] = useState<Player[]>([])
  const [selectedTeleportTarget, setSelectedTeleportTarget] = useState<number | null>(null)
  const [targetSearch, setTargetSearch] = useState('')
  const [targetDropdownOpen, setTargetDropdownOpen] = useState(false)

  const [whisperText, setWhisperText] = useState('')
  const [whisperSenderName, setWhisperSenderName] = useState('GM')

  const [spawnVehicleId, setSpawnVehicleId] = useState('')
  const [spawnVehicleTemplate, setSpawnVehicleTemplate] = useState('')
  const [spawnVehiclePartition, setSpawnVehiclePartition] = useState('')
  const [spawnVehiclePersistent, setSpawnVehiclePersistent] = useState(true)
  const [spawnX, setSpawnX] = useState('')
  const [spawnY, setSpawnY] = useState('')
  const [spawnZ, setSpawnZ] = useState('')
  const [showSpawnMapPicker, setShowSpawnMapPicker] = useState(false)

  const [events, setEvents] = useState<GameEvent[]>([])
  const [dungeons, setDungeons] = useState<DungeonRecord[]>([])
  const [historyLoaded, setHistoryLoaded] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)

  useEffect(() => {
    Promise.resolve().then(() => {
      setMounted((prev) => {
        if (prev.has(section)) return prev
        const next = new Set(prev)
        next.add(section)
        return next
      })
    })
  }, [section])

  useEffect(() => {
    Promise.resolve().then(() => {
      setSection('resources')
      setNodesLoaded(false)
      setNodes([])
      setPlayerSpecs([])
      setPlayerKeystones([])
      setSpecsLoaded(false)
      setHistoryLoaded(false)
      setEvents([])
      setDungeons([])
      setCharXPCurrent(null)
      setTagsLoaded(false)
      setTags([])
      setPendingTags([])
      setConfirmPending(null)
      setContractCatalogLoaded(false)
      setContractCatalog([])
      setContractCatalogError('')
      setPresetsLoaded(false)
      setPresets([])
      setSelectedContracts([])
    })
  }, [player.id])

  useEffect(() => {
    Promise.resolve()
      .then(() => setFactionId(player.faction_id > 0 ? player.faction_id : 1))
      .then(() => Promise.all([api.locations.list(), api.players.charXPCurrent(player.id), api.players.list()]))
      .then(([parts, xp, ps]) => {
        setPartitions(parts)
        setCharXPCurrent(xp)
        setAllPlayers(ps.filter((p) => p.id !== player.id))
      })
      .catch(() => {})
  }, [player.id, player.faction_id])

  useEffect(() => {
    if (section === 'journey' && !nodesLoaded) {
      Promise.resolve()
        .then(() => setNodesLoading(true))
        .then(() => api.players.journey(player.account_id))
        .then((n) => {
          setNodes(n)
          setNodesLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setNodesLoading(false))
    }
    if ((section === 'progression' || section === 'contracts') && !contractCatalogLoaded) {
      api.contracts
        .list()
        .then((c) => {
          setContractCatalog(c)
          setContractCatalogLoaded(true)
          setContractCatalogError('')
        })
        .catch((e: unknown) => {
          setContractCatalogError(e instanceof Error ? e.message : String(e))
          setContractCatalogLoaded(true)
        })
    }
    if (section === 'progression' && !presetsLoaded) {
      api.progression
        .presets()
        .then((p) => {
          setPresets(p)
          setPresetsLoaded(true)
        })
        .catch(() => setPresetsLoaded(true))
    }
  }, [section, nodesLoaded, contractCatalogLoaded, presetsLoaded, player.account_id])

  useEffect(() => {
    if (section === 'specs' && !specsLoaded) {
      Promise.resolve()
        .then(() => setSpecsLoading(true))
        .then(() =>
          Promise.all([api.players.specs_for(player.controller_id), api.players.keystones(player.controller_id)]),
        )
        .then(([s, k]) => {
          setPlayerSpecs(s)
          setPlayerKeystones(k)
          setSpecsLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setSpecsLoading(false))
    }
  }, [section, specsLoaded, player.controller_id])

  useEffect(() => {
    if (section === 'history' && !historyLoaded) {
      Promise.resolve()
        .then(() => setHistoryLoading(true))
        .then(() => Promise.all([api.players.events(player.id), api.players.dungeons(player.id)]))
        .then(([evts, dngns]) => {
          setEvents(evts)
          setDungeons(dngns)
          setHistoryLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setHistoryLoading(false))
    }
  }, [section, historyLoaded, player.id])

  useEffect(() => {
    if (section !== 'tags' || tagsLoaded) return
    Promise.resolve()
      .then(() => setTagsLoading(true))
      .then(() => api.players.tags(player.account_id))
      .then((t) => {
        setTags(t)
        setTagsLoaded(true)
      })
      .catch(() => {})
      .finally(() => setTagsLoading(false))
  }, [section, tagsLoaded, player.account_id])

  const run = async (fn: () => Promise<unknown>, label: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(label)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setBusy(false)
    }
  }

  const gate = (title: string, description: string, confirmLabel: string, action: () => void) => {
    setConfirmPending({ title, description, confirmLabel, onConfirm: action })
  }

  const filteredNodes = useMemo(() => {
    if (!nodeSearch) return nodes
    const q = nodeSearch.toLowerCase()
    return nodes.filter((n) => n.node_id.toLowerCase().includes(q))
  }, [nodes, nodeSearch])

  return (
    <>
      <div className="flex flex-row h-full min-h-0 gap-3">
        <Tabs
          orientation="vertical"
          selectedKey={section}
          onSelectionChange={(k) => setSection(k as ActionSection)}
        >
          <Tabs.ListContainer className="shrink-0">
            <Tabs.List aria-label={t('players.actions.sectionsLabel' as never)}>
              {ACTION_SECTIONS.map((s) => (
                <Tabs.Tab key={s.key} id={s.key}>
                  {t(s.label as never)}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>
        </Tabs>

        <div className="flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
          {ACTION_SECTIONS.map((s) =>
            mounted.has(s.key)
              ? (
                  <div key={s.key} className={s.key === section ? 'flex flex-col flex-1 min-h-0 overflow-hidden' : 'hidden'}>
                    {s.key === 'resources' && (
                      <ResourcesSection
                        player={player}
                        busy={busy}
                        currency={currency}
                        setCurrency={setCurrency}
                        scrip={scrip}
                        setScrip={setScrip}
                        intel={intel}
                        setIntel={setIntel}
                        charXP={charXP}
                        setCharXP={setCharXP}
                        charXPCurrent={charXPCurrent}
                        factionId={factionId}
                        setFactionId={setFactionId}
                        repDelta={repDelta}
                        setRepDelta={setRepDelta}
                        skillPointsAmount={skillPointsAmount}
                        setSkillPointsAmount={setSkillPointsAmount}
                        skillModule={skillModule}
                        setSkillModule={setSkillModule}
                        skillModuleLevel={skillModuleLevel}
                        setSkillModuleLevel={setSkillModuleLevel}
                        run={run}
                      />
                    )}
                    {s.key === 'specs' && (
                      <SpecsSection
                        player={player}
                        playerSpecs={playerSpecs}
                        playerKeystones={playerKeystones}
                        specsLoading={specsLoading}
                        busy={busy}
                        run={run}
                        gate={gate}
                        onRefresh={() => setSpecsLoaded(false)}
                        onSpecsUpdate={setPlayerSpecs}
                      />
                    )}
                    {s.key === 'progression' && (
                      <ProgressionSection
                        player={player}
                        busy={busy}
                        presets={presets}
                        presetsLoaded={presetsLoaded}
                        contractCatalog={contractCatalog}
                        contractCatalogLoaded={contractCatalogLoaded}
                        contractCatalogError={contractCatalogError}
                        selectedTrainer={selectedTrainer}
                        setSelectedTrainer={setSelectedTrainer}
                        selectedMQ={selectedMQ}
                        setSelectedMQ={setSelectedMQ}
                        unlockFaction={unlockFaction}
                        setUnlockFaction={setUnlockFaction}
                        unlockPreset={unlockPreset}
                        setUnlockPreset={setUnlockPreset}
                        run={run}
                        gate={gate}
                        onNodesLoaded={() => setNodesLoaded(false)}
                      />
                    )}
                    {s.key === 'contracts' && (
                      <ContractsSection
                        player={player}
                        busy={busy}
                        contractCatalog={contractCatalog}
                        contractCatalogLoaded={contractCatalogLoaded}
                        contractCatalogError={contractCatalogError}
                        contractSearch={contractSearch}
                        setContractSearch={setContractSearch}
                        selectedContracts={selectedContracts}
                        setSelectedContracts={setSelectedContracts}
                        onNodesInvalidate={() => setNodesLoaded(false)}
                        run={run}
                      />
                    )}
                    {s.key === 'journey' && (
                      <JourneySection
                        player={player}
                        busy={busy}
                        nodes={nodes}
                        nodesLoading={nodesLoading}
                        nodeSearch={nodeSearch}
                        setNodeSearch={setNodeSearch}
                        filteredNodes={filteredNodes}
                        run={run}
                        gate={gate}
                        onRefresh={() => setNodesLoaded(false)}
                        onNodesUpdate={setNodes}
                      />
                    )}
                    {s.key === 'admin' && (
                      <AdminSection
                        player={player}
                        busy={busy}
                        partitions={partitions}
                        selectedPartition={selectedPartition}
                        setSelectedPartition={setSelectedPartition}
                        teleportX={teleportX}
                        setTeleportX={setTeleportX}
                        teleportY={teleportY}
                        setTeleportY={setTeleportY}
                        teleportZ={teleportZ}
                        setTeleportZ={setTeleportZ}
                        setShowManageLocations={setShowManageLocations}
                        setShowTeleportMapPicker={setShowTeleportMapPicker}
                        allPlayers={allPlayers}
                        selectedTeleportTarget={selectedTeleportTarget}
                        setSelectedTeleportTarget={setSelectedTeleportTarget}
                        targetSearch={targetSearch}
                        setTargetSearch={setTargetSearch}
                        targetDropdownOpen={targetDropdownOpen}
                        setTargetDropdownOpen={setTargetDropdownOpen}
                        whisperText={whisperText}
                        setWhisperText={setWhisperText}
                        whisperSenderName={whisperSenderName}
                        setWhisperSenderName={setWhisperSenderName}
                        spawnVehicleId={spawnVehicleId}
                        setSpawnVehicleId={setSpawnVehicleId}
                        spawnVehicleTemplate={spawnVehicleTemplate}
                        setSpawnVehicleTemplate={setSpawnVehicleTemplate}
                        spawnVehiclePartition={spawnVehiclePartition}
                        setSpawnVehiclePartition={setSpawnVehiclePartition}
                        spawnVehiclePersistent={spawnVehiclePersistent}
                        setSpawnVehiclePersistent={setSpawnVehiclePersistent}
                        spawnX={spawnX}
                        setSpawnX={setSpawnX}
                        spawnY={spawnY}
                        setSpawnY={setSpawnY}
                        spawnZ={spawnZ}
                        setSpawnZ={setSpawnZ}
                        setShowSpawnMapPicker={setShowSpawnMapPicker}
                        run={run}
                        gate={gate}
                      />
                    )}
                    {s.key === 'tags' && (
                      <TagsSection
                        player={player}
                        tags={tags}
                        tagsLoading={tagsLoading}
                        pendingTags={pendingTags}
                        setPendingTags={setPendingTags}
                        filteredActiveTags={filteredActiveTags}
                        onFilterChange={setTagRemoveSearch}
                        run={run}
                        onAddTag={handleAddTag}
                        onTagsUpdate={setTags}
                      />
                    )}
                    {s.key === 'history' && (
                      <HistorySection
                        events={events}
                        dungeons={dungeons}
                        historyLoading={historyLoading}
                      />
                    )}
                    {s.key === 'experimental' && (
                      <ExperimentalSection
                        player={player}
                        busy={busy}
                        customScriptName={customScriptName}
                        setCustomScriptName={setCustomScriptName}
                        run={run}
                        gate={gate}
                      />
                    )}
                  </div>
                )
              : null,
          )}
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
      {showTeleportMapPicker && (
        <MapCoordPickerModal
          onPick={(x, y, z) => {
            setTeleportX(String(Math.round(x)))
            setTeleportY(String(Math.round(y)))
            setTeleportZ(String(Math.round(z)))
            setShowTeleportMapPicker(false)
          }}
          onClose={() => setShowTeleportMapPicker(false)}
        />
      )}
      {showSpawnMapPicker && (
        <MapCoordPickerModal
          onPick={(x, y, z) => {
            setSpawnX(String(Math.round(x)))
            setSpawnY(String(Math.round(y)))
            setSpawnZ(String(Math.round(z)))
            setShowSpawnMapPicker(false)
          }}
          onClose={() => setShowSpawnMapPicker(false)}
        />
      )}
    </>
  )
}
