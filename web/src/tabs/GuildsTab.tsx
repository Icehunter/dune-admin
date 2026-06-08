import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Modal, Spinner, toast } from '@heroui/react'
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

export const GuildsTab: React.FC = () => {
  const { t } = useTranslation()
  const [guilds, setGuilds] = useState<GuildSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [detail, setDetail] = useState<GuildDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [open, setOpen] = useState(false)

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

  const openDetail = (id: number) => {
    setOpen(true)
    setDetail(null)
    setDetailLoading(true)
    api.guilds.get(id)
      .then(setDetail)
      .catch((e: unknown) =>
        toast.danger(t('guilds.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setDetailLoading(false))
  }

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
        emptyState={<div className="py-8 text-center text-muted">{t('guilds.empty')}</div>}
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
                  {t('guilds.view')}
                </Button>
              )
          }
        }}
      />

      <Modal>
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
                    {detail.description && <p className="text-sm text-muted">{detail.description}</p>}

                    <div>
                      <SectionLabel>{t('guilds.members')}</SectionLabel>
                      {detail.members.length === 0
                        ? <div className="text-xs text-muted py-1">{t('guilds.noMembers')}</div>
                        : (
                            <div className="mt-1">
                              {detail.members.map((m) => (
                                <div
                                  key={m.player_id}
                                  className="flex items-center justify-between py-1.5 border-b border-border/40 text-sm"
                                >
                                  <span className="text-foreground">{m.character_name}</span>
                                  <span className="text-xs text-muted">{t('guilds.roleN', { id: m.role_id })}</span>
                                </div>
                              ))}
                            </div>
                          )}
                    </div>

                    <div>
                      <SectionLabel>{t('guilds.invites')}</SectionLabel>
                      {detail.invites.length === 0
                        ? <div className="text-xs text-muted py-1">{t('guilds.noInvites')}</div>
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
      </Modal>
    </div>
  )
}
