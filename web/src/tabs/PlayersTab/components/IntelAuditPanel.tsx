import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { IntelAuditRow } from '../../../api/client'
import { DataTable, Panel, SectionLabel } from '../../../dune-ui'
import type { Column } from '../../../dune-ui'
import { usePermissions } from '../../../hooks/usePermissions'
import type { IntelAuditCol } from './types'

// Every column below is sortable, and the header's sort chevron renders
// inline after the label text (plain text flow, not a fixed-position icon)
// — so any column can wrap the chevron onto its own line the moment it
// becomes the active sort column, if its width was only ever sized for the
// bare label. Each width here includes headroom for that inline icon, not
// just the label text.
const columns: Column<IntelAuditCol>[] = [
  { key: 'name', label: 'Name', width: '1fr', minWidth: 160 },
  { key: 'account_id', label: 'Account ID', width: 140, align: 'end' },
  { key: 'level', label: 'Level', width: 90, align: 'end' },
  { key: 'intel', label: 'Intel', width: 100, align: 'end' },
  { key: 'expected_intel', label: 'Expected', width: 120, align: 'end' },
  { key: 'delta', label: 'Over by', width: 120, align: 'end' },
  { key: 'actions', label: '', width: 150, align: 'end', sortable: false },
]

// IntelAuditPanel lists characters holding more intel than their level should
// have earned (#293 mass-grant cleanup) with a one-click "set to expected"
// repair per row. Players must be offline for the repair to apply.
export const IntelAuditPanel: React.FC = (): React.ReactElement => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canWrite = can('players:write')

  const [rows, setRows] = React.useState<IntelAuditRow[]>([])
  const [loading, setLoading] = React.useState(false)
  const [fixing, setFixing] = React.useState<number | null>(null)

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.intelAudit())
      .then(setRows)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, [])

  const fixRow = (row: IntelAuditRow): void => {
    setFixing(row.pawn_id)
    api.players.setIntel(row.pawn_id, row.expected_intel)
      .then(() => {
        toast.success(t('players.intelAudit.fixed', { name: row.name, intel: row.expected_intel }))
        load()
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setFixing(null))
  }

  const renderActions = (row: IntelAuditRow): React.ReactNode => {
    if (!canWrite) return null
    if (row.online) {
      return <Chip size="sm" variant="soft" color="warning">{t('players.intelAudit.online')}</Chip>
    }
    return (
      <Button
        size="sm"
        variant="ghost"
        isDisabled={fixing !== null}
        isPending={fixing === row.pawn_id}
        onPress={() => fixRow(row)}
      >
        {t('players.intelAudit.setExpected')}
      </Button>
    )
  }

  const renderCell = (row: IntelAuditRow, key: IntelAuditCol): React.ReactNode => {
    switch (key) {
      case 'name': return row.name
      case 'account_id': return <span className="tabular-nums text-muted font-mono">{row.account_id}</span>
      case 'level': return <span className="tabular-nums">{row.level}</span>
      case 'intel': return <span className="tabular-nums">{row.intel.toLocaleString()}</span>
      case 'expected_intel': return <span className="tabular-nums">{row.expected_intel.toLocaleString()}</span>
      case 'delta': return <span className="tabular-nums text-danger">{`+${(row.intel - row.expected_intel).toLocaleString()}`}</span>
      case 'actions': return renderActions(row)
    }
  }

  const sortValue = (row: IntelAuditRow, key: IntelAuditCol): string | number => {
    switch (key) {
      case 'name': return row.name
      case 'account_id': return row.account_id
      case 'level': return row.level
      case 'intel': return row.intel
      case 'expected_intel': return row.expected_intel
      case 'delta': return row.intel - row.expected_intel
      case 'actions': return 0
    }
  }

  const renderBody = (): React.ReactNode => {
    if (loading && rows.length === 0) {
      return <div className="flex justify-center py-6"><Spinner size="sm" /></div>
    }
    return (
      <DataTable<IntelAuditRow, IntelAuditCol>
        aria-label={t('players.intelAudit.title')}
        columns={columns}
        rows={rows}
        rowId={(r) => String(r.pawn_id)}
        renderCell={renderCell}
        sortValue={sortValue}
        initialSort={{ column: 'delta', direction: 'descending' }}
        rowHeight={48}
        emptyState={<div className="text-sm text-muted py-4 text-center">{t('players.intelAudit.empty')}</div>}
      />
    )
  }

  return (
    <Panel>
      <div className="flex items-center justify-between">
        <SectionLabel>{t('players.intelAudit.title')}</SectionLabel>
        <Button size="sm" variant="ghost" isDisabled={loading} onPress={load}>
          {t('players.intelAudit.refresh')}
        </Button>
      </div>
      <div className="text-xs text-muted mb-2">{t('players.intelAudit.note')}</div>
      {renderBody()}
    </Panel>
  )
}
