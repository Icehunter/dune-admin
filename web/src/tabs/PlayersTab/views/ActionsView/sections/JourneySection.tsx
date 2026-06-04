import { useTranslation } from 'react-i18next'
import { Button, Spinner } from '@heroui/react'
import { DataTable, Icon, SectionLabel } from '../../../../../dune-ui'
import { DebouncedSearchField } from '../components/DebouncedSearchField'
import type { Player, JourneyNode } from '../../../../../api/client'

interface JourneySectionProps {
  player: Player
  busy: boolean
  nodes: JourneyNode[]
  nodesLoading: boolean
  nodeSearch: string
  setNodeSearch: (v: string) => void
  filteredNodes: JourneyNode[]
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
  gate: (title: string, description: string, confirmLabel: string, action: () => void) => void
  onRefresh: () => void
  onNodesUpdate: (nodes: JourneyNode[]) => void
}

export function JourneySection({
  player,
  busy,
  nodes,
  nodesLoading,
  setNodeSearch,
  filteredNodes,
  run,
  gate,
  onRefresh,
  onNodesUpdate,
}: JourneySectionProps) {
  const { t } = useTranslation()

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-2 overflow-y-hidden">
      <div className="flex items-center gap-2 shrink-0 min-h-8">
        <SectionLabel>{t('players.actions.journey.title')}</SectionLabel>
        <div className="flex-1" />
        <Button size="sm" variant="ghost" onPress={onRefresh} isDisabled={nodesLoading}>
          {nodesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        </Button>
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy}
          onPress={() =>
            gate(
              t('players.actions.journey.wipeAllTitle'),
              t('players.actions.journey.wipeAllDesc', { player: player.name }),
              t('players.actions.journey.wipeAll'),
              () =>
                run(
                  () => import('../../../../../api/client').then((m) => m.api.players.journeyWipe(player.account_id)),
                  `Wiped all journey nodes for ${player.name}`,
                ).then(() => onNodesUpdate([])),
            )}
        >
          {t('players.actions.journey.wipeAll')}
        </Button>
      </div>
      <DebouncedSearchField className="shrink-0" placeholder={t('players.actions.journey.filterPlaceholder')} onSearch={setNodeSearch} />
      <DataTable<JourneyNode, 'node' | 'done' | 'revealed' | 'reward' | 'actions'>
        aria-label={t('players.actions.journey.journeyLabel')}
        className="min-h-0 max-h-full"
        loading={nodesLoading}
        virtualized
        rowHeight={36}
        columns={[
          { key: 'node', label: t('players.actions.journey.columns.nodeId'), isRowHeader: true, minWidth: 200 },
          { key: 'done', label: t('players.actions.journey.columns.done'), width: 70 },
          { key: 'revealed', label: t('players.actions.journey.columns.revealed'), width: 120 },
          { key: 'reward', label: t('players.actions.journey.columns.reward'), width: 105 },
          { key: 'actions', label: '\u00a0', sortable: false, width: 200 },
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
          <div className="text-center py-8 text-xs text-muted">
            {nodes.length === 0 ? t('players.actions.journey.noNodes') : t('players.actions.journey.noMatching')}
          </div>
        )}
        renderCell={(n, key) => {
          switch (key) {
            case 'node': return <span className="font-mono">{n.node_id}</span>
            case 'done': return n.is_complete ? '✓' : '—'
            case 'revealed': return n.is_revealed ? '✓' : '—'
            case 'reward': return n.has_pending_reward ? '✓' : '—'
            case 'actions':
              return (
                <div className="grid grid-cols-2 gap-1 w-full">
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={busy}
                    className="w-full"
                    onPress={() =>
                      run(
                        () => import('../../../../../api/client').then((m) => m.api.players.journeyComplete(player.account_id, n.node_id)),
                        `Completed ${n.node_id}`,
                      ).then(() => {
                        onNodesUpdate(
                          nodes.map((x) =>
                            x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                              ? { ...x, is_complete: true, is_revealed: true }
                              : x,
                          ),
                        )
                      })}
                  >
                    {n.is_complete ? t('players.actions.journey.redo') : t('players.actions.journey.complete')}
                  </Button>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    isDisabled={busy}
                    className="w-full"
                    onPress={() =>
                      run(
                        () => import('../../../../../api/client').then((m) => m.api.players.journeyReset(player.account_id, n.node_id)),
                        `Reset ${n.node_id}`,
                      ).then(() => {
                        onNodesUpdate(
                          nodes.map((x) =>
                            x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                              ? { ...x, is_complete: false, has_pending_reward: false }
                              : x,
                          ),
                        )
                      })}
                  >
                    {t('players.actions.journey.reset')}
                  </Button>
                </div>
              )
          }
        }}
      />
    </div>
  )
}
