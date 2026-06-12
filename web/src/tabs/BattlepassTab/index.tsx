import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Separator, Switch, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import { EmptyState, Segment } from '@heroui-pro/react'
import { api } from '../../api/client'
import type { BattlepassPendingRow, BattlepassTier, BattlepassTierCounts } from '../../api/client'
import { ActionBar, ConfirmDialog, DataTable, FieldSelect, Icon, NumberInput, PageHeader, Panel, SectionLabel, type Column } from '../../dune-ui'
import { TierEditorModal } from './modals/TierEditorModal'
import { TrackView } from './TrackView'
import { ProgressView } from './views/ProgressView'

type Section = 'pending' | 'progress' | 'catalog' | 'track'
type TierKey = 'label' | 'category' | 'requirement' | 'intel' | 'rewards' | 'earned' | 'granted' | 'enabled' | 'actions'
type PendingKey = 'account_id' | 'name' | 'online' | 'pending_intel' | 'actions'

const CATEGORY_ALL = 'all'
const CATEGORY_ORDER = ['level', 'story', 'side_quest', 'faction', 'exploration', 'achievement']

const categoryColor = (cat: string): 'accent' | 'warning' | 'success' | 'default' => {
  switch (cat) {
    case 'level': return 'accent'
    case 'story': return 'warning'
    case 'side_quest':
    case 'faction': return 'success'
    default: return 'default'
  }
}

export const BattlepassTab: React.FC = () => {
  const { t } = useTranslation()
  const [section, setSection] = React.useState<Section>('pending')
  const [tiers, setTiers] = React.useState<BattlepassTier[]>([])
  const [counts, setCounts] = React.useState<Record<string, BattlepassTierCounts>>({})
  const [playerCount, setPlayerCount] = React.useState(0)
  const [loading, setLoading] = React.useState(false)
  const [category, setCategory] = React.useState<string>(CATEGORY_ALL)
  const [pending, setPending] = React.useState<BattlepassPendingRow[]>([])
  const [pendingLoading, setPendingLoading] = React.useState(false)
  const [granting, setGranting] = React.useState<number | null>(null)
  const [reseedOpen, setReseedOpen] = React.useState(false)
  const [grantTarget, setGrantTarget] = React.useState<BattlepassPendingRow | null>(null)
  const [editorTier, setEditorTier] = React.useState<BattlepassTier | null>(null)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())
  const [bulkDeleteOpen, setBulkDeleteOpen] = React.useState(false)

  const loadTiers = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.battlepass.tiers())
      .then((resp) => {
        setTiers(resp.tiers)
        setCounts(resp.counts)
        setPlayerCount(resp.player_count)
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setLoading(false))
  }, [t])

  const loadPending = React.useCallback(() => {
    Promise.resolve()
      .then(() => setPendingLoading(true))
      .then(() => api.battlepass.pending())
      .then(setPending)
      .catch((e: unknown) => {
        toast.danger(t('battlepass.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setPendingLoading(false))
  }, [t])

  React.useEffect(() => {
    loadTiers()
    loadPending()
  }, [loadTiers, loadPending])

  const refresh = () => {
    loadTiers()
    loadPending()
  }

  const handleGrant = (row: BattlepassPendingRow) => {
    setGrantTarget(null)
    setGranting(row.account_id)
    api.battlepass
      .grant(row.account_id)
      .then((r) => {
        toast.success(t('battlepass.grantSuccess', { intel: r.granted_intel, tiers: r.tiers }))
        loadPending()
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.grantFailed', { message: e instanceof Error ? e.message : String(e) }))
        loadPending()
      })
      .finally(() => setGranting(null))
  }

  const saveTier = (tier: BattlepassTier, intel: number, enabled: boolean) => {
    api.battlepass
      .updateTier(tier.id, { intel, enabled })
      .then(loadTiers)
      .catch((e: unknown) => {
        toast.danger(t('battlepass.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const handleReseed = () => {
    setReseedOpen(false)
    api.battlepass
      .reseed()
      .then((r) => {
        toast.success(t('battlepass.reseedSuccess', { count: r.seeded }))
        loadTiers()
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const visibleTiers = category === CATEGORY_ALL ? tiers : tiers.filter((x) => x.category === category)
  const totalIntel = tiers.reduce((sum, x) => (x.enabled ? sum + x.intel : sum), 0)
  const selectionCount = selectedKeys === 'all' ? visibleTiers.length : (selectedKeys as Set<string>).size

  const selectedIds = (): number[] => {
    if (selectedKeys === 'all') return visibleTiers.map((x) => x.id)
    const keys = selectedKeys as Set<string>
    return visibleTiers.filter((x) => keys.has(x.tier_key)).map((x) => x.id)
  }

  const handleBulk = (action: 'enable' | 'disable' | 'delete') => {
    const ids = selectedIds()
    setBulkDeleteOpen(false)
    setSelectedKeys(new Set())
    api.battlepass
      .tiersBulk(ids, action)
      .then(() => {
        toast.success(t('battlepass.bulkSuccess', { count: ids.length }))
        loadTiers()
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
        loadTiers()
      })
  }

  const categoryLabel = (cat: string): string => {
    switch (cat) {
      case 'level': return t('battlepass.categories.level')
      case 'story': return t('battlepass.categories.story')
      case 'side_quest': return t('battlepass.categories.side_quest')
      case 'faction': return t('battlepass.categories.faction')
      case 'exploration': return t('battlepass.categories.exploration')
      case 'achievement': return t('battlepass.categories.achievement')
      default: return cat
    }
  }

  const requirementText = (tier: BattlepassTier): string =>
    tier.signal === 'level' ? t('battlepass.requirementLevel', { level: tier.threshold }) : tier.signal_key

  const rewardItemCount = (tier: BattlepassTier): number => {
    if (!tier.reward_items) return 0
    try {
      return (JSON.parse(tier.reward_items) as unknown[]).length
    }
    catch {
      return 0
    }
  }

  const TIER_COLUMNS: Column<TierKey>[] = [
    { key: 'label', label: t('battlepass.columns.label'), minWidth: 200 },
    { key: 'category', label: t('battlepass.columns.category'), width: 120 },
    { key: 'requirement', label: t('battlepass.columns.requirement'), minWidth: 200 },
    { key: 'intel', label: t('battlepass.columns.intel'), width: 130, sortable: false },
    { key: 'rewards', label: t('battlepass.columns.rewards'), width: 90 },
    { key: 'earned', label: t('battlepass.columns.earned'), width: 90 },
    { key: 'granted', label: t('battlepass.columns.granted'), width: 90 },
    { key: 'enabled', label: t('battlepass.columns.enabled'), width: 90, sortable: false },
    { key: 'actions', label: '', width: 60, sortable: false },
  ]

  const PENDING_COLUMNS: Column<PendingKey>[] = [
    { key: 'account_id', label: t('battlepass.pending.accountId'), width: 110 },
    { key: 'name', label: t('battlepass.pending.name'), minWidth: 160 },
    { key: 'online', label: t('battlepass.pending.online'), width: 90 },
    { key: 'pending_intel', label: t('battlepass.pending.intel'), width: 110 },
    { key: 'actions', label: '', width: 150, sortable: false },
  ]

  return (
    <>
      <div className="flex flex-col h-full gap-3 min-h-0">
        <PageHeader
          title={t('battlepass.title', { count: tiers.length })}
          subtitle={t('battlepass.subtitle', { intel: totalIntel })}
        >
          <Segment
            selectedKey={section}
            onSelectionChange={(k) => setSection(k as Section)}
            size="sm"
            aria-label={t('battlepass.title', { count: tiers.length })}
          >
            <Segment.Item id="pending">
              <Segment.Separator />
              {t('battlepass.sections.pending', { count: pending.length })}
            </Segment.Item>
            <Segment.Item id="progress">
              <Segment.Separator />
              {t('battlepass.sections.progress')}
            </Segment.Item>
            <Segment.Item id="catalog">
              <Segment.Separator />
              {t('battlepass.sections.catalog')}
            </Segment.Item>
            <Segment.Item id="track">
              <Segment.Separator />
              {t('battlepass.sections.track')}
            </Segment.Item>
          </Segment>
          <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading || pendingLoading}>
            <Icon name="refresh-cw" />
            {' '}
            {t('common.refresh')}
          </Button>
        </PageHeader>

        {section === 'pending' && (
          <div className="flex flex-col min-h-0 flex-1">
            <DataTable<BattlepassPendingRow, PendingKey>
              aria-label={t('battlepass.sections.pending', { count: pending.length })}
              pageSize={50}
              rowHeight={48}
              columns={PENDING_COLUMNS}
              rows={pending}
              loading={pendingLoading}
              rowId={(p) => String(p.account_id)}
              initialSort={{ column: 'pending_intel', direction: 'descending' }}
              sortValue={(p, k) => {
                if (k === 'online') return p.online ? 1 : 0
                if (k === 'actions') return ''
                return (p as unknown as Record<string, string | number>)[k] ?? ''
              }}
              emptyState={(
                <EmptyState size="sm">
                  <EmptyState.Header>
                    <EmptyState.Title>{t('battlepass.pending.none')}</EmptyState.Title>
                  </EmptyState.Header>
                </EmptyState>
              )}
              renderCell={(p, key) => {
                switch (key) {
                  case 'account_id':
                    return <span className="font-mono text-xs">{p.account_id}</span>
                  case 'name':
                    return p.name || <span className="text-muted">—</span>
                  case 'online':
                    return (
                      <Chip size="sm" variant="soft" color={p.online ? 'success' : 'default'}>
                        {p.online ? t('battlepass.pending.onlineState') : t('battlepass.pending.offlineState')}
                      </Chip>
                    )
                  case 'pending_intel':
                    return <span className="font-mono tabular-nums">{p.pending_intel}</span>
                  case 'actions':
                    return (
                      <Button
                        size="sm"
                        variant="primary"
                        onPress={() => setGrantTarget(p)}
                        isDisabled={p.online || granting === p.account_id}
                      >
                        <Icon name="gift" />
                        {' '}
                        {p.online ? t('battlepass.pending.grantOnlineHint') : t('battlepass.pending.grant')}
                      </Button>
                    )
                }
              }}
            />
          </div>
        )}

        {section === 'progress' && <ProgressView />}

        {section === 'catalog' && (
          <div className="flex flex-col min-h-0 flex-1">
            <div className="flex items-center gap-2 mb-3">
              <SectionLabel>{t('battlepass.catalog.title', { count: visibleTiers.length })}</SectionLabel>
              <FieldSelect
                value={category}
                onChange={(v) => {
                  setCategory(v)
                  setSelectedKeys(new Set())
                }}
                options={[CATEGORY_ALL, ...CATEGORY_ORDER]}
                className="w-44"
              />
              <Button size="sm" variant="danger" className="ml-auto" onPress={() => setReseedOpen(true)}>
                <Icon name="rotate-ccw" />
                {' '}
                {t('battlepass.reseed')}
              </Button>
            </div>
            <div className="flex-1 min-h-0">
              <DataTable<BattlepassTier, TierKey>
                selectionMode="multiple"
                selectedKeys={selectedKeys}
                onSelectionChange={setSelectedKeys}
                aria-label={t('battlepass.catalog.title', { count: visibleTiers.length })}
                pageSize={50}
                rowHeight={48}
                columns={TIER_COLUMNS}
                rows={visibleTiers}
                loading={loading}
                rowId={(x) => x.tier_key}
                initialSort={{ column: 'category', direction: 'ascending' }}
                sortValue={(x, k) => {
                  switch (k) {
                    case 'requirement': return requirementText(x)
                    case 'rewards': return rewardItemCount(x)
                    case 'earned': return counts[x.tier_key]?.earned ?? 0
                    case 'granted': return counts[x.tier_key]?.granted ?? 0
                    case 'enabled': return x.enabled ? 1 : 0
                    case 'intel': return x.intel
                    case 'actions': return ''
                    default: return (x as unknown as Record<string, string | number>)[k] ?? ''
                  }
                }}
                emptyState={(
                  <EmptyState size="sm">
                    <EmptyState.Header>
                      <EmptyState.Title>{t('battlepass.catalog.empty')}</EmptyState.Title>
                    </EmptyState.Header>
                  </EmptyState>
                )}
                renderCell={(tier, key) => {
                  switch (key) {
                    case 'label':
                      return tier.label
                    case 'category':
                      return (
                        <Chip size="sm" variant="soft" color={categoryColor(tier.category)}>
                          {categoryLabel(tier.category)}
                        </Chip>
                      )
                    case 'requirement':
                      return <span className="font-mono text-xs text-muted">{requirementText(tier)}</span>
                    case 'intel':
                      return (
                        <NumberInput
                          ariaLabel={t('battlepass.columns.intel')}
                          min={0}
                          value={tier.intel}
                          onChange={(v) => {
                            if (v !== tier.intel) saveTier(tier, v, tier.enabled)
                          }}
                          showButtons={false}
                          className="w-24"
                        />
                      )
                    case 'rewards': {
                      const n = rewardItemCount(tier)
                      return n > 0
                        ? (
                            <Chip size="sm" variant="soft" color="accent">
                              <Icon name="package" className="size-3" />
                              {' '}
                              {n}
                            </Chip>
                          )
                        : <span className="text-muted">—</span>
                    }
                    case 'earned':
                      return <span className="text-muted tabular-nums">{counts[tier.tier_key]?.earned ?? 0}</span>
                    case 'granted':
                      return <span className="text-muted tabular-nums">{counts[tier.tier_key]?.granted ?? 0}</span>
                    case 'enabled':
                      return (
                        <Switch
                          size="sm"
                          isSelected={tier.enabled}
                          onChange={() => saveTier(tier, tier.intel, !tier.enabled)}
                          aria-label={t('battlepass.toggleEnabled')}
                        >
                          <Switch.Control><Switch.Thumb /></Switch.Control>
                        </Switch>
                      )
                    case 'actions':
                      return (
                        <Button
                          size="sm"
                          variant="ghost"
                          isIconOnly
                          onPress={() => setEditorTier(tier)}
                          aria-label={t('common.edit') as string}
                        >
                          <Icon name="pencil" />
                        </Button>
                      )
                  }
                }}
              />
            </div>
          </div>
        )}

        {section === 'track' && (
          <Panel className="flex flex-col min-h-0 flex-1 overflow-y-auto">
            <TrackView tiers={tiers} counts={counts} playerCount={playerCount} categoryLabel={categoryLabel} />
          </Panel>
        )}

        <ConfirmDialog
          open={reseedOpen}
          title={t('battlepass.reseedDialog.title')}
          confirmLabel={t('battlepass.reseed')}
          onConfirm={handleReseed}
          onCancel={() => setReseedOpen(false)}
          description={(
            <div className="flex flex-col gap-2">
              <p>{t('battlepass.reseedDialog.intro', { count: 158 })}</p>
              <ul className="list-disc pl-5 space-y-1">
                <li>{t('battlepass.reseedDialog.resetsIntel')}</li>
                <li>{t('battlepass.reseedDialog.resetsEnabled')}</li>
                <li>{t('battlepass.reseedDialog.removesCustom')}</li>
              </ul>
              <p>{t('battlepass.reseedDialog.keeps')}</p>
            </div>
          )}
        />

        <ConfirmDialog
          open={grantTarget != null}
          title={t('battlepass.grantDialog.title')}
          confirmLabel={t('battlepass.grantDialog.confirm', { intel: grantTarget?.pending_intel ?? 0 })}
          onConfirm={() => grantTarget && handleGrant(grantTarget)}
          onCancel={() => setGrantTarget(null)}
          description={t('battlepass.grantDialog.body', {
            intel: grantTarget?.pending_intel ?? 0,
            account: grantTarget?.name || grantTarget?.account_id || '',
          })}
        />

        <ConfirmDialog
          open={bulkDeleteOpen}
          title={t('battlepass.bulkDeleteDialog.title')}
          confirmLabel={t('battlepass.bulkDeleteDialog.confirm', { count: selectionCount })}
          onConfirm={() => handleBulk('delete')}
          onCancel={() => setBulkDeleteOpen(false)}
          description={t('battlepass.bulkDeleteDialog.body', { count: selectionCount })}
        />

        <TierEditorModal
          isOpen={editorTier != null}
          onClose={() => setEditorTier(null)}
          tier={editorTier}
          onSaved={loadTiers}
        />
      </div>

      <ActionBar aria-label={t('battlepass.catalog.title', { count: visibleTiers.length })} isOpen={section === 'catalog' && selectionCount > 0}>
        <ActionBar.Prefix>
          <Chip size="sm" className="shrink-0 tabular-nums">{selectionCount}</Chip>
        </ActionBar.Prefix>
        <Separator />
        <ActionBar.Content>
          <Button size="sm" variant="ghost" onPress={() => handleBulk('enable')}>
            <Icon name="circle-check" />
            <span className="action-bar__label">{t('battlepass.bulk.enable')}</span>
          </Button>
          <Button size="sm" variant="ghost" onPress={() => handleBulk('disable')}>
            <Icon name="circle-off" />
            <span className="action-bar__label">{t('battlepass.bulk.disable')}</span>
          </Button>
          <Button size="sm" variant="ghost" className="text-danger" onPress={() => setBulkDeleteOpen(true)}>
            <Icon name="trash-2" />
            <span className="action-bar__label">{t('battlepass.bulk.delete')}</span>
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
