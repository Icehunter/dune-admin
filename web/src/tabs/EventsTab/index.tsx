import type React from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Switch, toast } from '@heroui/react'
import { api } from '../../api/client'
import type { EventDefinition, EventClaimRecord } from '../../api/client'
import { DataTable, Icon, PageHeader, Panel, SectionLabel, type Column } from '../../dune-ui'
import { EventEditorModal } from './modals/EventEditorModal'

type ListKey = 'name' | 'type' | 'enabled' | 'claims' | 'actions'
type ClaimKey = 'account_id' | 'version' | 'status' | 'attempts' | 'claimed_at' | 'last_error'

export const EventsTab: React.FC = () => {
  const { t } = useTranslation()
  const [events, setEvents] = useState<EventDefinition[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedEvent, setSelectedEvent] = useState<EventDefinition | null>(null)
  const [claims, setClaims] = useState<EventClaimRecord[]>([])
  const [claimsLoading, setClaimsLoading] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingEvent, setEditingEvent] = useState<EventDefinition | null>(null)

  const LIST_COLUMNS: Column<ListKey>[] = [
    { key: 'name', label: t('events.columns.name'), minWidth: 200 },
    { key: 'type', label: t('events.columns.type'), width: 120 },
    { key: 'enabled', label: t('events.columns.enabled'), width: 90, sortable: false },
    { key: 'claims', label: t('events.columns.claims'), width: 80 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  const CLAIM_COLUMNS: Column<ClaimKey>[] = [
    { key: 'account_id', label: t('events.claims.accountId'), width: 110 },
    { key: 'version', label: t('events.claims.version'), width: 70 },
    { key: 'status', label: t('events.claims.status'), width: 90 },
    { key: 'attempts', label: t('events.claims.attempts'), width: 80 },
    { key: 'claimed_at', label: t('events.claims.claimedAt'), minWidth: 160 },
    { key: 'last_error', label: t('events.claims.lastError'), minWidth: 200 },
  ]

  const loadEvents = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.events.list())
      .then(setEvents)
      .catch((e: unknown) => {
        toast.danger(t('events.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setLoading(false))
  }, [t])

  const loadStatus = useCallback(
    (ev: EventDefinition) => {
      setSelectedEvent(ev)
      setClaimsLoading(true)
      api.events
        .status(ev.id)
        .then((s) => setClaims(s.claims))
        .catch((e: unknown) => {
          toast.danger(t('events.failedToLoadStatus', { message: e instanceof Error ? e.message : String(e) }))
        })
        .finally(() => setClaimsLoading(false))
    },
    [t],
  )

  useEffect(() => {
    loadEvents()
  }, [loadEvents])

  const handleToggleEnabled = (ev: EventDefinition) => {
    api.events
      .setEnabled(ev.id, !ev.enabled)
      .then(loadEvents)
      .catch((e: unknown) => {
        toast.danger(t('events.toggleFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const handleDelete = (ev: EventDefinition) => {
    if (!confirm(t('events.confirmDelete', { name: ev.name }))) return
    api.events
      .delete(ev.id)
      .then(() => {
        if (selectedEvent?.id === ev.id) setSelectedEvent(null)
        loadEvents()
      })
      .catch((e: unknown) => {
        toast.danger(t('events.deleteFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const handleReset = (ev: EventDefinition) => {
    if (!confirm(t('events.confirmReset', { name: ev.name }))) return
    api.events
      .reset(ev.id)
      .then(() => {
        toast.success(t('events.resetSuccess'))
        if (selectedEvent?.id === ev.id) loadStatus(ev)
      })
      .catch((e: unknown) => {
        toast.danger(t('events.resetFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
  }

  const openCreate = () => {
    setEditingEvent(null)
    setEditorOpen(true)
  }

  const openEdit = (ev: EventDefinition) => {
    setEditingEvent(ev)
    setEditorOpen(true)
  }

  const typeChipColor = (type: EventDefinition['type']): 'warning' | 'accent' =>
    type === 'zone_race' ? 'warning' : 'accent'

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('events.title', { count: events.length })} subtitle={t('events.subtitle')}>
        <Button size="sm" variant="ghost" onPress={loadEvents} isDisabled={loading}>
          <Icon name="refresh-cw" />
          {' '}
          {t('common.refresh')}
        </Button>
        <Button size="sm" variant="primary" onPress={openCreate}>
          <Icon name="plus" />
          {' '}
          {t('events.create')}
        </Button>
      </PageHeader>

      <DataTable<EventDefinition, ListKey>
        aria-label={t('events.ariaLabel')}
        className="min-h-0 flex-1"
        columns={LIST_COLUMNS}
        rows={events}
        loading={loading}
        rowId={(e) => String(e.id)}
        initialSort={{ column: 'name', direction: 'ascending' }}
        sortValue={(e, k) => {
          if (k === 'enabled') return e.enabled ? 1 : 0
          if (k === 'actions' || k === 'claims') return ''
          return (e as unknown as Record<string, string | number>)[k] ?? ''
        }}
        emptyState={<div className="py-8 text-center text-muted">{t('events.noEvents')}</div>}
        renderCell={(ev, key) => {
          switch (key) {
            case 'name':
              return (
                <button
                  className="text-left text-accent hover:underline"
                  onClick={() => loadStatus(ev)}
                  type="button"
                >
                  {ev.name}
                </button>
              )
            case 'type':
              return (
                <Chip size="sm" variant="soft" color={typeChipColor(ev.type)}>
                  {ev.type === 'zone_race' ? t('events.types.zoneRace') : t('events.types.milestone')}
                </Chip>
              )
            case 'enabled':
              return (
                <Switch
                  size="sm"
                  isSelected={ev.enabled}
                  onChange={() => handleToggleEnabled(ev)}
                  aria-label={t('events.toggleEnabled')}
                />
              )
            case 'claims':
              return <span className="text-muted font-mono text-xs">{ev.version}</span>
            case 'actions':
              return (
                <div className="flex gap-1">
                  <Button size="sm" variant="ghost" onPress={() => openEdit(ev)} aria-label={t('common.edit') as string}>
                    <Icon name="pencil" />
                  </Button>
                  <Button size="sm" variant="danger-soft" onPress={() => handleDelete(ev)} aria-label={t('common.delete') as string}>
                    <Icon name="trash-2" />
                  </Button>
                </div>
              )
          }
        }}
      />

      {selectedEvent && (
        <Panel>
          <div className="flex items-center justify-between mb-3">
            <SectionLabel>
              {t('events.status.title', { name: selectedEvent.name })}
            </SectionLabel>
            <div className="flex gap-2">
              <Button size="sm" variant="ghost" onPress={() => loadStatus(selectedEvent)} isDisabled={claimsLoading}>
                <Icon name="refresh-cw" />
              </Button>
              <Button size="sm" variant="outline" onPress={() => handleReset(selectedEvent)}>
                {t('events.status.reset')}
              </Button>
              <Button size="sm" variant="ghost" onPress={() => setSelectedEvent(null)}>
                <Icon name="x" />
              </Button>
            </div>
          </div>
          <DataTable<EventClaimRecord, ClaimKey>
            aria-label={t('events.status.claimsLabel')}
            className="max-h-64"
            columns={CLAIM_COLUMNS}
            rows={claims}
            loading={claimsLoading}
            rowId={(c) => `${c.event_id}-${c.version}-${c.account_id}`}
            initialSort={{ column: 'claimed_at', direction: 'descending' }}
            sortValue={(c, k) => (c as unknown as Record<string, string | number>)[k] ?? ''}
            emptyState={<div className="py-4 text-center text-muted text-xs">{t('events.status.noClaims')}</div>}
            renderCell={(c, key) => {
              switch (key) {
                case 'account_id':
                  return <span className="font-mono text-xs">{c.account_id}</span>
                case 'version':
                  return <span className="font-mono text-xs text-muted">{c.version}</span>
                case 'status':
                  return (
                    <Chip size="sm" variant="soft" color={c.status === 'granted' ? 'success' : 'danger'}>
                      {c.status}
                    </Chip>
                  )
                case 'attempts':
                  return <span className="text-muted text-xs">{c.attempts}</span>
                case 'claimed_at':
                  return <span className="text-muted text-xs">{c.claimed_at || '—'}</span>
                case 'last_error':
                  return <span className="text-muted text-xs">{c.last_error || '—'}</span>
              }
            }}
          />
        </Panel>
      )}

      <EventEditorModal
        isOpen={editorOpen}
        onClose={() => setEditorOpen(false)}
        editing={editingEvent}
        onSaved={loadEvents}
      />
    </div>
  )
}
