import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Separator, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { api } from '../../../api/client'
import type { DiscordGuild, DiscordServerLink, ServerInfo } from '../../../api/client'
import { usePermissions } from '../../../hooks/usePermissions'
import { ActionBar, ConfirmDialog, DataTable, Icon, PageHeader, type Column } from '../../../dune-ui'
import { GuildEditModal } from './GuildEditModal'

type GuildColKey = 'guild_id' | 'servers' | 'roles' | 'actions'

// GuildsPanel is the global Guilds configuration surface: a full-height,
// scrolling table of configured guilds, each carrying ONLY its capability roles
// (edited via GuildEditModal). Servers link to a guild from their own per-server
// Discord tab — that mapping is shown here read-only. Multi-select drives an
// ActionBar mass-delete, consistent with the other list views (e.g. Events).
export const GuildsPanel: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const [guilds, setGuilds] = React.useState<DiscordGuild[]>([])
  const [links, setLinks] = React.useState<DiscordServerLink[]>([])
  const [servers, setServers] = React.useState<ServerInfo[]>([])
  const [loading, setLoading] = React.useState(false)
  const [modalOpen, setModalOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<DiscordGuild | null>(null)
  const [removeTarget, setRemoveTarget] = React.useState<DiscordGuild | null>(null)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())

  const selectionCount = selectedKeys === 'all' ? guilds.length : (selectedKeys as Set<string>).size

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([api.discord.guilds.list(), api.discord.servers.list(), api.servers.list()]))
      .then(([gs, ls, srv]) => {
        setGuilds(gs)
        setLinks(ls)
        setServers(srv)
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  React.useEffect(() => {
    load()
  }, [load])

  const openAdd = () => {
    setEditing(null)
    setModalOpen(true)
  }
  const openEdit = (g: DiscordGuild) => {
    setEditing(g)
    setModalOpen(true)
  }

  const confirmRemove = () => {
    if (!removeTarget) return
    api.discord.guilds.remove(removeTarget.guild_id)
      .then(() => {
        toast.success(t('discordGuilds.removed'))
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.removeFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setRemoveTarget(null))
  }

  const handleBulkDelete = () => {
    const ids = selectedKeys === 'all'
      ? guilds.map((g) => g.guild_id)
      : [...(selectedKeys as Set<string>)]
    setSelectedKeys(new Set())
    Promise.all(ids.map((id) => api.discord.guilds.remove(id)))
      .then(() => {
        toast.success(t('discordGuilds.removed'))
        load()
      })
      .catch((e: unknown) => {
        toast.danger(t('discordGuilds.removeFailed', { message: e instanceof Error ? e.message : String(e) }))
        load()
      })
  }

  const serverName = (id: number) => servers.find((s) => s.id === id)?.name ?? `#${id}`
  const linkedServers = (g: DiscordGuild) => links.filter((l) => l.guild_id === g.guild_id)
  const roleCount = (g: DiscordGuild) =>
    [g.roles_viewer, g.roles_economy, g.roles_admin]
      .reduce((n, csv) => n + csv.split(',').map((s) => s.trim()).filter(Boolean).length, 0)

  const COLUMNS: Column<GuildColKey>[] = [
    { key: 'guild_id', label: t('discordGuilds.columns.guildId'), minWidth: 180 },
    { key: 'servers', label: t('discordGuilds.columns.servers'), minWidth: 220, sortable: false },
    { key: 'roles', label: t('discordGuilds.columns.roles'), width: 120, sortable: false },
    { key: 'actions', label: '', width: 160, sortable: false },
  ]

  return (
    <>
      <div className="flex flex-col h-full gap-3 min-h-0">
        <PageHeader title={t('discordGuilds.title')} subtitle={t('discordGuilds.subtitle')}>
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
            <Icon name="refresh-cw" />
            {' '}
            {t('common.refresh')}
          </Button>
          <Button size="sm" variant="primary" onPress={openAdd}>
            <Icon name="plus" />
            {' '}
            {t('discordGuilds.add')}
          </Button>
        </PageHeader>

        <DataTable<DiscordGuild, GuildColKey>
          aria-label={t('discordGuilds.ariaLabel')}
          columns={COLUMNS}
          rows={guilds}
          loading={loading}
          rowId={(g) => g.guild_id}
          rowHeight={48}
          className="min-h-0 flex-1"
          selectionMode="multiple"
          selectedKeys={selectedKeys}
          onSelectionChange={setSelectedKeys}
          initialSort={{ column: 'guild_id', direction: 'ascending' }}
          sortValue={(g, k) => (k === 'guild_id' ? g.guild_id : '')}
          emptyState={(
            <EmptyState size="sm">
              <EmptyState.Header>
                <EmptyState.Title>{t('discordGuilds.empty')}</EmptyState.Title>
              </EmptyState.Header>
            </EmptyState>
          )}
          renderCell={(g, key) => {
            switch (key) {
              case 'guild_id':
                return <span className="font-mono text-foreground">{g.guild_id}</span>
              case 'servers': {
                const ls = linkedServers(g)
                return ls.length === 0
                  ? <span className="text-muted">—</span>
                  : (
                      <div className="flex flex-wrap gap-1">
                        {ls.map((l) => (
                          <Chip key={l.server_id} size="sm" variant="soft" color={l.status_enabled ? 'success' : 'default'}>
                            {serverName(l.server_id)}
                          </Chip>
                        ))}
                      </div>
                    )
              }
              case 'roles':
                return <span className="text-muted">{t('discordGuilds.roleCount', { count: roleCount(g) })}</span>
              case 'actions':
                return (
                  <div className="flex gap-1">
                    <Button size="sm" variant="outline" onPress={() => openEdit(g)}>
                      <Icon name="pencil" />
                      {' '}
                      {t('common.edit')}
                    </Button>
                    <Button size="sm" variant="danger-soft" onPress={() => setRemoveTarget(g)}>
                      <Icon name="trash-2" />
                    </Button>
                  </div>
                )
            }
          }}
        />
      </div>

      <GuildEditModal
        open={modalOpen}
        existing={editing}
        takenGuildIds={guilds.map((x) => x.guild_id)}
        onClose={() => setModalOpen(false)}
        onSaved={load}
      />

      <ConfirmDialog
        open={!!removeTarget}
        title={t('discordGuilds.removeTitle')}
        description={t('discordGuilds.removeConfirm', { guild: removeTarget?.guild_id ?? '' })}
        confirmLabel={t('common.remove')}
        onConfirm={confirmRemove}
        onCancel={() => setRemoveTarget(null)}
      />

      <ActionBar aria-label={t('discordGuilds.ariaLabel')} isOpen={can('config:write') && selectionCount > 0}>
        <ActionBar.Prefix>
          <Chip size="sm" className="shrink-0 tabular-nums">{selectionCount}</Chip>
        </ActionBar.Prefix>
        <Separator />
        <ActionBar.Content>
          <Button
            size="sm"
            variant="ghost"
            className="text-danger"
            onPress={handleBulkDelete}
            aria-label={t('common.deleteSelected')}
          >
            <Icon name="trash-2" />
            <span className="action-bar__label">{t('common.deleteSelected')}</span>
          </Button>
        </ActionBar.Content>
        <Separator />
        <ActionBar.Suffix>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            onPress={() => setSelectedKeys(new Set())}
            aria-label={t('common.clearSelection')}
          >
            <Icon name="x" />
          </Button>
        </ActionBar.Suffix>
      </ActionBar>
    </>
  )
}
