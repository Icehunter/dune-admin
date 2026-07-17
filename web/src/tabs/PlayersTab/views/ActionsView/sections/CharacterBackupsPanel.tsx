import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { Panel, SectionLabel, Icon } from '../../../../../dune-ui'
import { api } from '../../../../../api/client'
import type { CharacterBackup } from '../../../../../api/client'
import { busyAtom } from '../store'
import { useRun, useGate } from '../hooks/useActions'
import { usePermissions } from '../../../../../hooks/usePermissions'
import { BackupScopeCard } from './BackupScopeCard'
import { CharacterRestoreModal } from './CharacterRestoreModal'
import type { CharacterBackupsPanelProps } from './interfaces'

const fmtDate = (iso: string): string => new Date(iso).toLocaleString()

export const CharacterBackupsPanel: React.FC<CharacterBackupsPanelProps> = ({ player }): React.ReactElement => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canManage = can('backups:manage')
  const [busy, setBusy] = useAtom(busyAtom(player.id))
  const run = useRun(player.id)
  const gate = useGate(player.id)
  const [backups, setBackups] = React.useState<CharacterBackup[]>([])
  const [loading, setLoading] = React.useState(true)
  const [backingUp, setBackingUp] = React.useState(false)
  const [restoreTarget, setRestoreTarget] = React.useState<CharacterBackup | null>(null)
  const online = player.online_status === 'Online'

  const load = (): void => {
    setLoading(true)
    api.players.listBackups(player.account_id)
      .then(setBackups)
      .catch(() => setBackups([]))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    void Promise.resolve().then(load)
  }, [player.account_id]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleBackupNow = (): void => {
    setBusy(true)
    setBackingUp(true)
    api.players.backupCharacter(player.account_id, player.name, t('players.actions.admin.backupsManualReason'))
      .then(() => {
        toast.success(t('players.actions.admin.backupsCreated'))
        load()
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => {
        setBackingUp(false)
        setBusy(false)
      })
  }

  const handleRestoreConfirmed = (backup: CharacterBackup): void => {
    setRestoreTarget(null)
    void run(
      () => api.characterBackups.restore(backup.id),
      t('players.actions.admin.backupsRestored'),
    )
  }

  const handleDelete = (backup: CharacterBackup): void => gate(
    t('players.actions.admin.backupsDeleteTitle'),
    t('players.actions.admin.backupsDeleteConfirmDesc'),
    t('players.actions.admin.delete'),
    () => {
      setBusy(true)
      api.characterBackups.remove(backup.id)
        .then(() => {
          toast.success(t('players.actions.admin.backupsDeleted'))
          load()
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setBusy(false))
    },
  )

  return (
    <Panel>
      <SectionLabel>{t('players.actions.admin.characterBackups')}</SectionLabel>
      <div className="text-xs text-muted mb-2">{t('players.actions.admin.characterBackupsDesc')}</div>
      <div className="mb-2"><BackupScopeCard /></div>
      {canManage && (
        <div className="flex items-end gap-3 py-1 border-b border-border/40 mb-2">
          <div className="flex-1 text-xs text-muted">
            {online ? t('players.actions.admin.backupsOfflineRequired') : t('players.actions.admin.backupsManualDesc')}
          </div>
          <Button
            size="sm"
            variant="ghost"
            isDisabled={busy || online}
            onPress={handleBackupNow}
          >
            {backingUp ? <Spinner size="sm" color="current" /> : t('players.actions.admin.backupsCreateNow')}
          </Button>
        </div>
      )}
      {loading
        ? (
            <div className="py-3 flex justify-center">
              <Spinner size="sm" color="current" />
            </div>
          )
        : backups.length === 0
          ? (
              <EmptyState size="sm">
                <EmptyState.Header>
                  <EmptyState.Media variant="icon">
                    <IconifyIcon icon="gravity-ui:document" className="size-5" />
                  </EmptyState.Media>
                  <EmptyState.Title>{t('players.actions.admin.backupsEmpty')}</EmptyState.Title>
                </EmptyState.Header>
              </EmptyState>
            )
          : (
              <div className="flex flex-col gap-1">
                {backups.map((b) => (
                  <div
                    key={b.id}
                    className="grid grid-cols-[1fr_auto] gap-3 items-center px-2 py-1.5 rounded bg-surface border border-border/40"
                  >
                    <div className="flex flex-col min-w-0">
                      <span className="text-sm">{fmtDate(b.created_at)}</span>
                      <span className="text-xs text-muted truncate">
                        {b.action}
                        {b.reason ? ` — ${b.reason}` : ''}
                      </span>
                    </div>
                    <div className="flex items-center gap-1">
                      {canManage && (
                        <Button size="sm" variant="outline" isDisabled={busy} onPress={() => setRestoreTarget(b)}>
                          {t('players.actions.admin.backupsRestore')}
                        </Button>
                      )}
                      <a href={api.characterBackups.downloadUrl(b.id)} download>
                        <Button size="sm" variant="ghost" isIconOnly aria-label={t('players.actions.admin.backupsDownload')}>
                          <Icon name="download" />
                        </Button>
                      </a>
                      {canManage && (
                        <Button
                          size="sm"
                          variant="ghost"
                          isIconOnly
                          aria-label={t('players.actions.admin.backupsDeleteLabel')}
                          isDisabled={busy}
                          onPress={() => handleDelete(b)}
                        >
                          <Icon name="trash-2" />
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
      <CharacterRestoreModal
        backup={restoreTarget}
        playerName={player.name}
        busy={busy}
        onCancel={() => setRestoreTarget(null)}
        onConfirm={handleRestoreConfirmed}
      />
    </Panel>
  )
}
