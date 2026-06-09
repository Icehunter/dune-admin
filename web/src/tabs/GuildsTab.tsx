import { useEffect, useState } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Spinner } from '@/components/ui/spinner'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api } from '../api/client'
import type { GuildSummary } from '../api/client'
import { qk } from '../api/queryKeys'
import { DataTable, Icon, PageHeader, SectionLabel, type Column } from '../dune-ui'

type Key = 'name' | 'faction' | 'members' | 'description'

// Faction names are the stable dune.factions enum (Atreides/Harkonnen/None/
// Smuggler), so colour-coding by name is safe. Unknown/None → default.
const FACTION_TONE: Record<string, 'accent' | 'danger' | 'warning' | 'default'> = {
  Atreides: 'accent',
  Harkonnen: 'danger',
  Smuggler: 'warning',
}

// Confirmed guild role ids (dune guild procs): 100 = admin, 50 = member.
const ROLE_ADMIN = 100
const ROLE_MEMBER = 50

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

interface GuildsTabProps {
  isSignedIn?: boolean
}

export const GuildsTab: React.FC<GuildsTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [seededId, setSeededId] = useState<number | null>(null)

  const list = useQuery({ queryKey: qk.guilds.list, queryFn: api.guilds.list })

  const detail = useQuery({
    queryKey: ['guilds', 'detail', selectedId],
    queryFn: () => api.guilds.get(selectedId as number),
    enabled: selectedId != null,
  })
  const d = detail.data

  // Seed the editable form once per opened guild — a render-time state
  // adjustment (the React-blessed alternative to a setState-in-effect). The
  // guard flips false after the first run for a given selection, so it can't
  // loop, and a later detail update (e.g. after save) won't clobber edits.
  if (d && seededId !== selectedId) {
    setSeededId(selectedId)
    setEditName(d.name)
    setEditDesc(d.description)
  }

  useEffect(() => {
    if (list.error) toast.danger(t('guilds.failedToLoad', { message: errMsg(list.error) }))
  }, [list.error, t])
  useEffect(() => {
    if (detail.error) toast.danger(t('guilds.failedToLoad', { message: errMsg(detail.error) }))
  }, [detail.error, t])

  const saveMutation = useMutation({
    mutationFn: (vars: { id: number, name: string, description: string }) =>
      api.guilds.update(vars.id, { name: vars.name, description: vars.description }),
    onSuccess: (updated) => {
      toast.success(t('guilds.saved'))
      queryClient.setQueryData(['guilds', 'detail', updated.guild_id], updated)
      queryClient.invalidateQueries({ queryKey: qk.guilds.list })
    },
    onError: (e) => toast.danger(t('guilds.saveFailed', { message: errMsg(e) })),
  })

  const roleMutation = useMutation({
    mutationFn: (vars: { guildId: number, playerId: number, role: number }) =>
      api.guilds.setRole(vars.guildId, vars.playerId, vars.role),
    onSuccess: (_res, vars) => {
      toast.success(t('guilds.roleChanged'))
      queryClient.invalidateQueries({ queryKey: ['guilds', 'detail', vars.guildId] })
      queryClient.invalidateQueries({ queryKey: qk.guilds.list })
    },
    onError: (e) => toast.danger(t('guilds.roleChangeFailed', { message: errMsg(e) })),
  })

  const guilds = list.data ?? []

  const roleLabel = (id: number) =>
    id === ROLE_ADMIN ? t('guilds.roleAdmin') : id === ROLE_MEMBER ? t('guilds.roleMember') : t('guilds.roleN', { id })

  const promote = (playerId: number) => {
    if (d) roleMutation.mutate({ guildId: d.guild_id, playerId, role: ROLE_ADMIN })
  }

  const COLUMNS: Column<Key>[] = [
    { key: 'name', label: t('guilds.columns.name'), minWidth: 200 },
    { key: 'faction', label: t('guilds.columns.faction'), width: 150 },
    { key: 'members', label: t('guilds.columns.members'), width: 110 },
    { key: 'description', label: t('guilds.columns.description'), minWidth: 240 },
  ]

  const onSave = (e: React.FormEvent) => {
    e.preventDefault()
    if (selectedId != null) saveMutation.mutate({ id: selectedId, name: editName.trim(), description: editDesc })
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('guilds.title', { count: guilds.length })} subtitle={t('guilds.subtitle')}>
        <Button size="sm" variant="ghost" onClick={() => list.refetch()} disabled={list.isFetching}>
          {list.isFetching
            ? (
                <Spinner size="sm" color="current" />
              )
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
        loading={list.isLoading}
        rowId={(g) => String(g.guild_id)}
        initialSort={{ column: 'name', direction: 'ascending' }}
        onRowAction={(g) => setSelectedId(g.guild_id)}
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
              return <Badge tone={FACTION_TONE[g.faction_name] ?? 'default'}>{g.faction_name || '—'}</Badge>
            case 'members':
              return <span className="text-muted">{g.member_count}</span>
            case 'description':
              return g.description
                ? <span className="text-muted">{g.description}</span>
                : <span className="text-muted">—</span>
          }
        }}
      />

      <Dialog open={selectedId != null} onOpenChange={(v) => { if (!v) setSelectedId(null) }}>
        <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-lg">
          <DialogHeader>
            <div className="flex items-center gap-3 flex-wrap">
              <DialogTitle>{d?.name || t('guilds.title', { count: 0 })}</DialogTitle>
              {d && (
                <Badge tone={FACTION_TONE[d.faction_name] ?? 'default'}>{d.faction_name || '—'}</Badge>
              )}
            </div>
          </DialogHeader>

          <div className="mt-2 -mr-2 flex flex-col gap-4 overflow-y-auto pr-2">
            {detail.isLoading && (
              <div className="flex items-center justify-center gap-2 py-8 text-muted">
                <Spinner size="sm" color="current" />
              </div>
            )}

            {!detail.isLoading && d && (
              <>
                {isSignedIn
                  ? (
                      <form onSubmit={onSave} className="flex flex-col gap-3">
                        <SectionLabel>{t('guilds.editGuild')}</SectionLabel>
                        <div className="flex flex-col gap-1">
                          <Label htmlFor="guild-name">{t('guilds.nameLabel')}</Label>
                          <Input id="guild-name" value={editName} onChange={(e) => setEditName(e.target.value)} />
                        </div>
                        <div className="flex flex-col gap-1">
                          <Label htmlFor="guild-desc">{t('guilds.descLabel')}</Label>
                          <Textarea
                            id="guild-desc"
                            rows={2}
                            value={editDesc}
                            onChange={(e) => setEditDesc(e.target.value)}
                          />
                        </div>
                        <div>
                          <Button type="submit" size="sm" disabled={saveMutation.isPending || editName.trim() === ''}>
                            {saveMutation.isPending ? <Spinner size="sm" color="current" /> : t('guilds.save')}
                          </Button>
                        </div>
                      </form>
                    )
                  : d.description && <p className="text-sm text-muted">{d.description}</p>}

                <div>
                  <SectionLabel>{t('guilds.members')}</SectionLabel>
                  {d.members.length === 0
                    ? <div className="text-xs text-muted py-1">{t('guilds.noMembers')}</div>
                    : (
                        <div className="mt-1">
                          {d.members.map((m) => (
                            <div
                              key={m.player_id}
                              className="flex items-center justify-between py-1.5 border-b border-border/40 text-sm gap-2"
                            >
                              <span className="text-foreground flex-1 truncate">{m.character_name}</span>
                              <Badge tone={m.role_id === ROLE_ADMIN ? 'accent' : 'default'}>{roleLabel(m.role_id)}</Badge>
                              {isSignedIn && m.role_id !== ROLE_ADMIN && (
                                <Button
                                  size="sm"
                                  variant="outline"
                                  disabled={roleMutation.isPending}
                                  onClick={() => promote(m.player_id)}
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
                  {d.invites.length === 0
                    ? <div className="text-xs text-muted py-1">{t('guilds.noInvites')}</div>
                    : (
                        <div className="mt-1">
                          {d.invites.map((iv) => (
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
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
