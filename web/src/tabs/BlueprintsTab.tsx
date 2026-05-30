import { useState, useEffect, useCallback } from 'react'
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
import { api } from '../api/client'
import type { BlueprintRow, Player } from '../api/client'
import { DataTable, Dropzone, Icon, PageHeader, type Column } from '../dune-ui'

type Key = 'id' | 'owner_name' | 'name' | 'item_id' | 'pieces' | 'placeables' | 'actions'

const COLUMNS: Column<Key>[] = [
  { key: 'id', label: 'ID', width: 80 },
  { key: 'owner_name', label: 'Owner', minWidth: 140 },
  { key: 'name', label: 'Name', minWidth: 200 },
  { key: 'item_id', label: 'Item ID', minWidth: 200 },
  { key: 'pieces', label: 'Pieces', width: 100 },
  { key: 'placeables', label: 'Placeables', width: 110 },
  { key: 'actions', label: '', width: 110, sortable: false },
]

export default function BlueprintsTab({ isSignedIn = true }: { isSignedIn?: boolean }) {
  const [blueprints, setBlueprints] = useState<BlueprintRow[]>([])
  const [loading, setLoading] = useState(false)
  const [showImport, setShowImport] = useState(false)

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.blueprints.list())
      .then(setBlueprints)
      .catch((e: unknown) => toast.danger(`Failed to load blueprints: ${e instanceof Error ? e.message : String(e)}`))
      .finally(() => setLoading(false))
  }, [])

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
            <strong>Layout Tools</strong>
            {' '}
            account is required to export or import blueprints. Sign in using the button
            in the top right.
          </span>
        </div>
      )}

      <PageHeader
        title={`Blueprints (${blueprints.length})`}
        subtitle="Manage saved base blueprints. Export or import player constructions."
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
                  Refresh
                </>
              )}
        </Button>
        <Button size="sm" onPress={() => setShowImport(true)} isDisabled={!isSignedIn}>
          <Icon name="upload" />
          {' '}
          Import Blueprint
        </Button>
      </PageHeader>

      {loading
        ? (
            <div className="flex justify-center py-12">
              <Spinner size="lg" />
            </div>
          )
        : (
            <DataTable<BlueprintRow, Key>
              aria-label="Blueprints"
              className="min-h-0 max-h-full"
              columns={COLUMNS}
              rows={blueprints}
              rowId={(b) => String(b.id)}
              initialSort={{ column: 'id', direction: 'ascending' }}
              sortValue={(b, k) => (k === 'actions' ? '' : (b as unknown as Record<string, string | number>)[k])}
              emptyState={<div className="py-8 text-center text-muted">No blueprints found.</div>}
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
                              Export
                            </Button>
                          </a>
                        )
                      : (
                          <Button size="sm" variant="outline" className="w-full" isDisabled>
                            <Icon name="download" />
                            {' '}
                            Export
                          </Button>
                        )
                }
              }}
            />
          )}

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

function ImportModal({ open, onClose, onSuccess }: { open: boolean, onClose: () => void, onSuccess: () => void }) {
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
      toast.warning('Select a blueprint file')
      return
    }
    if (!selectedPlayer) {
      toast.warning('Select a player')
      return
    }
    setSubmitting(true)
    try {
      const res = await api.blueprints.import(file, selectedPlayer.id)
      if (res.ok) {
        toast.success('Blueprint imported successfully')
        onSuccess()
      }
      else {
        toast.danger(`Import failed: ${res.error ?? 'unknown error'}`)
      }
    }
    catch (e: unknown) {
      toast.danger(`Import failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
        <Modal.Container>
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">Import Blueprint</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4">
              <TextField>
                <Label>Blueprint File</Label>
                <Dropzone
                  accept=".json"
                  file={file}
                  onSelect={setFile}
                  prompt="Drop or click to upload a .json blueprint file"
                />
              </TextField>

              <TextField>
                <Label>Player</Label>
                <Select
                  aria-label="Player"
                  placeholder="Select a player…"
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
                        aria-label="Players"
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
                Cancel
              </Button>
              <Button onPress={handleSubmit} isDisabled={submitting || !file || !selectedPlayer}>
                {submitting ? <Spinner size="sm" color="current" /> : <Icon name="upload" />}
                Import
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
