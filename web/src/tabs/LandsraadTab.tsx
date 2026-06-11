import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { api } from '../api/client'
import type { LandsraadOverview, LandsraadTask } from '../api/client'
import { DataTable, Icon, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'
import type { TaskKey, FieldProps } from './types'

const Field: React.FC<FieldProps> = ({ label, value }) => (
  <div>
    <div className="text-xs text-muted">{label}</div>
    <div className="text-foreground">{value}</div>
  </div>
)

export const LandsraadTab: React.FC = () => {
  const { t } = useTranslation()
  const [data, setData] = React.useState<LandsraadOverview | null>(null)
  const [loading, setLoading] = React.useState(false)

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.landsraad.get())
      .then(setData)
      .catch((e: unknown) =>
        toast.danger(t('landsraad.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  React.useEffect(() => {
    load()
  }, [load])

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

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6 pr-1">
        <Panel>
          <SectionLabel>{t('landsraad.currentTerm')}</SectionLabel>
          {term
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
                    <Chip size="sm" variant="soft" color="warning" className="mt-2">{t('landsraad.testTerm')}</Chip>
                  )}
                </>
              )
            : <div className="text-xs text-muted mt-2">{t('landsraad.noTerm')}</div>}
        </Panel>

        <Panel>
          <SectionLabel>{t('landsraad.decrees')}</SectionLabel>
          <div className="text-xs text-muted mb-2">{t('landsraad.decreesDesc')}</div>
          {decrees.length === 0
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
                        <Chip size="sm" variant="soft" color={d.disabled ? 'danger' : 'success'}>
                          {d.disabled ? t('landsraad.disabled') : t('landsraad.enabled')}
                        </Chip>
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
            loading={loading}
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
            emptyState={(
              <EmptyState size="sm">
                <EmptyState.Header>
                  <EmptyState.Title>{t('landsraad.noTasks')}</EmptyState.Title>
                </EmptyState.Header>
              </EmptyState>
            )}
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
                    <Chip size="sm" variant="soft" color={tk.completed ? 'success' : 'default'}>
                      {tk.completed ? t('landsraad.tasks.done') : t('landsraad.tasks.open')}
                    </Chip>
                  )
                case 'sysselraad':
                  return tk.sysselraad
                    ? <Chip size="sm" variant="soft" color="accent">{t('landsraad.tasks.yes')}</Chip>
                    : <span className="text-muted">—</span>
              }
            }}
          />
        </div>
      </div>
    </div>
  )
}
