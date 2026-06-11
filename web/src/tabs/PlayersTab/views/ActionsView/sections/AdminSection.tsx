import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Checkbox, Input, ListBox, SearchField, Select, TextArea, toast } from '@heroui/react'
import { Panel, SectionLabel } from '../../../../../dune-ui'
import { vehiclesSyncAtom } from '../../../../../data/store'
import { api } from '../../../../../api/client'
import { busyAtom, partitionsAtom, allPlayersAtom } from '../store'
import { useRun, useGate } from '../hooks/useActions'
import type { AdminSectionProps } from './types'

export const AdminSection: React.FC<AdminSectionProps> = ({
  player, onManageLocations, onTeleportPicker, onSpawnPicker,
}) => {
  const { t } = useTranslation()
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
  const [targetSearch, setTargetSearch] = React.useState('')
  const [targetDropdownOpen, setTargetDropdownOpen] = React.useState(false)
  const [whisperText, setWhisperText] = React.useState('')
  const [whisperSenderName, setWhisperSenderName] = React.useState('GM')
  const [spawnVehicleId, setSpawnVehicleId] = React.useState('')
  const [spawnVehicleTemplate, setSpawnVehicleTemplate] = React.useState('')
  const [spawnVehiclePartition, setSpawnVehiclePartition] = React.useState('')
  const [spawnVehiclePersistent, setSpawnVehiclePersistent] = React.useState(true)
  const [spawnX, setSpawnX] = React.useState('')
  const [spawnY, setSpawnY] = React.useState('')
  const [spawnZ, setSpawnZ] = React.useState('')

  const handleKick = () =>
    run(() => api.players.kick(player.fls_id), `Kick command sent for ${player.name}`)

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

  const handleTargetSearch = (v: string) => {
    setTargetSearch(v)
    setSelectedTeleportTarget(null)
    setTargetDropdownOpen(true)
  }

  const handleTargetPlayerClick = (targetPlayer: typeof allPlayers[0]) => {
    setTargetSearch(targetPlayer.name)
    setSelectedTeleportTarget(targetPlayer.id)
    setTargetDropdownOpen(false)
  }

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
        <div className="flex items-end gap-2 mt-2">
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
          <div className="relative flex-1" onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setTargetDropdownOpen(false) }}>
            <SearchField
              value={targetSearch}
              onChange={handleTargetSearch}
              onFocus={() => setTargetDropdownOpen(true)}
              className="w-full"
              aria-label={t('players.actions.admin.pickTarget')}
            >
              <SearchField.Group>
                <SearchField.SearchIcon />
                <SearchField.Input
                  placeholder={allPlayers.length === 0 ? t('players.actions.admin.loadingPlayers') : t('players.actions.admin.pickTarget')}
                  aria-label={t('players.actions.admin.selectTargetLabel')}
                  onKeyDown={(e) => { if (e.key === 'Escape') setTargetDropdownOpen(false) }}
                />
                <SearchField.ClearButton />
              </SearchField.Group>
            </SearchField>
            {targetDropdownOpen && (
              <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52 shadow-lg">
                {allPlayers
                  .filter(
                    (p) =>
                      !targetSearch
                      || p.name.toLowerCase().includes(targetSearch.toLowerCase()),
                  )
                  .slice(0, 50)
                  .map((p) => (
                    <button
                      key={p.id}
                      type="button"
                      className="w-full text-left px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover flex items-center justify-between gap-2"
                      onMouseDown={(e) => {
                        e.preventDefault()
                        handleTargetPlayerClick(p)
                      }}
                    >
                      <span className="font-medium">{p.name}</span>
                      <span className="text-muted">
                        {p.map || '—'}
                        {' · '}
                        {p.online_status}
                      </span>
                    </button>
                  ))}
              </div>
            )}
          </div>
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
            <Input
              aria-label={t('players.actions.admin.whisperFrom')}
              value={whisperSenderName}
              onChange={(e) => setWhisperSenderName(e.target.value)}
              placeholder="GM"
              maxLength={32}
              className="w-32"
            />
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
          <div className="flex items-end gap-2 mt-2">
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
            <Checkbox isSelected={spawnVehiclePersistent} onChange={setSpawnVehiclePersistent}>
              <span className="text-xs">{t('players.actions.admin.persistent')}</span>
            </Checkbox>
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
    </div>
  )
}
