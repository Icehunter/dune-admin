import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Input, ListBox, Select, Switch, TextArea, toast } from '@heroui/react'
import { Panel, SectionLabel } from '../../../../../dune-ui'
import { vehiclesSyncAtom } from '../../../../../data/store'
import { api } from '../../../../../api/client'
import { busyAtom, partitionsAtom, allPlayersAtom } from '../store'
import { useRun, useGate } from '../hooks/useActions'
import { usePermissions } from '../../../../../hooks/usePermissions'
import { PlayerSearchField } from '../../../../../components/PlayerSearchField'
import { DeleteCharacterModal } from './DeleteCharacterModal'
import type { AdminSectionProps } from './types'

export const AdminSection: React.FC<AdminSectionProps> = ({
  player, onManageLocations, onTeleportPicker, onSpawnPicker,
}) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canPlayersWrite = can('players:write')
  const canPlayersDelete = can('players:delete')
  const canExportData = can('data:export')
  const [busy] = useAtom(busyAtom(player.id))
  const [partitions] = useAtom(partitionsAtom(player.id))
  const [allPlayers] = useAtom(allPlayersAtom(player.id))
  const run = useRun(player.id)
  const gate = useGate(player.id)
  const [allVehicles] = useAtom(vehiclesSyncAtom)

  const [selectedPartition, setSelectedPartition] = React.useState('')
  const [teleportX, setTeleportX] = React.useState('')
  const [teleportY, setTeleportY] = React.useState('')
  const [teleportZ, setTeleportZ] = React.useState('')
  const [selectedTeleportTarget, setSelectedTeleportTarget] = React.useState<number | null>(null)
  const [whisperText, setWhisperText] = React.useState('')
  const [spawnVehicleId, setSpawnVehicleId] = React.useState('')
  const [spawnVehicleTemplate, setSpawnVehicleTemplate] = React.useState('')
  const [spawnVehiclePartition, setSpawnVehiclePartition] = React.useState('')
  const [spawnVehiclePersistent, setSpawnVehiclePersistent] = React.useState(true)
  const [spawnX, setSpawnX] = React.useState('')
  const [spawnY, setSpawnY] = React.useState('')
  const [spawnZ, setSpawnZ] = React.useState('')

  const handleKick = () =>
    run(() => api.players.kick(player.fls_id), `Kick command sent for ${player.name}`)

  const [deleteModalOpen, setDeleteModalOpen] = React.useState(false)

  const handleDeleteCharacter = (reason: string) => {
    setDeleteModalOpen(false)
    run(
      () => api.players.deleteCharacter(player.account_id, reason),
      t('players.actions.admin.deleteCharacterDone', { player: player.name }),
    )
  }

  const handleWipeInventory = () => gate(
    t('players.actions.admin.wipeInventoryTitle'),
    t('players.actions.admin.wipeInventoryConfirmDesc', { player: player.name }),
    t('players.actions.admin.confirmWipe'),
    () =>
      run(
        () => api.players.cleanInventory(player.fls_id),
        `Inventory wiped for ${player.name}`,
      ),
  )

  const handleResetProgression = () => gate(
    t('players.actions.admin.resetProgressionTitle'),
    t('players.actions.admin.resetProgressionConfirmDesc', { player: player.name }),
    t('players.actions.admin.confirmReset'),
    () =>
      run(
        () => api.players.resetProgression(player.fls_id),
        `Progression reset for ${player.name}`,
      ),
  )

  const handleDeleteTutorials = () => gate(
    t('players.actions.admin.deleteTutorialsTitle'),
    t('players.actions.admin.deleteTutorialsConfirmDesc', { player: player.name }),
    t('players.actions.admin.delete'),
    () =>
      run(
        () => api.players.deleteTutorials(player.id),
        `Deleted tutorials for ${player.name}`,
      ),
  )

  const handleWipeCodex = () => gate(
    t('players.actions.admin.wipeCodexTitle'),
    t('players.actions.admin.wipeCodexConfirmDesc', { player: player.name }),
    t('players.actions.admin.wipe'),
    () =>
      run(
        () => api.players.wipeCodex(player.account_id),
        `Wiped codex for ${player.name}`,
      ),
  )

  const handleDismissReturning = () =>
    run(
      () => api.players.dismissReturningPlayerAward(player.account_id),
      `Dismissed returning player popup for ${player.name}`,
    )

  const handleExportPlayer = () =>
    run(() => api.players.exportPlayer(player.account_id), t('players.actions.admin.exportDownloaded'))

  const handleTeleportToPartition = () =>
    run(
      () => api.players.teleport(player.fls_id, selectedPartition),
      `Teleported ${player.name} to ${selectedPartition}`,
    )

  const handleGetCurrentPosition = async () => {
    try {
      const pos = await api.players.position(player.id)
      setTeleportX(String(Math.round(pos.x)))
      setTeleportY(String(Math.round(pos.y)))
      setTeleportZ(String(Math.round(pos.z)))
    }
    catch {
      toast.danger(t('players.actions.admin.positionReadFailed'))
    }
  }

  const handleTeleportPickerClick = () =>
    onTeleportPicker((x, y, z) => {
      setTeleportX(String(Math.round(x)))
      setTeleportY(String(Math.round(y)))
      setTeleportZ(String(Math.round(z)))
    })

  const handleTeleportToCoords = () =>
    run(
      () =>
        api.players.teleportCoords(
          player.fls_id,
          Number(teleportX) || 0,
          Number(teleportY) || 0,
          Number(teleportZ) || 0,
        ),
      `Teleported ${player.name} to (${teleportX}, ${teleportY}, ${teleportZ})`,
    )

  const handleTeleportToPlayer = () => {
    const target = allPlayers.find((p) => p.id === selectedTeleportTarget)
    if (!target) return
    run(
      () => api.players.teleportToPlayer(player.fls_id, target.id),
      `Teleported ${player.name} to ${target.name}`,
    )
  }

  const handleWhisperSend = () =>
    run(
      () => api.chat.whisper(player.account_id, whisperText.trim()),
      t('players.actions.admin.whisperSent', { player: player.name }),
    ).then(() => setWhisperText(''))

  const handleVehicleSelect = (k: React.Key | null) => {
    const id = k ? String(k) : ''
    setSpawnVehicleId(id)
    const v = allVehicles.find((x) => x.id === id)
    setSpawnVehicleTemplate(v?.templates[0] ?? '')
  }

  const handleSpawnPartitionSelect = (k: React.Key | null) => {
    setSpawnVehiclePartition(k ? String(k) : '')
    const p = partitions.find((x) => x.name === String(k))
    if (p) {
      setSpawnX(String(Math.round(p.x)))
      setSpawnY(String(Math.round(p.y)))
      setSpawnZ(String(Math.round(p.z)))
    }
  }

  const handleGetSpawnPosition = async () => {
    try {
      const pos = await api.players.position(player.id)
      setSpawnX(String(Math.round(pos.x)))
      setSpawnY(String(Math.round(pos.y)))
      setSpawnZ(String(Math.round(pos.z)))
    }
    catch {
      toast.danger(t('players.actions.admin.positionReadFailed'))
    }
  }

  const handleSpawnPickerClick = () =>
    onSpawnPicker((x, y, z) => {
      setSpawnX(String(Math.round(x)))
      setSpawnY(String(Math.round(y)))
      setSpawnZ(String(Math.round(z)))
    })

  const handleSpawnVehicle = () => {
    const v = allVehicles.find((x) => x.id === spawnVehicleId)
    if (!v) return
    run(
      () =>
        api.players.spawnVehicle(
          player.fls_id,
          v.actor_class,
          Number(spawnX) || 0,
          Number(spawnY) || 0,
          Number(spawnZ) || 0,
          {
            template_name: spawnVehicleTemplate || undefined,
            persistent: spawnVehiclePersistent,
          },
        ),
      `Spawn ${spawnVehicleId} command sent for ${player.name}`,
    )
  }

  const actionRow = (
    label: string,
    inputs: React.ReactNode,
    btnLabel: string,
    onAction: () => void,
    danger = false,
    confirmGate?: { title: string, description: string },
  ) => (
    <div className="flex items-end gap-3 py-3 border-b border-border/40 last:border-b-0">
      <div className="w-36 shrink-0 text-sm text-muted">{label}</div>
      <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
      <Button
        size="sm"
        variant={danger ? 'danger-soft' : 'ghost'}
        isDisabled={busy}
        onPress={confirmGate ? () => gate(confirmGate.title, confirmGate.description, btnLabel, onAction) : onAction}
      >
        {btnLabel}
      </Button>
    </div>
  )

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <DeleteCharacterModal
        open={deleteModalOpen}
        playerName={player.name}
        online={player.online_status === 'Online'}
        busy={busy}
        onCancel={() => setDeleteModalOpen(false)}
        onConfirm={handleDeleteCharacter}
      />
      {canPlayersWrite && (
        <>
          <Panel>
            <SectionLabel>{t('players.actions.admin.liveActions')}</SectionLabel>
            <div className="text-xs text-muted mb-2">{t('players.actions.admin.liveActionsDesc')}</div>
            {actionRow(
              t('players.actions.admin.kickPlayer'),
              <span className="text-xs text-muted">
                {t('players.actions.admin.kickDesc')}
              </span>,
              t('players.actions.admin.kick'),
              handleKick,
            )}
          </Panel>

          <Panel>
            <SectionLabel>{t('players.actions.admin.destructive')}</SectionLabel>
            <div className="text-xs text-muted mb-2">{t('players.actions.admin.destructiveDesc')}</div>
            <div className="flex items-end gap-3 py-3 border-b border-border/40">
              <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.admin.wipeInventory')}</div>
              <div className="flex-1 text-xs text-muted">{t('players.actions.admin.wipeInventoryDesc')}</div>
              <Button
                size="sm"
                variant="danger-soft"
                isDisabled={busy}
                onPress={handleWipeInventory}
              >
                {t('players.actions.admin.wipe')}
              </Button>
            </div>
            <div className="flex items-end gap-3 py-3">
              <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.admin.resetProgression')}</div>
              <div className="flex-1 text-xs text-muted">{t('players.actions.admin.resetProgressionDesc')}</div>
              <Button
                size="sm"
                variant="danger-soft"
                isDisabled={busy}
                onPress={handleResetProgression}
              >
                {t('players.actions.admin.confirmReset')}
              </Button>
            </div>
          </Panel>

          <Panel>
            <SectionLabel>{t('players.actions.admin.resetActions')}</SectionLabel>
            {actionRow(
              t('players.actions.admin.deleteTutorials'),
              <span className="text-xs text-muted">
                {t('players.actions.admin.deleteTutorialsDesc')}
              </span>,
              t('players.actions.admin.delete'),
              handleDeleteTutorials,
              true,
            )}
            {actionRow(
              t('players.actions.admin.wipeCodex'),
              <span className="text-xs text-muted">
                {t('players.actions.admin.wipeCodexDesc')}
              </span>,
              t('players.actions.admin.wipe'),
              handleWipeCodex,
              true,
            )}
            {actionRow(
              t('players.actions.admin.dismissReturning'),
              <span className="text-xs text-muted">
                {t('players.actions.admin.dismissReturningDesc')}
              </span>,
              t('players.actions.admin.dismiss'),
              handleDismissReturning,
              true,
            )}
          </Panel>
        </>
      )}

      {canPlayersDelete && (
        <Panel>
          <SectionLabel>{t('players.actions.admin.deleteCharacter')}</SectionLabel>
          <div className="text-xs text-muted mb-2">{t('players.actions.admin.deleteCharacterDesc')}</div>
          <div className="flex items-end gap-3 py-1">
            <div className="flex-1 text-xs text-danger">{t('players.actions.admin.deleteCharacterIrreversible')}</div>
            <Button
              size="sm"
              variant="danger-soft"
              isDisabled={busy}
              onPress={() => setDeleteModalOpen(true)}
            >
              {t('players.actions.admin.deleteCharacterButton')}
            </Button>
          </div>
        </Panel>
      )}

      {canExportData && (
        <Panel>
          <SectionLabel>{t('players.actions.admin.characterExport')}</SectionLabel>
          <div className="flex items-end gap-3 py-1">
            <div className="flex-1 text-xs text-muted">{t('players.actions.admin.characterExportDesc')}</div>
            <Button
              size="sm"
              variant="ghost"
              isDisabled={busy}
              onPress={handleExportPlayer}
            >
              {t('players.actions.admin.downloadExport')}
            </Button>
          </div>
        </Panel>
      )}

      {canPlayersWrite && (
        <>
          <Panel>
            <div className="flex items-center justify-between mb-1">
              <SectionLabel>{t('players.actions.admin.teleport')}</SectionLabel>
              <Button size="sm" variant="ghost" onPress={onManageLocations}>{t('players.actions.admin.manageLocations')}</Button>
            </div>
            <div className="flex items-end gap-3 py-1">
              <Select
                aria-label={t('players.actions.admin.teleport')}
                placeholder={t('players.actions.admin.teleportPlaceholder')}
                selectedKey={selectedPartition || null}
                onSelectionChange={(k) => setSelectedPartition(k ? String(k) : '')}
                className="flex-1"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {partitions.map((p) => (
                      <ListBox.Item key={p.name} id={p.name} textValue={p.name}>
                        {p.name}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Button
                size="sm"
                variant="ghost"
                isDisabled={busy || !selectedPartition}
                onPress={handleTeleportToPartition}
              >
                {t('players.actions.admin.move')}
              </Button>
            </div>
            <div className="flex gap-2 mt-2 items-center">
              <Input aria-label="X" className="w-24" value={teleportX} onChange={(e) => setTeleportX(e.target.value)} placeholder="X" />
              <Input aria-label="Y" className="w-24" value={teleportY} onChange={(e) => setTeleportY(e.target.value)} placeholder="Y" />
              <Input aria-label="Z" className="w-24" value={teleportZ} onChange={(e) => setTeleportZ(e.target.value)} placeholder="Z" />
              <Button
                size="sm"
                variant="ghost"
                isDisabled={busy}
                onPress={handleGetCurrentPosition}
              >
                {t('players.actions.admin.useCurrent')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isDisabled={busy}
                onPress={handleTeleportPickerClick}
              >
                {t('players.actions.admin.pickOnMap')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isDisabled={busy || (!teleportX && !teleportY)}
                onPress={handleTeleportToCoords}
              >
                {t('players.actions.admin.moveToXyz')}
              </Button>
            </div>
            <span className="text-xs text-muted mt-1">{t('players.actions.admin.teleportNote')}</span>
          </Panel>

          <Panel>
            <SectionLabel>{t('players.actions.admin.teleportToPlayer')}</SectionLabel>
            <div className="text-xs text-muted mb-2">
              Drop
              {player.name}
              {' '}
              exactly on another character&apos;s current position.
            </div>
            <div className="flex items-center gap-3">
              <PlayerSearchField
                className="flex-1"
                ariaLabel={t('players.actions.admin.pickTarget')}
                placeholder={allPlayers.length === 0 ? t('players.actions.admin.loadingPlayers') : t('players.actions.admin.pickTarget')}
                players={allPlayers}
                onSelect={(p) => setSelectedTeleportTarget(p.id)}
                onClear={() => setSelectedTeleportTarget(null)}
              />
              <Button
                size="sm"
                isDisabled={busy || selectedTeleportTarget == null}
                onPress={handleTeleportToPlayer}
              >
                {t('players.actions.admin.move')}
              </Button>
            </div>
          </Panel>

          <Panel>
            <SectionLabel>{t('players.actions.admin.whisper')}</SectionLabel>
            <div className="text-xs text-muted mb-2">
              Send a private chat message to
              {' '}
              {player.name}
              .
              {' '}
              <span className="text-warning">Experimental</span>
            </div>
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted shrink-0">{t('players.actions.admin.whisperFrom')}</span>
                <span className="text-xs text-foreground font-mono">GM</span>
              </div>
              <TextArea
                aria-label={t('players.actions.admin.whisper')}
                value={whisperText}
                onChange={(e) => setWhisperText(e.target.value)}
                placeholder={`Message to ${player.name}…`}
                rows={2}
                maxLength={500}
                fullWidth
                style={{ resize: 'vertical' }}
              />
              <div className="flex items-center justify-end gap-2">
                <span className="text-xs text-muted">
                  {whisperText.length}
                  {' '}
                  / 500
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy || !whisperText.trim()}
                  onPress={handleWhisperSend}
                >
                  Send
                </Button>
              </div>
            </div>
          </Panel>

          <Panel>
            <SectionLabel>{t('players.actions.admin.spawnVehicle')}</SectionLabel>
            <div className="text-xs text-muted mb-2">{t('players.actions.admin.spawnVehicleDesc')}</div>
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <Select
                  aria-label={t('players.actions.admin.vehicleLabel')}
                  placeholder={t('players.actions.admin.selectVehicle')}
                  selectedKey={spawnVehicleId || null}
                  onSelectionChange={handleVehicleSelect}
                  className="flex-1"
                >
                  <Select.Trigger>
                    <Select.Value />
                    <Select.Indicator />
                  </Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      {allVehicles.map((v) => (
                        <ListBox.Item key={v.id} id={v.id} textValue={v.label}>
                          {v.label}
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                      ))}
                    </ListBox>
                  </Select.Popover>
                </Select>
                {spawnVehicleId && (() => {
                  const templates = allVehicles.find((v) => v.id === spawnVehicleId)?.templates ?? []
                  return templates.length > 1
                    ? (
                        <Select
                          aria-label={t('players.actions.admin.templateLabel')}
                          selectedKey={spawnVehicleTemplate || null}
                          onSelectionChange={(k) => setSpawnVehicleTemplate(k ? String(k) : '')}
                          className="w-44"
                        >
                          <Select.Trigger>
                            <Select.Value />
                            <Select.Indicator />
                          </Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              {templates.map((tmpl) => (
                                <ListBox.Item key={tmpl} id={tmpl} textValue={tmpl}>
                                  {tmpl}
                                  <ListBox.ItemIndicator />
                                </ListBox.Item>
                              ))}
                            </ListBox>
                          </Select.Popover>
                        </Select>
                      )
                    : null
                })()}
              </div>
              <div className="flex items-center gap-2">
                <Select
                  aria-label={t('players.actions.admin.spawnLocationLabel')}
                  placeholder={t('players.actions.admin.selectSpawnLocation')}
                  selectedKey={spawnVehiclePartition || null}
                  onSelectionChange={handleSpawnPartitionSelect}
                  className="flex-1"
                >
                  <Select.Trigger>
                    <Select.Value />
                    <Select.Indicator />
                  </Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      {partitions.map((p) => (
                        <ListBox.Item key={p.name} id={p.name} textValue={p.name}>
                          {p.name}
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                      ))}
                    </ListBox>
                  </Select.Popover>
                </Select>
              </div>
              <div className="flex gap-2 mt-2 items-center">
                <Input aria-label="X" className="w-24" value={spawnX} onChange={(e) => setSpawnX(e.target.value)} placeholder="X" />
                <Input aria-label="Y" className="w-24" value={spawnY} onChange={(e) => setSpawnY(e.target.value)} placeholder="Y" />
                <Input aria-label="Z" className="w-24" value={spawnZ} onChange={(e) => setSpawnZ(e.target.value)} placeholder="Z" />
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy}
                  onPress={handleGetSpawnPosition}
                >
                  {t('players.actions.admin.useCurrent')}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy}
                  onPress={handleSpawnPickerClick}
                >
                  {t('players.actions.admin.pickOnMap')}
                </Button>
                <Switch isSelected={spawnVehiclePersistent} onChange={setSpawnVehiclePersistent} size="sm">
                  <Switch.Control><Switch.Thumb /></Switch.Control>
                  <Switch.Content><span className="text-xs">{t('players.actions.admin.persistent')}</span></Switch.Content>
                </Switch>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy || !spawnVehicleId || (!spawnX && !spawnY)}
                  onPress={handleSpawnVehicle}
                >
                  {t('players.actions.admin.spawn')}
                </Button>
              </div>
            </div>
          </Panel>
        </>
      )}
    </div>
  )
}
