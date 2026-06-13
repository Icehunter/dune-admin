import * as React from 'react'
import { Chip, SearchField, Tooltip, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BattlepassClaim, BattlepassTier, Player } from '../../../api/client'
import type { RewardItem } from '../../EventsTab/types'
import { DataTable, Icon, SideNav, type Column } from '../../../dune-ui'

type ClaimKey = 'tier_key' | 'status' | 'intel' | 'earned_at' | 'granted_at' | 'last_error'

const claimStatusColor = (status: BattlepassClaim['status']): 'success' | 'warning' | 'default' => {
  if (status === 'granted') return 'success'
  if (status === 'earned') return 'warning'
  return 'default'
}

const parseItems = (raw: string): RewardItem[] => {
  if (!raw) return []
  try {
    return JSON.parse(raw) as RewardItem[]
  }
  catch {
    return []
  }
}

const tierRequirement = (tier: BattlepassTier, levelLabel: (lvl: number) => string): string => {
  if (tier.signal === 'level') return levelLabel(tier.threshold)
  if (tier.signal === 'journey_node') return tier.signal_key || tier.tier_key
  if (tier.signal === 'player_tag') return tier.signal_key || tier.tier_key
  return tier.tier_key
}

const CATEGORY_LABELS: Record<string, string> = {
  level: 'Level',
  story: 'Story',
  side_quest: 'Side Quest',
  faction: 'Faction',
  exploration: 'Exploration',
  achievement: 'Achievement',
}

const categoryLabel = (cat: string): string => CATEGORY_LABELS[cat] ?? cat

export const ProgressView: React.FC = () => {
  const { t } = useTranslation()
  const [players, setPlayers] = React.useState<Player[]>([])
  const [playersLoading, setPlayersLoading] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const [selected, setSelected] = React.useState<Player | null>(null)
  const [claims, setClaims] = React.useState<BattlepassClaim[]>([])
  const [claimsLoading, setClaimsLoading] = React.useState(false)
  const [tierMap, setTierMap] = React.useState<Map<string, BattlepassTier>>(new Map())

  const loadPlayers = React.useCallback(() => {
    Promise.resolve()
      .then(() => setPlayersLoading(true))
      .then(() => Promise.all([api.players.list(), api.battlepass.tiers()]))
      .then(([pl, tr]) => {
        setPlayers(pl)
        setTierMap(new Map(tr.tiers.map((t) => [t.tier_key, t])))
      })
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
    { key: 'tier_key', label: t('battlepass.claims.tier'), minWidth: 260 },
    { key: 'status', label: t('battlepass.claims.status'), width: 110 },
    { key: 'intel', label: t('battlepass.columns.intel'), width: 100 },
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
                      const tier = tierMap.get(c.tier_key)
                      switch (key) {
                        case 'tier_key': {
                          const cell = (
                            <div className="flex flex-col gap-0.5 py-0.5">
                              <span className="font-medium text-sm leading-tight">
                                {tier?.label ?? c.tier_key}
                              </span>
                              {tier && (
                                <div className="flex items-center gap-1.5">
                                  <Chip size="sm" variant="soft" color="default" className="text-xs h-4 px-1">
                                    {categoryLabel(tier.category)}
                                  </Chip>
                                  <span className="text-xs text-muted">{tierRequirement(tier, (lvl) => t('battlepass.requirementLevel', { level: lvl }))}</span>
                                </div>
                              )}
                              {!tier && (
                                <span className="font-mono text-xs text-muted">{c.tier_key}</span>
                              )}
                            </div>
                          )
                          return tier
                            ? (
                                <Tooltip delay={400}>
                                  <Tooltip.Trigger>{cell}</Tooltip.Trigger>
                                  <Tooltip.Content>
                                    <span className="font-mono text-xs text-muted">{c.tier_key}</span>
                                  </Tooltip.Content>
                                </Tooltip>
                              )
                            : cell
                        }
                        case 'status':
                          return (
                            <Chip size="sm" variant="soft" color={claimStatusColor(c.status)}>
                              {statusLabel(c.status)}
                            </Chip>
                          )
                        case 'intel': {
                          const items = parseItems(tier?.reward_items ?? '')
                          if (items.length === 0) {
                            return <span className="font-mono tabular-nums">{c.intel}</span>
                          }
                          return (
                            <Tooltip delay={300}>
                              <Tooltip.Trigger>
                                <span className="flex items-center gap-1.5 font-mono tabular-nums cursor-default">
                                  {c.intel}
                                  <Chip size="sm" variant="soft" color="default" className="text-xs h-4 px-1">
                                    {`+${items.length} ${t('battlepass.claims.items')}`}
                                  </Chip>
                                </span>
                              </Tooltip.Trigger>
                              <Tooltip.Content>
                                <div className="flex flex-col gap-0.5 text-xs">
                                  {items.map((item, i) => (
                                    <span key={i} className="font-mono">
                                      {item.qty}
                                      ×
                                      {' '}
                                      {item.template}
                                      {item.quality > 0 ? ` (q${item.quality})` : ''}
                                    </span>
                                  ))}
                                </div>
                              </Tooltip.Content>
                            </Tooltip>
                          )
                        }
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
