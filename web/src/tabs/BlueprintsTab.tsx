import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Label,
  ListBox,
  ListLayout,
  Modal,
  Select,
  Spinner,
  TextField,
  Virtualizer,
  toast,
} from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../api/client'
import type { BlueprintRow, Player } from '../api/client'
import { DataTable, Dropzone, Icon, PageHeader, type Column } from '../dune-ui'

type Key = 'id' | 'owner_name' | 'name' | 'item_id' | 'pieces' | 'placeables' | 'actions'

interface BlueprintsTabProps {
  isSignedIn?: boolean
}

export const BlueprintsTab: React.FC<BlueprintsTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()
  const [blueprints, setBlueprints] = useState<BlueprintRow[]>([])
  const [loading, setLoading] = useState(false)
  const [showImport, setShowImport] = useState(false)

  const COLUMNS: Column<Key>[] = [
    { key: 'id', label: t('blueprints.columns.id'), width: 80 },
    { key: 'owner_name', label: t('blueprints.columns.owner'), minWidth: 140 },
    { key: 'name', label: t('blueprints.columns.name'), minWidth: 200 },
    { key: 'item_id', label: t('blueprints.columns.itemId'), minWidth: 200 },
    { key: 'pieces', label: t('blueprints.columns.pieces'), width: 100 },
    { key: 'placeables', label: t('blueprints.columns.placeables'), width: 110 },
    { key: 'actions', label: '', width: 110, sortable: false },
  ]

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.blueprints.list())
      .then(setBlueprints)
      .catch((e: unknown) => toast.danger(t('blueprints.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      {!isSignedIn && (
        <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
          <Icon name="triangle-alert" />
          <span>
            A
            {' '}
            <strong>{t('blueprints.layoutAccountStrong')}</strong>
            {' '}
            account is required to export or import blueprints. Sign in using the button
            in the top right.
          </span>
        </div>
      )}

      <PageHeader
        title={t('blueprints.title', { count: blueprints.length })}
        subtitle={t('blueprints.subtitle')}
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
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
        <Button size="sm" onPress={() => setShowImport(true)} isDisabled={!isSignedIn}>
          <Icon name="upload" />
          {' '}
          {t('blueprints.importBlueprint')}
        </Button>
      </PageHeader>

      <DataTable<BlueprintRow, Key>
        aria-label={t('blueprints.ariaLabel')}
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={blueprints}
        loading={loading}
        rowId={(b) => String(b.id)}
        initialSort={{ column: 'id', direction: 'ascending' }}
        sortValue={(b, k) => (k === 'actions' ? '' : (b as unknown as Record<string, string | number>)[k])}
        emptyState={(
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:box" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('blueprints.noBlueprintsFound')}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}
        renderCell={(b, key) => {
          switch (key) {
            case 'id':
              return <span className="font-mono text-muted">{b.id}</span>
            case 'owner_name':
              return b.owner_name
            case 'name':
              return b.name || <span className="text-muted">—</span>
            case 'item_id':
              return <span className="font-mono text-muted">{b.item_id}</span>
            case 'pieces':
              return <span className="text-muted">{b.pieces}</span>
            case 'placeables':
              return <span className="text-muted">{b.placeables}</span>
            case 'actions':
              return isSignedIn
                ? (
                    <a
                      href={api.blueprints.exportUrl(b.id)}
                      download={b.name ? `${b.name.replace(/[/\\:*?"<>|]/g, '_')}.json` : `blueprint_${b.id}.json`}
                    >
                      <Button size="sm" variant="outline" className="w-full">
                        <Icon name="download" />
                        {' '}
                        {t('common.export')}
                      </Button>
                    </a>
                  )
                : (
                    <Button size="sm" variant="outline" className="w-full" isDisabled>
                      <Icon name="download" />
                      {' '}
                      {t('common.export')}
                    </Button>
                  )
          }
        }}
      />

      <ImportModal
        open={showImport}
        onClose={() => setShowImport(false)}
        onSuccess={() => {
          setShowImport(false)
          load()
        }}
      />
    </div>
  )
}

interface ImportModalProps {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

function ImportModal({ open, onClose, onSuccess }: ImportModalProps) {
  const { t } = useTranslation()
  const [file, setFile] = useState<File | null>(null)
  const [players, setPlayers] = useState<Player[]>([])
  const [selectedPlayerId, setSelectedPlayerId] = useState<number | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    Promise.resolve()
      .then(() => {
        setFile(null)
        setSelectedPlayerId(null)
      })
      .then(() => api.players.list())
      .then(setPlayers)
      .catch(() => {})
  }, [open])

  const selectedPlayer = players.find((p) => p.id === selectedPlayerId) ?? null

  const handleSubmit = async () => {
    if (!file) {
      toast.warning(t('blueprints.selectFile'))
      return
    }
    if (!selectedPlayer) {
      toast.warning(t('blueprints.selectPlayer'))
      return
    }
    setSubmitting(true)
    try {
      const res = await api.blueprints.import(file, selectedPlayer.id)
      if (res.ok) {
        toast.success(t('blueprints.importSuccess'))
        onSuccess()
      }
      else {
        toast.danger(t('blueprints.importFailed', { message: res.error ?? 'unknown error' }))
      }
    }
    catch (e: unknown) {
      toast.danger(t('blueprints.importFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container>
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('blueprints.importModal.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4">
            <TextField>
              <Label>{t('blueprints.importModal.blueprintFile')}</Label>
              <Dropzone
                accept=".json"
                file={file}
                onSelect={setFile}
                prompt={t('blueprints.importModal.dropzone')}
              />
            </TextField>

            <TextField>
              <Label>{t('blueprints.importModal.playerLabel')}</Label>
              <Select
                aria-label={t('blueprints.importModal.playerLabel')}
                placeholder={t('blueprints.importModal.playerPlaceholder')}
                selectedKey={selectedPlayerId !== null ? String(selectedPlayerId) : null}
                onSelectionChange={(k) => setSelectedPlayerId(k ? Number(k) : null)}
                className="w-full"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover className="!w-[320px] !max-w-[90vw]">
                  <Virtualizer layout={ListLayout} layoutOptions={{ rowHeight: 36 }}>
                    <ListBox
                      aria-label={t('blueprints.importModal.playersLabel')}
                      className="overflow-y-auto"
                      style={{ height: Math.min(players.length * 36 + 8, 320) }}
                      items={players.map((p) => ({ id: String(p.id), name: p.name, actorId: p.id }))}
                    >
                      {(item: { id: string, name: string, actorId: number }) => (
                        <ListBox.Item id={item.id} textValue={item.name}>
                          <span className="flex items-baseline gap-2">
                            <span>{item.name}</span>
                            <span className="text-xs text-muted font-mono">
                              #
                              {item.actorId}
                            </span>
                          </span>
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                      )}
                    </ListBox>
                  </Virtualizer>
                </Select.Popover>
              </Select>
            </TextField>
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" slot="close">
              {t('common.cancel')}
            </Button>
            <Button onPress={handleSubmit} isDisabled={submitting || !file || !selectedPlayer}>
              {submitting ? <Spinner size="sm" color="current" /> : <Icon name="upload" />}
              {t('blueprints.importModal.import')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
