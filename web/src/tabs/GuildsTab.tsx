import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Input, Modal, Spinner, TextArea, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../api/client'
import type { GuildSummary, GuildDetail } from '../api/client'
import { DataTable, Icon, PageHeader, SectionLabel, type Column } from '../dune-ui'

type Key = 'name' | 'faction' | 'members' | 'description' | 'actions'

// Faction names are the stable dune.factions enum (Atreides/Harkonnen/None/
// Smuggler), so colour-coding by name is safe. Unknown/None → default.
const FACTION_COLOR: Record<string, 'accent' | 'danger' | 'warning' | 'default'> = {
  Atreides: 'accent',
  Harkonnen: 'danger',
  Smuggler: 'warning',
}

// Confirmed guild role ids (dune guild procs): 100 = admin, 50 = member.
const ROLE_ADMIN = 100
const ROLE_MEMBER = 50

interface GuildsTabProps {
  isSignedIn?: boolean
}

export const GuildsTab: React.FC<GuildsTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()
  const [guilds, setGuilds] = useState<GuildSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [detail, setDetail] = useState<GuildDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [saving, setSaving] = useState(false)
  const [roleBusy, setRoleBusy] = useState(false)

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.guilds.list())
      .then(setGuilds)
      .catch((e: unknown) =>
        toast.danger(t('guilds.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const applyDetail = (d: GuildDetail) => {
    setDetail(d)
    setEditName(d.name)
    setEditDesc(d.description)
  }

  const openDetail = (id: number) => {
    setOpen(true)
    setDetail(null)
    setDetailLoading(true)
    api.guilds.get(id)
      .then(applyDetail)
      .catch((e: unknown) =>
        toast.danger(t('guilds.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setDetailLoading(false))
  }

  const save = () => {
    if (!detail) return
    setSaving(true)
    api.guilds.update(detail.guild_id, { name: editName.trim(), description: editDesc })
      .then((d) => {
        applyDetail(d)
        toast.success(t('guilds.saved'))
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('guilds.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const makeAdmin = (playerId: number) => {
    if (!detail) return
    setRoleBusy(true)
    api.guilds.setRole(detail.guild_id, playerId, ROLE_ADMIN)
      .then(() => api.guilds.get(detail.guild_id))
      .then((d) => {
        applyDetail(d)
        toast.success(t('guilds.roleChanged'))
      })
      .catch((e: unknown) =>
        toast.danger(t('guilds.roleChangeFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setRoleBusy(false))
  }

  const roleLabel = (id: number) =>
    id === ROLE_ADMIN ? t('guilds.roleAdmin') : id === ROLE_MEMBER ? t('guilds.roleMember') : t('guilds.roleN', { id })

  const COLUMNS: Column<Key>[] = [
    { key: 'name', label: t('guilds.columns.name'), minWidth: 200 },
    { key: 'faction', label: t('guilds.columns.faction'), width: 150 },
    { key: 'members', label: t('guilds.columns.members'), width: 110 },
    { key: 'description', label: t('guilds.columns.description'), minWidth: 240 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('guilds.title', { count: guilds.length })} subtitle={t('guilds.subtitle')}>
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      <DataTable<GuildSummary, Key>
        aria-label={t('guilds.title', { count: guilds.length })}
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={guilds}
        loading={loading}
        rowId={(g) => String(g.guild_id)}
        initialSort={{ column: 'name', direction: 'ascending' }}
        sortValue={(g, k) => {
          switch (k) {
            case 'name': return g.name
            case 'faction': return g.faction_name
            case 'members': return g.member_count
            case 'description': return g.description
            default: return ''
          }
        }}
        emptyState={(
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:persons" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('guilds.empty')}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}
        renderCell={(g, key) => {
          switch (key) {
            case 'name':
              return g.name || <span className="text-muted">—</span>
            case 'faction':
              return (
                <Chip size="sm" variant="soft" color={FACTION_COLOR[g.faction_name] ?? 'default'}>
                  {g.faction_name || '—'}
                </Chip>
              )
            case 'members':
              return <span className="text-muted">{g.member_count}</span>
            case 'description':
              return g.description
                ? <span className="text-muted">{g.description}</span>
                : <span className="text-muted">—</span>
            case 'actions':
              return (
                <Button size="sm" variant="outline" className="w-full" onPress={() => openDetail(g.guild_id)}>
                  <Icon name="users" />
                  {' '}
                  {isSignedIn ? t('guilds.manage') : t('guilds.view')}
                </Button>
              )
          }
        }}
      />

      <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && setOpen(false)}>
        <Modal.Container size="lg" scroll="outside">
          <Modal.Dialog className="max-h-[85vh] flex flex-col">
            <Modal.CloseTrigger />
            <Modal.Header>
              <div className="flex items-baseline gap-3 flex-wrap">
                <Modal.Heading className="text-accent">{detail?.name || t('guilds.title', { count: 0 })}</Modal.Heading>
                {detail && (
                  <Chip size="sm" variant="soft" color={FACTION_COLOR[detail.faction_name] ?? 'default'}>
                    {detail.faction_name || '—'}
                  </Chip>
                )}
              </div>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4 overflow-y-auto">
              {detailLoading && (
                <div className="flex items-center justify-center py-8 gap-2 text-muted">
                  <Spinner size="sm" color="current" />
                </div>
              )}
              {!detailLoading && detail && (
                <>
                  {isSignedIn
                    ? (
                        <div className="flex flex-col gap-3">
                          <SectionLabel>{t('guilds.editGuild')}</SectionLabel>
                          <div>
                            <label className="text-xs text-muted">{t('guilds.nameLabel')}</label>
                            <Input
                              aria-label={t('guilds.nameLabel')}
                              className="w-full"
                              value={editName}
                              onChange={(e) => setEditName(e.target.value)}
                            />
                          </div>
                          <div>
                            <label className="text-xs text-muted">{t('guilds.descLabel')}</label>
                            <TextArea
                              aria-label={t('guilds.descLabel')}
                              fullWidth
                              rows={2}
                              value={editDesc}
                              onChange={(e) => setEditDesc(e.target.value)}
                            />
                          </div>
                          <div>
                            <Button size="sm" onPress={save} isDisabled={saving || editName.trim() === ''}>
                              {saving ? <Spinner size="sm" color="current" /> : t('guilds.save')}
                            </Button>
                          </div>
                        </div>
                      )
                    : detail.description && <p className="text-sm text-muted">{detail.description}</p>}

                  <div>
                    <SectionLabel>{t('guilds.members')}</SectionLabel>
                    {detail.members.length === 0
                      ? (
                          <EmptyState size="sm">
                            <EmptyState.Header>
                              <EmptyState.Title>{t('guilds.noMembers')}</EmptyState.Title>
                            </EmptyState.Header>
                          </EmptyState>
                        )
                      : (
                          <div className="mt-1">
                            {detail.members.map((m) => (
                              <div
                                key={m.player_id}
                                className="flex items-center justify-between py-1.5 border-b border-border/40 text-sm gap-2"
                              >
                                <span className="text-foreground flex-1 truncate">{m.character_name}</span>
                                <Chip size="sm" variant="soft" color={m.role_id === ROLE_ADMIN ? 'accent' : 'default'}>
                                  {roleLabel(m.role_id)}
                                </Chip>
                                {isSignedIn && m.role_id !== ROLE_ADMIN && (
                                  <Button
                                    size="sm"
                                    variant="outline"
                                    isDisabled={roleBusy}
                                    onPress={() => makeAdmin(m.player_id)}
                                  >
                                    {t('guilds.makeAdmin')}
                                  </Button>
                                )}
                              </div>
                            ))}
                          </div>
                        )}
                  </div>

                  <div>
                    <SectionLabel>{t('guilds.invites')}</SectionLabel>
                    {detail.invites.length === 0
                      ? (
                          <EmptyState size="sm">
                            <EmptyState.Header>
                              <EmptyState.Title>{t('guilds.noInvites')}</EmptyState.Title>
                            </EmptyState.Header>
                          </EmptyState>
                        )
                      : (
                          <div className="mt-1">
                            {detail.invites.map((iv) => (
                              <div
                                key={iv.invite_id}
                                className="flex items-center justify-between py-1.5 border-b border-border/40 text-sm"
                              >
                                <span className="text-foreground">{iv.character_name}</span>
                                <span className="text-xs text-muted">{t('guilds.invitedBy', { name: iv.sender_name })}</span>
                              </div>
                            ))}
                          </div>
                        )}
                  </div>
                </>
              )}
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </div>
  )
}
