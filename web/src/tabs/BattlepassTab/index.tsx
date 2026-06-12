import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Separator, Switch, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import { EmptyState, Segment } from '@heroui-pro/react'
import { api } from '../../api/client'
import type { BattlepassCatalogExport, BattlepassPendingRow, BattlepassTier, BattlepassTierCounts } from '../../api/client'
import { ActionBar, ConfirmDialog, DataTable, FieldSelect, Icon, NumberInput, PageHeader, Panel, SectionLabel, type Column } from '../../dune-ui'
import { TierEditorModal } from './modals/TierEditorModal'
import { RewardIcon } from './RewardIcons'
import { TrackView } from './TrackView'
import { ConfigView } from './views/ConfigView'
import { ProgressView } from './views/ProgressView'

type Section = 'pending' | 'progress' | 'catalog' | 'track' | 'config'
type TierKey = 'label' | 'category' | 'requirement' | 'intel' | 'rewards' | 'earned' | 'granted' | 'enabled' | 'actions'
type PendingKey = 'name' | 'tier_label' | 'intel' | 'items' | 'actions'

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
  const [granting, setGranting] = React.useState<string | null>(null)
  const [defaultCount, setDefaultCount] = React.useState(0)
  const importInputRef = React.useRef<HTMLInputElement>(null)
  const [reseedOpen, setReseedOpen] = React.useState(false)
  const [createOpen, setCreateOpen] = React.useState(false)
  const [importOpen, setImportOpen] = React.useState(false)
  const [pendingImport, setPendingImport] = React.useState<BattlepassCatalogExport | null>(null)
  const [grantTarget, setGrantTarget] = React.useState<BattlepassPendingRow | null>(null)
  const [editorTier, setEditorTier] = React.useState<BattlepassTier | null>(null)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())
  const [bulkDeleteOpen, setBulkDeleteOpen] = React.useState(false)
  const [pendingSelectedKeys, setPendingSelectedKeys] = React.useState<Selection>(new Set())
  const [bulkGrantOpen, setBulkGrantOpen] = React.useState(false)

  const loadTiers = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.battlepass.tiers())
      .then((resp) => {
        setTiers(resp.tiers)
        setCounts(resp.counts)
        setPlayerCount(resp.player_count)
        setDefaultCount(resp.default_count)
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
    const grantKey = `${row.account_id}:${row.tier_key}`
    setGranting(grantKey)
    api.battlepass
      .grantTier(row.account_id, row.tier_key)
      .then((r) => {
        toast.success(t('battlepass.grantTierSuccess', { intel: r.granted_intel, tier: row.tier_label }))
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

  const handleExport = () => {
    api.battlepass
      .exportCatalog()
      .then((data) => {
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = 'battlepass-catalog.json'
        a.click()
        URL.revokeObjectURL(url)
        toast.success(t('battlepass.exportSuccess'))
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.importFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const handleImportFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const parsed = JSON.parse(ev.target?.result as string) as BattlepassCatalogExport
        setPendingImport(parsed)
        setImportOpen(true)
      }
      catch {
        toast.danger(t('battlepass.importFailed', { message: 'Invalid JSON' }))
      }
    }
    reader.readAsText(file)
  }

  const handleImportConfirm = () => {
    if (!pendingImport) return
    setImportOpen(false)
    api.battlepass
      .importCatalog(pendingImport)
      .then((r) => {
        toast.success(t('battlepass.importSuccess', { count: r.imported }))
        setPendingImport(null)
        loadTiers()
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.importFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const visibleTiers = category === CATEGORY_ALL ? tiers : tiers.filter((x) => x.category === category)
  const totalIntel = tiers.reduce((sum, x) => (x.enabled ? sum + x.intel : sum), 0)
  const selectionCount = selectedKeys === 'all' ? visibleTiers.length : (selectedKeys as Set<string>).size
  const pendingSelectionCount = pendingSelectedKeys === 'all' ? pending.length : (pendingSelectedKeys as Set<string>).size

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

  const handleBulkGrant = () => {
    setBulkGrantOpen(false)
    const keys = pendingSelectedKeys === 'all'
      ? pending.map((p) => `${p.account_id}:${p.tier_key}`)
      : [...(pendingSelectedKeys as Set<string>)]
    const eligible = pending.filter((p) => !p.online && keys.includes(`${p.account_id}:${p.tier_key}`))
    const skipped = keys.length - eligible.length
    setPendingSelectedKeys(new Set())
    const grant = (row: BattlepassPendingRow) =>
      api.battlepass.grantTier(row.account_id, row.tier_key).then(() => 1 as const).catch(() => 0 as const)
    Promise.all(eligible.map(grant)).then((results) => {
      const granted = results.reduce((a, b) => a + b, 0 as number)
      toast.success(t('battlepass.pending.bulkGrantSuccess', { granted, skipped }))
      loadPending()
    }).catch(() => { loadPending() })
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
    { key: 'name', label: t('battlepass.pending.name'), minWidth: 160 },
    { key: 'tier_label', label: t('battlepass.pending.tier'), minWidth: 180 },
    { key: 'intel', label: t('battlepass.pending.intel'), width: 90 },
    { key: 'items', label: t('battlepass.pending.items'), width: 60, sortable: false },
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
            <Segment.Item id="config">
              <Segment.Separator />
              {t('battlepass.sections.config')}
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
              selectionMode="multiple"
              selectedKeys={pendingSelectedKeys}
              onSelectionChange={setPendingSelectedKeys}
              aria-label={t('battlepass.sections.pending', { count: pending.length })}
              pageSize={50}
              rowHeight={48}
              columns={PENDING_COLUMNS}
              rows={pending}
              loading={pendingLoading}
              rowId={(p) => `${p.account_id}:${p.tier_key}`}
              initialSort={{ column: 'intel', direction: 'descending' }}
              sortValue={(p, k) => {
                if (k === 'actions' || k === 'items') return ''
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
                const grantKey = `${p.account_id}:${p.tier_key}`
                switch (key) {
                  case 'name':
                    return (
                      <span className="flex items-center gap-1.5">
                        <span
                          className={`inline-block w-1.5 h-1.5 rounded-full flex-shrink-0 ${p.online ? 'bg-success' : 'bg-border'}`}
                          title={p.online ? t('battlepass.pending.onlineState') : t('battlepass.pending.offlineState')}
                        />
                        {p.name || <span className="text-muted">—</span>}
                      </span>
                    )
                  case 'tier_label':
                    return <span>{p.tier_label}</span>
                  case 'intel':
                    return <span className="font-mono tabular-nums">{p.intel}</span>
                  case 'items':
                    return p.reward_items
                      ? (
                          <RewardIcon
                            tier={{ reward_items: p.reward_items, category: '' } as Parameters<typeof RewardIcon>[0]['tier']}
                            className="w-4 h-4 text-muted"
                          />
                        )
                      : <span className="text-muted">—</span>
                  case 'actions':
                    return (
                      <Button
                        size="sm"
                        variant="primary"
                        onPress={() => setGrantTarget(p)}
                        isDisabled={p.online || granting === grantKey}
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
              <Button size="sm" variant="ghost" onPress={() => setCreateOpen(true)}>
                <Icon name="plus" />
                {' '}
                {t('battlepass.catalog.newTier')}
              </Button>
              <Button size="sm" variant="ghost" onPress={handleExport}>
                <Icon name="download" />
                {' '}
                {t('battlepass.catalog.export')}
              </Button>
              <Button size="sm" variant="ghost" onPress={() => importInputRef.current?.click()}>
                <Icon name="upload" />
                {' '}
                {t('battlepass.catalog.import')}
              </Button>
              <input
                ref={importInputRef}
                type="file"
                accept=".json"
                className="hidden"
                onChange={handleImportFile}
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

        {section === 'config' && (
          <Panel className="flex flex-col min-h-0 flex-1">
            <ConfigView />
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
              <p>{t('battlepass.reseedDialog.intro', { count: defaultCount })}</p>
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
          confirmLabel={t('battlepass.grantDialog.confirm', { intel: grantTarget?.intel ?? 0 })}
          onConfirm={() => grantTarget && handleGrant(grantTarget)}
          onCancel={() => setGrantTarget(null)}
          description={t('battlepass.grantDialog.body', {
            intel: grantTarget?.intel ?? 0,
            account: grantTarget?.name || grantTarget?.account_id || '',
            tier: grantTarget?.tier_label ?? '',
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

        <ConfirmDialog
          open={bulkGrantOpen}
          title={t('battlepass.pending.bulkGrantDialog.title')}
          confirmLabel={t('battlepass.pending.bulkGrantDialog.confirm', { count: pendingSelectionCount })}
          onConfirm={handleBulkGrant}
          onCancel={() => setBulkGrantOpen(false)}
          description={t('battlepass.pending.bulkGrantDialog.body', { count: pendingSelectionCount })}
        />

        <ConfirmDialog
          open={importOpen}
          title={t('battlepass.importDialog.title')}
          confirmLabel={t('battlepass.importDialog.confirm')}
          onConfirm={handleImportConfirm}
          onCancel={() => {
            setImportOpen(false)
            setPendingImport(null)
          }}
          description={t('battlepass.importDialog.body')}
        />

        {/* Create mode: tier=null triggers create flow in modal */}
        <TierEditorModal
          isOpen={createOpen}
          onClose={() => setCreateOpen(false)}
          tier={null}
          onSaved={() => {
            setCreateOpen(false)
            loadTiers()
          }}
        />

        {/* Edit mode */}
        <TierEditorModal
          isOpen={editorTier != null}
          onClose={() => setEditorTier(null)}
          tier={editorTier}
          onSaved={loadTiers}
        />
      </div>

      <ActionBar aria-label={t('battlepass.pending.title', { count: pending.length })} isOpen={section === 'pending' && pendingSelectionCount > 0}>
        <ActionBar.Prefix>
          <Chip size="sm" className="shrink-0 tabular-nums">{pendingSelectionCount}</Chip>
        </ActionBar.Prefix>
        <Separator />
        <ActionBar.Content>
          <Button size="sm" variant="ghost" onPress={() => setBulkGrantOpen(true)}>
            <Icon name="gift" />
            <span className="action-bar__label">{t('battlepass.pending.grantSelected')}</span>
          </Button>
        </ActionBar.Content>
        <Separator />
        <ActionBar.Suffix>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            onPress={() => setPendingSelectedKeys(new Set())}
            aria-label={t('common.clearSelection')}
          >
            <Icon name="x" />
          </Button>
        </ActionBar.Suffix>
      </ActionBar>

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
