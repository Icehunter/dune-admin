import * as React from 'react'
import { Chip, SearchField, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BattlepassClaim, Player } from '../../../api/client'
import { DataTable, Icon, SideNav, type Column } from '../../../dune-ui'

type ClaimKey = 'tier_key' | 'status' | 'intel' | 'earned_at' | 'granted_at' | 'last_error'

const claimStatusColor = (status: BattlepassClaim['status']): 'success' | 'warning' | 'default' => {
  if (status === 'granted') return 'success'
  if (status === 'earned') return 'warning'
  return 'default'
}

export const ProgressView: React.FC = () => {
  const { t } = useTranslation()
  const [players, setPlayers] = React.useState<Player[]>([])
  const [playersLoading, setPlayersLoading] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const [selected, setSelected] = React.useState<Player | null>(null)
  const [claims, setClaims] = React.useState<BattlepassClaim[]>([])
  const [claimsLoading, setClaimsLoading] = React.useState(false)

  const loadPlayers = React.useCallback(() => {
    Promise.resolve()
      .then(() => setPlayersLoading(true))
      .then(() => api.players.list())
      .then(setPlayers)
      .catch((e: unknown) => {
        toast.danger(t('battlepass.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setPlayersLoading(false))
  }, [t])

  React.useEffect(() => {
    loadPlayers()
  }, [loadPlayers])

  const loadClaims = React.useCallback(
    (player: Player) => {
      setSelected(player)
      setClaimsLoading(true)
      api.battlepass
        .progress(player.account_id)
        .then((p) => setClaims(p.claims))
        .catch((e: unknown) => {
          toast.danger(t('battlepass.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
        })
        .finally(() => setClaimsLoading(false))
    },
    [t],
  )

  const filtered = React.useMemo(() => {
    const q = search.toLowerCase()
    return q
      ? players.filter((p) => p.name.toLowerCase().includes(q) || String(p.account_id).includes(q))
      : players
  }, [players, search])

  const navItems = React.useMemo(
    () =>
      filtered.map((p) => {
        const dotColor = p.online_status === 'Online'
          ? 'bg-success'
          : p.online_status === 'LoggingOut'
            ? 'bg-warning'
            : 'bg-muted/40'
        return {
          key: String(p.account_id),
          icon: (active: boolean) => (
            <div className="relative w-8 h-8 shrink-0">
              <div className="w-full h-full rounded-4xl overflow-hidden bg-surface-secondary flex items-center justify-center [transform:translateZ(0)]">
                {p.discord_avatar
                  ? <img src={p.discord_avatar} alt={p.name} className="w-full h-full object-cover" />
                  : <Icon name="user" className="size-3.5 text-muted" />}
              </div>
              <span
                className={`absolute bottom-0 right-0 z-[1] size-3 rounded-full border-2 ${dotColor}`}
                style={{ borderColor: active ? 'var(--accent)' : 'var(--surface)' }}
              />
            </div>
          ),
          label: p.name,
          sublabel: `#${p.account_id}`,
        }
      }),
    [filtered],
  )

  const CLAIM_COLUMNS: Column<ClaimKey>[] = [
    { key: 'tier_key', label: t('battlepass.claims.tier'), minWidth: 220 },
    { key: 'status', label: t('battlepass.claims.status'), width: 100 },
    { key: 'intel', label: t('battlepass.columns.intel'), width: 80 },
    { key: 'earned_at', label: t('battlepass.claims.earnedAt'), minWidth: 150 },
    { key: 'granted_at', label: t('battlepass.claims.grantedAt'), minWidth: 150 },
    { key: 'last_error', label: t('battlepass.claims.lastError'), minWidth: 180 },
  ]

  const statusLabel = (status: BattlepassClaim['status']): string => {
    switch (status) {
      case 'baseline': return t('battlepass.status.baseline')
      case 'earned': return t('battlepass.status.earned')
      case 'granted': return t('battlepass.status.granted')
    }
  }

  return (
    <div className="flex h-full min-h-0 gap-3">
      <SideNav
        items={navItems}
        active={selected ? String(selected.account_id) : null}
        onSelect={(id) => {
          const p = players.find((x) => String(x.account_id) === id)
          if (p) loadClaims(p)
        }}
        title={t('battlepass.progress.title')}
        titleAction={playersLoading
          ? <Icon name="loader" className="size-4 text-muted animate-spin" />
          : undefined}
        width="w-72"
      >
        <SearchField
          aria-label={t('battlepass.progress.searchPlaceholder')}
          className="w-full"
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder={t('battlepass.progress.searchPlaceholder')} />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
      </SideNav>

      <div className="flex-1 min-w-0 flex flex-col min-h-0">
        {selected
          ? (
              <>
                <div className="shrink-0 flex items-center gap-2 pb-3">
                  <span className="font-semibold text-accent">{selected.name}</span>
                  <span className="text-muted text-xs font-mono">
                    #
                    {selected.account_id}
                  </span>
                </div>
                <div className="flex-1 min-h-0">
                  <DataTable<BattlepassClaim, ClaimKey>
                    aria-label={t('battlepass.progress.title')}
                    pageSize={50}
                    columns={CLAIM_COLUMNS}
                    rows={claims}
                    loading={claimsLoading}
                    rowId={(c) => c.tier_key}
                    initialSort={{ column: 'earned_at', direction: 'descending' }}
                    sortValue={(c, k) => (c as unknown as Record<string, string | number>)[k] ?? ''}
                    emptyState={(
                      <EmptyState size="sm">
                        <EmptyState.Header>
                          <EmptyState.Title>
                            {t('battlepass.progress.noClaims', { account: selected.name })}
                          </EmptyState.Title>
                        </EmptyState.Header>
                      </EmptyState>
                    )}
                    renderCell={(c, key) => {
                      switch (key) {
                        case 'tier_key':
                          return <span className="font-mono text-xs">{c.tier_key}</span>
                        case 'status':
                          return (
                            <Chip size="sm" variant="soft" color={claimStatusColor(c.status)}>
                              {statusLabel(c.status)}
                            </Chip>
                          )
                        case 'intel':
                          return <span className="font-mono tabular-nums">{c.intel}</span>
                        case 'earned_at':
                          return <span className="text-muted text-xs">{c.earned_at || '—'}</span>
                        case 'granted_at':
                          return <span className="text-muted text-xs">{c.granted_at || '—'}</span>
                        case 'last_error':
                          return <span className="text-muted text-xs">{c.last_error || '—'}</span>
                      }
                    }}
                  />
                </div>
              </>
            )
          : (
              <div className="flex-1 flex items-center justify-center">
                <EmptyState size="sm">
                  <EmptyState.Header>
                    <EmptyState.Title>{t('battlepass.progress.selectPlayer')}</EmptyState.Title>
                    <EmptyState.Description>{t('battlepass.progress.hint')}</EmptyState.Description>
                  </EmptyState.Header>
                </EmptyState>
              </div>
            )}
      </div>
    </div>
  )
}
