import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Chip, Separator, Spinner } from '@heroui/react'
import type { Selection } from '@heroui/react'
import { ActionBar, EmptyState, Segment } from '@heroui-pro/react'
import type { Key } from 'react-aria-components'
import { DataTable, Icon, SectionLabel } from '../../../../../dune-ui'
import { DebouncedSearchField } from '../components/DebouncedSearchField'
import { api } from '../../../../../api/client'
import type { Player, JourneyNode } from '../../../../../api/client'
import { busyAtom, nodesAtom, nodesLoadedAtom } from '../store'
import { useRun, useGate } from '../hooks/useActions'

type FilterTab = 'all' | 'done' | 'revealed' | 'reward'

interface JourneySectionProps { player: Player }

export function JourneySection({ player }: JourneySectionProps) {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const [nodes, setNodes] = useAtom(nodesAtom(player.id))
  const [nodesLoaded, setNodesLoaded] = useAtom(nodesLoadedAtom(player.id))
  const run = useRun(player.id)
  const gate = useGate(player.id)

  const [nodesLoading, setNodesLoading] = useState(false)
  const [nodeSearch, setNodeSearch] = useState('')
  const [filterTab, setFilterTab] = useState<FilterTab>('all')
  const [selectedKeys, setSelectedKeys] = useState<Selection>(new Set())

  useEffect(() => {
    if (nodesLoaded) return
    Promise.resolve()
      .then(() => setNodesLoading(true))
      .then(() => api.players.journey(player.account_id))
      .then((n) => {
        setNodes(n)
        setNodesLoaded(true)
      })
      .catch(() => {})
      .finally(() => setNodesLoading(false))
  }, [nodesLoaded, player.account_id, setNodes, setNodesLoaded])

  useEffect(() => {
    Promise.resolve().then(() => setSelectedKeys(new Set()))
  }, [filterTab, nodeSearch])

  const filteredNodes = useMemo(() => {
    let result = nodes
    if (filterTab === 'done') result = result.filter((n) => n.is_complete)
    else if (filterTab === 'revealed') result = result.filter((n) => n.is_revealed)
    else if (filterTab === 'reward') result = result.filter((n) => n.has_pending_reward)
    if (nodeSearch) {
      const q = nodeSearch.toLowerCase()
      result = result.filter((n) => n.node_id.toLowerCase().includes(q))
    }
    return result
  }, [nodes, filterTab, nodeSearch])

  const selectedCount = selectedKeys === 'all'
    ? filteredNodes.length
    : (selectedKeys as Set<string>).size

  const selectedNodes = useMemo(() => {
    if (selectedKeys === 'all') return filteredNodes
    const keys = selectedKeys as Set<string>
    return filteredNodes.filter((n) => keys.has(n.node_id))
  }, [selectedKeys, filteredNodes])

  const incompleteSelected = useMemo(
    () => selectedNodes.filter((n) => !n.is_complete),
    [selectedNodes],
  )
  const completeSelected = useMemo(
    () => selectedNodes.filter((n) => n.is_complete),
    [selectedNodes],
  )

  const handleWipeAllJourney = () => {
    gate(
      t('players.actions.journey.wipeAllTitle'),
      t('players.actions.journey.wipeAllDesc', { player: player.name }),
      t('players.actions.journey.wipeAll'),
      () => run(
        () => api.players.journeyWipe(player.account_id),
        `Wiped all journey nodes for ${player.name}`,
      ).then(() => setNodes([])),
    )
  }

  const handleCompleteNode = (n: JourneyNode) => {
    run(
      () => api.players.journeyComplete(player.account_id, n.node_id),
      `Completed ${n.node_id}`,
    ).then(() =>
      setNodes((prev) =>
        prev.map((x) =>
          x.node_id === n.node_id || x.node_id.startsWith(`${n.node_id}.`)
            ? { ...x, is_complete: true, is_revealed: true }
            : x,
        ),
      ),
    )
  }

  const handleResetNode = (n: JourneyNode) => {
    run(
      () => api.players.journeyReset(player.account_id, n.node_id),
      `Reset ${n.node_id}`,
    ).then(() =>
      setNodes((prev) =>
        prev.map((x) =>
          x.node_id === n.node_id || x.node_id.startsWith(`${n.node_id}.`)
            ? { ...x, is_complete: false, has_pending_reward: false }
            : x,
        ),
      ),
    )
  }

  const handleBulkComplete = () => {
    incompleteSelected.forEach((n) => handleCompleteNode(n))
    setSelectedKeys(new Set())
  }

  const handleBulkRedo = () => {
    completeSelected.forEach((n) => handleCompleteNode(n))
    setSelectedKeys(new Set())
  }

  const handleBulkReset = () => {
    selectedNodes.forEach((n) => handleResetNode(n))
    setSelectedKeys(new Set())
  }

  return (
    <>
      <div className="h-full flex flex-col gap-2">
        <div className="flex items-center gap-2 shrink-0 min-h-8">
          <SectionLabel>{t('players.actions.journey.title')}</SectionLabel>
          <div className="flex-1" />
          <Button
            size="sm"
            variant="ghost"
            isDisabled={nodesLoading}
            onPress={() => setNodesLoaded(false)}
          >
            {nodesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
          </Button>
          <Button
            size="sm"
            variant="danger-soft"
            isDisabled={busy}
            onPress={handleWipeAllJourney}
          >
            {t('players.actions.journey.wipeAll')}
          </Button>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Segment
            selectedKey={filterTab}
            onSelectionChange={(k: Key) => setFilterTab(k as FilterTab)}
          >
            <Segment.Item id="all">{t('players.actions.journey.filterAll')}</Segment.Item>
            <Segment.Item id="done">{t('players.actions.journey.filterDone')}</Segment.Item>
            <Segment.Item id="revealed">{t('players.actions.journey.filterRevealed')}</Segment.Item>
            <Segment.Item id="reward">{t('players.actions.journey.filterReward')}</Segment.Item>
          </Segment>
          <DebouncedSearchField
            className="flex-1"
            placeholder={t('players.actions.journey.filterPlaceholder')}
            onSearch={setNodeSearch}
          />
        </div>
        <div className="flex-1 min-h-0">
          <DataTable<JourneyNode, 'node' | 'done' | 'revealed' | 'reward' | 'actions'>
            aria-label={t('players.actions.journey.journeyLabel')}
            contentClassName="min-w-[755px]"
            loading={nodesLoading}
            pageSize={50}
            virtualized
            rowHeight={48}
            selectionMode="multiple"
            selectedKeys={selectedKeys}
            onSelectionChange={setSelectedKeys}
            columns={[
              {
                key: 'node',
                label: t('players.actions.journey.columns.nodeId'),
                isRowHeader: true,
                width: 300,
              },
              { key: 'done', label: t('players.actions.journey.columns.done'), width: 70, align: 'center' },
              { key: 'revealed', label: t('players.actions.journey.columns.revealed'), width: 120, align: 'center' },
              { key: 'reward', label: t('players.actions.journey.columns.reward'), width: 105, align: 'center' },
              { key: 'actions', label: ' ', sortable: false, width: 200 },
            ]}
            rows={filteredNodes}
            rowId={(n) => n.node_id}
            initialSort={{ column: 'node', direction: 'ascending' }}
            sortValue={(n, k) => {
              if (k === 'node') return n.node_id
              if (k === 'done') return n.is_complete ? 1 : 0
              if (k === 'revealed') return n.is_revealed ? 1 : 0
              if (k === 'reward') return n.has_pending_reward ? 1 : 0
              return ''
            }}
            emptyState={(
              <EmptyState size="sm">
                <EmptyState.Header>
                  <EmptyState.Title>
                    {nodes.length === 0
                      ? t('players.actions.journey.noNodes')
                      : t('players.actions.journey.noMatching')}
                  </EmptyState.Title>
                </EmptyState.Header>
              </EmptyState>
            )}
            renderCell={(n, key) => {
              switch (key) {
                case 'node':
                  return (
                    <span className="font-mono text-xs truncate block max-w-full" title={n.node_id}>
                      {n.node_id}
                    </span>
                  )
                case 'done':
                  return n.is_complete ? '✓' : '—'
                case 'revealed':
                  return n.is_revealed ? '✓' : '—'
                case 'reward':
                  return n.has_pending_reward ? '✓' : '—'
                case 'actions':
                  return (
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        isDisabled={busy}
                        onPress={() => handleCompleteNode(n)}
                      >
                        {n.is_complete
                          ? t('players.actions.journey.redo')
                          : t('players.actions.journey.complete')}
                      </Button>
                      <Button
                        size="sm"
                        variant="danger-soft"
                        isDisabled={busy}
                        onPress={() => handleResetNode(n)}
                      >
                        {t('players.actions.journey.reset')}
                      </Button>
                    </div>
                  )
              }
            }}
          />
        </div>
      </div>

      <ActionBar aria-label={t('players.actions.journey.bulkActionsLabel')} isOpen={selectedCount > 0}>
        <ActionBar.Prefix>
          <Chip size="sm" className="tabular-nums shrink-0">{selectedCount}</Chip>
        </ActionBar.Prefix>
        <Separator />
        <ActionBar.Content>
          {incompleteSelected.length > 0 && (
            <Button
              size="sm"
              variant="ghost"
              isDisabled={busy}
              onPress={handleBulkComplete}
            >
              <Icon name="check" />
              {`${t('players.actions.journey.bulkComplete')} (${incompleteSelected.length})`}
            </Button>
          )}
          {completeSelected.length > 0 && (
            <Button
              size="sm"
              variant="ghost"
              isDisabled={busy}
              onPress={handleBulkRedo}
            >
              <Icon name="rotate-ccw" />
              {`${t('players.actions.journey.bulkRedo')} (${completeSelected.length})`}
            </Button>
          )}
          <Button
            className="text-danger bg-danger/10"
            size="sm"
            variant="ghost"
            isDisabled={busy}
            onPress={handleBulkReset}
          >
            <Icon name="trash-2" />
            {t('players.actions.journey.bulkReset')}
          </Button>
        </ActionBar.Content>
        <Separator />
        <ActionBar.Suffix>
          <Button
            isIconOnly
            aria-label={t('players.actions.journey.clearSelection')}
            size="sm"
            variant="ghost"
            onPress={() => setSelectedKeys(new Set())}
          >
            <Icon name="trash" />
          </Button>
        </ActionBar.Suffix>
      </ActionBar>
    </>
  )
}
