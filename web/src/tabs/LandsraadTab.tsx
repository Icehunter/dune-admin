import { useEffect } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Spinner } from '@/components/ui/spinner'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { api } from '../api/client'
import type { LandsraadTask } from '../api/client'
import { qk } from '../api/queryKeys'
import { DataTable, Icon, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'

type TaskKey = 'board_index' | 'house' | 'goal_amount' | 'completed' | 'sysselraad'

const Field: React.FC<{ label: string, value: string }> = ({ label, value }) => (
  <div>
    <div className="text-xs text-muted">{label}</div>
    <div className="text-foreground">{value}</div>
  </div>
)

// One labelled value's worth of skeleton, used while the overview first loads so
// we never flash "no current term" before the data has even arrived.
const FieldSkeleton: React.FC = () => (
  <div className="space-y-1.5">
    <Skeleton className="h-3 w-16" />
    <Skeleton className="h-4 w-28" />
  </div>
)

export const LandsraadTab: React.FC = () => {
  const { t } = useTranslation()

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: qk.landsraad.overview,
    queryFn: api.landsraad.get,
  })

  useEffect(() => {
    if (error) {
      toast.danger(t('landsraad.failedToLoad', { message: error instanceof Error ? error.message : String(error) }))
    }
  }, [error, t])

  const term = data?.term ?? null
  const decrees = data?.decrees ?? []
  const tasks = data?.tasks ?? []

  const fmtDate = (s: string) => {
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
  }
  const dash = (s: string) => s || '—'

  const TASK_COLUMNS: Column<TaskKey>[] = [
    { key: 'board_index', label: t('landsraad.tasks.index'), width: 70 },
    { key: 'house', label: t('landsraad.tasks.house'), minWidth: 160 },
    { key: 'goal_amount', label: t('landsraad.tasks.goal'), width: 120 },
    { key: 'completed', label: t('landsraad.tasks.completed'), width: 120 },
    { key: 'sysselraad', label: t('landsraad.tasks.sysselraad'), width: 120 },
  ]

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('landsraad.title')} subtitle={t('landsraad.subtitle')}>
        <Button size="sm" variant="ghost" onClick={() => refetch()} disabled={isFetching}>
          {isFetching
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

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6 pr-1">
        <Panel>
          <SectionLabel>{t('landsraad.currentTerm')}</SectionLabel>
          {isLoading
            ? (
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 mt-2">
                  {Array.from({ length: 6 }, (_, i) => <FieldSkeleton key={i} />)}
                </div>
              )
            : term
              ? (
                  <>
                    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 mt-2 text-sm">
                      <Field label={t('landsraad.term.id')} value={`#${term.term_id}`} />
                      <Field
                        label={t('landsraad.term.window')}
                        value={`${fmtDate(term.start_time)} → ${fmtDate(term.end_time)}`}
                      />
                      <Field label={t('landsraad.term.reigning')} value={dash(term.reigning_faction)} />
                      <Field label={t('landsraad.term.activeDecree')} value={dash(term.active_decree)} />
                      <Field label={t('landsraad.term.electedDecree')} value={dash(term.elected_decree)} />
                      <Field label={t('landsraad.term.winning')} value={dash(term.winning_faction)} />
                    </div>
                    {term.test_term && (
                      <Badge tone="warning" className="mt-2">{t('landsraad.testTerm')}</Badge>
                    )}
                  </>
                )
              : <div className="text-xs text-muted mt-2">{t('landsraad.noTerm')}</div>}
        </Panel>

        <Panel>
          <SectionLabel>{t('landsraad.decrees')}</SectionLabel>
          <div className="text-xs text-muted mb-2">{t('landsraad.decreesDesc')}</div>
          {isLoading
            ? (
                <div className="mt-1 flex flex-col gap-2">
                  {Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-6 w-full" />)}
                </div>
              )
            : decrees.length === 0
              ? <div className="text-xs text-muted">{t('landsraad.noDecrees')}</div>
              : (
                  <div className="mt-1">
                    {decrees.map((d) => (
                      <div
                        key={d.id}
                        className="flex items-center justify-between py-1.5 border-b border-border/40 text-sm"
                      >
                        <span className="text-foreground">{d.name}</span>
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-muted">{t('landsraad.weight', { weight: d.weight })}</span>
                          <Badge tone={d.disabled ? 'danger' : 'success'}>
                            {d.disabled ? t('landsraad.disabled') : t('landsraad.enabled')}
                          </Badge>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
        </Panel>

        <div>
          <SectionLabel>{t('landsraad.taskBoard')}</SectionLabel>
          <div className="text-xs text-muted mb-2">{t('landsraad.taskBoardDesc')}</div>
          <DataTable<LandsraadTask, TaskKey>
            aria-label={t('landsraad.taskBoard')}
            className="min-h-0"
            columns={TASK_COLUMNS}
            rows={tasks}
            loading={isLoading}
            rowId={(tk) => String(tk.id)}
            initialSort={{ column: 'board_index', direction: 'ascending' }}
            sortValue={(tk, k) => {
              switch (k) {
                case 'board_index': return tk.board_index
                case 'house': return tk.house
                case 'goal_amount': return tk.goal_amount
                case 'completed': return tk.completed ? 1 : 0
                case 'sysselraad': return tk.sysselraad ? 1 : 0
                default: return ''
              }
            }}
            emptyState={<div className="py-8 text-center text-muted">{t('landsraad.noTasks')}</div>}
            renderCell={(tk, key) => {
              switch (key) {
                case 'board_index':
                  return <span className="font-mono text-muted">{tk.board_index}</span>
                case 'house':
                  return tk.house || <span className="text-muted">—</span>
                case 'goal_amount':
                  return <span className="text-muted">{tk.goal_amount.toLocaleString()}</span>
                case 'completed':
                  return (
                    <Badge tone={tk.completed ? 'success' : 'default'}>
                      {tk.completed ? t('landsraad.tasks.done') : t('landsraad.tasks.open')}
                    </Badge>
                  )
                case 'sysselraad':
                  return tk.sysselraad
                    ? <Badge tone="accent">{t('landsraad.tasks.yes')}</Badge>
                    : <span className="text-muted">—</span>
              }
            }}
          />
        </div>
      </div>
    </div>
  )
}
