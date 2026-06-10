import type React from 'react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Input, Modal, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { DataTable, SectionLabel, type Column } from '../../../dune-ui'
import { api } from '../../../api/client'
import type { TeleportLocation } from '../../../api/client'

interface ManageLocationsModalProps {
  onClose: (updated?: TeleportLocation[]) => void
}

type LocationKey = 'name' | 'x' | 'y' | 'z' | 'actions'

const COLUMNS: Column<LocationKey>[] = [
  { key: 'name', label: 'Name', isRowHeader: true, minWidth: 160 },
  { key: 'x', label: 'X', width: 100 },
  { key: 'y', label: 'Y', width: 100 },
  { key: 'z', label: 'Z', width: 80 },
  { key: 'actions', label: ' ', sortable: false, width: 80 },
]

export const ManageLocationsModal: React.FC<ManageLocationsModalProps> = ({ onClose }) => {
  const { t } = useTranslation()
  const [locations, setLocations] = useState<TeleportLocation[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('')
  const [x, setX] = useState('')
  const [y, setY] = useState('')
  const [z, setZ] = useState('')

  useEffect(() => {
    api.locations.list()
      .then(setLocations)
      .catch(() => toast.danger('Failed to load locations'))
      .finally(() => setLoading(false))
  }, [])

  const handleUpsert = async () => {
    if (!name.trim()) {
      toast.danger('Name is required')
      return
    }
    try {
      const locs = await api.locations.upsert(
        name.trim(),
        Number(x) || 0,
        Number(y) || 0,
        Number(z) || 0,
      )
      setLocations(locs)
      setName('')
      setX('')
      setY('')
      setZ('')
      toast.success(`Saved "${name.trim()}"`)
    }
    catch (e) {
      toast.danger(e instanceof Error ? e.message : 'Failed to save')
    }
  }

  const handleDelete = async (locName: string) => {
    try {
      const locs = await api.locations.remove(locName)
      setLocations(locs)
      toast.success(`Deleted "${locName}"`)
    }
    catch (e) {
      toast.danger(e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  const editIntoForm = (loc: TeleportLocation) => {
    setName(loc.name)
    setX(String(Math.round(loc.x)))
    setY(String(Math.round(loc.y)))
    setZ(String(Math.round(loc.z)))
  }

  return (
    <Modal.Backdrop isOpen onOpenChange={(v) => { if (!v) onClose(locations) }}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('players.actions.admin.manageLocationsModal.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4" style={{ minWidth: 580 }}>
            <div>
              <SectionLabel>{t('players.actions.admin.manageLocationsModal.addSection')}</SectionLabel>
              <p className="text-xs text-muted mt-2 mb-3">
                {t('players.actions.admin.manageLocationsModal.addHint')}
              </p>
              <div className="grid grid-cols-[1fr_90px_90px_90px_auto] gap-2 items-end">
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted">{t('players.actions.admin.manageLocationsModal.nameLabel')}</span>
                  <Input
                    aria-label={t('players.actions.admin.manageLocationsModal.nameLabel')}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t('players.actions.admin.manageLocationsModal.namePlaceholder')}
                    onKeyDown={(e) => { if (e.key === 'Enter') void handleUpsert() }}
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted">X</span>
                  <Input
                    aria-label="X coordinate"
                    value={x}
                    onChange={(e) => setX(e.target.value)}
                    placeholder="0"
                    onKeyDown={(e) => { if (e.key === 'Enter') void handleUpsert() }}
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted">Y</span>
                  <Input
                    aria-label="Y coordinate"
                    value={y}
                    onChange={(e) => setY(e.target.value)}
                    placeholder="0"
                    onKeyDown={(e) => { if (e.key === 'Enter') void handleUpsert() }}
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted">Z</span>
                  <Input
                    aria-label="Z coordinate"
                    value={z}
                    onChange={(e) => setZ(e.target.value)}
                    placeholder="0"
                    onKeyDown={(e) => { if (e.key === 'Enter') void handleUpsert() }}
                  />
                </div>
                <Button onPress={handleUpsert} className="self-end">{t('players.actions.admin.manageLocationsModal.save')}</Button>
              </div>
            </div>

            {loading
              ? (
                  <div className="flex justify-center py-8">
                    <Spinner />
                  </div>
                )
              : (
                  <DataTable<TeleportLocation, LocationKey>
                    aria-label="Saved locations"
                    columns={COLUMNS}
                    rows={locations}
                    rowId={(loc) => loc.name}
                    initialSort={{ column: 'name', direction: 'ascending' }}
                    emptyState={(
                      <EmptyState size="sm">
                        <EmptyState.Header>
                          <EmptyState.Title>
                            {t('players.actions.admin.manageLocationsModal.noLocations')}
                          </EmptyState.Title>
                        </EmptyState.Header>
                      </EmptyState>
                    )}
                    renderCell={(loc, key) => {
                      switch (key) {
                        case 'name':
                          return (
                            <Button
                              variant="ghost"
                              className="text-left text-foreground hover:text-accent font-medium w-full truncate px-0 h-auto min-w-0 justify-start"
                              onPress={() => editIntoForm(loc)}
                              aria-label="Click to load into editor"
                            >
                              {loc.name}
                            </Button>
                          )
                        case 'x':
                          return <span className="font-mono text-xs text-muted">{Math.round(loc.x).toLocaleString()}</span>
                        case 'y':
                          return <span className="font-mono text-xs text-muted">{Math.round(loc.y).toLocaleString()}</span>
                        case 'z':
                          return <span className="font-mono text-xs text-muted">{Math.round(loc.z).toLocaleString()}</span>
                        case 'actions':
                          return (
                            <Button
                              size="sm"
                              variant="danger-soft"
                              onPress={() => void handleDelete(loc.name)}
                            >
                              {t('players.actions.admin.manageLocationsModal.delete')}
                            </Button>
                          )
                        default:
                          return null
                      }
                    }}
                  />
                )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="ghost" onPress={() => onClose(locations)}>{t('players.actions.admin.manageLocationsModal.done')}</Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
