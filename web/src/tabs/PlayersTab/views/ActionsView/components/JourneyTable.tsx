import { useState, useRef, useCallback, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner } from '@heroui/react'
import { Icon } from '../../../../../dune-ui'
import type { JourneyNode } from '../../../../../api/client'

const ROW_HEIGHT = 48
const BUFFER = 8

type SortDir = 'asc' | 'desc'
type SortCol = 'node' | 'done' | 'revealed' | 'reward'

interface JourneyTableProps {
  nodes: JourneyNode[]
  busy: boolean
  loading?: boolean
  onComplete: (n: JourneyNode) => void
  onReset: (n: JourneyNode) => void
}

interface ColHeadProps {
  col: SortCol
  label: string
  sortCol: SortCol
  sortDir: SortDir
  style?: React.CSSProperties
  className?: string
  onSort: (col: SortCol) => void
}

function ColHead({ col, label, sortCol, sortDir, style, className, onSort }: ColHeadProps) {
  const active = sortCol === col
  return (
    <th
      style={style}
      className={`px-4 py-2.5 text-left text-xs font-medium text-muted cursor-pointer select-none whitespace-nowrap relative ${className ?? ''}`}
      onClick={() => onSort(col)}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active
          ? <Icon name={sortDir === 'asc' ? 'chevron-up' : 'chevron-down'} className="size-3 inline-flex shrink-0" />
          : <Icon name="chevrons-up-down" className="size-3 inline-flex shrink-0 opacity-30" />}
      </span>
    </th>
  )
}

function sortNodes(nodes: JourneyNode[], col: SortCol, dir: SortDir): JourneyNode[] {
  return [...nodes].sort((a, b) => {
    let av: string | number
    let bv: string | number
    if (col === 'node') {
      av = a.node_id
      bv = b.node_id
    }
    else if (col === 'done') {
      av = a.is_complete ? 1 : 0
      bv = b.is_complete ? 1 : 0
    }
    else if (col === 'revealed') {
      av = a.is_revealed ? 1 : 0
      bv = b.is_revealed ? 1 : 0
    }
    else {
      av = a.has_pending_reward ? 1 : 0
      bv = b.has_pending_reward ? 1 : 0
    }
    if (av < bv) return dir === 'asc' ? -1 : 1
    if (av > bv) return dir === 'asc' ? 1 : -1
    return 0
  })
}

export function JourneyTable({ nodes, busy, loading = false, onComplete, onReset }: JourneyTableProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [height, setHeight] = useState(400)
  const [sortCol, setSortCol] = useState<SortCol>('node')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setHeight(el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const sorted = useMemo(() => sortNodes(nodes, sortCol, sortDir), [nodes, sortCol, sortDir])

  const handleSort = useCallback((col: SortCol) => {
    setSortCol((prev) => {
      if (prev === col) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return prev
      }
      setSortDir('asc')
      return col
    })
  }, [])

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop)
  }, [])

  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - BUFFER)
  const end = Math.min(sorted.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + BUFFER)
  const paddingTop = start * ROW_HEIGHT
  const paddingBottom = (sorted.length - end) * ROW_HEIGHT
  const visible = sorted.slice(start, end)

  const nodeLabel = t('players.actions.journey.columns.nodeId')
  const doneLabel = t('players.actions.journey.columns.done')
  const revealedLabel = t('players.actions.journey.columns.revealed')
  const rewardLabel = t('players.actions.journey.columns.reward')

  return (
    <div
      ref={containerRef}
      className="flex-1 min-h-0 overflow-y-auto rounded-lg border border-border/50 bg-surface-secondary"
      onScroll={handleScroll}
    >
      <table className="w-full text-sm border-separate border-spacing-0">
        <thead className="sticky top-0 z-10 border-b border-border/50 bg-surface-secondary">
          <tr>
            <ColHead col="node" label={nodeLabel} sortCol={sortCol} sortDir={sortDir} onSort={handleSort} />
            <ColHead col="done" label={doneLabel} sortCol={sortCol} sortDir={sortDir} onSort={handleSort} style={{ width: 70 }} className="text-center" />
            <ColHead col="revealed" label={revealedLabel} sortCol={sortCol} sortDir={sortDir} onSort={handleSort} style={{ width: 120 }} className="text-center" />
            <ColHead col="reward" label={rewardLabel} sortCol={sortCol} sortDir={sortDir} onSort={handleSort} style={{ width: 105 }} className="text-center" />
            <th style={{ width: 200 }} className="px-4 py-2.5" />
          </tr>
        </thead>
        <tbody>
          {loading && (
            <tr>
              <td colSpan={5} className="py-12 text-center">
                <Spinner size="sm" />
              </td>
            </tr>
          )}
          {!loading && paddingTop > 0 && (
            <tr aria-hidden>
              <td colSpan={5} style={{ height: paddingTop }} />
            </tr>
          )}
          {!loading && visible.map((n) => (
            <tr key={n.node_id} className="group border-b border-border/50 last:border-b-0">
              <td className="px-4 py-2 font-mono text-xs bg-surface group-hover:bg-surface/40 align-middle max-w-0">
                <span className="block truncate" title={n.node_id}>{n.node_id}</span>
              </td>
              <td className="px-4 py-2 text-center text-foreground/70 bg-surface group-hover:bg-surface/40 align-middle">{n.is_complete ? '✓' : '—'}</td>
              <td className="px-4 py-2 text-center text-foreground/70 bg-surface group-hover:bg-surface/40 align-middle">{n.is_revealed ? '✓' : '—'}</td>
              <td className="px-4 py-2 text-center text-foreground/70 bg-surface group-hover:bg-surface/40 align-middle">{n.has_pending_reward ? '✓' : '—'}</td>
              <td className="px-4 py-2 bg-surface group-hover:bg-surface/40 align-middle">
                <div className="flex gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={busy}
                    className="flex-1"
                    onPress={() => onComplete(n)}
                  >
                    {n.is_complete
                      ? t('players.actions.journey.redo')
                      : t('players.actions.journey.complete')}
                  </Button>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    isDisabled={busy}
                    className="flex-1"
                    onPress={() => onReset(n)}
                  >
                    {t('players.actions.journey.reset')}
                  </Button>
                </div>
              </td>
            </tr>
          ))}
          {!loading && paddingBottom > 0 && (
            <tr aria-hidden>
              <td colSpan={5} style={{ height: paddingBottom }} />
            </tr>
          )}
          {!loading && sorted.length === 0 && (
            <tr>
              <td colSpan={5} className="py-12 text-center text-xs text-muted">
                {nodes.length === 0
                  ? t('players.actions.journey.noNodes')
                  : t('players.actions.journey.noMatching')}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
