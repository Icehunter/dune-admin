import { useState, useEffect, useMemo } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Modal, SearchField, Spinner, TextField, toast,
} from '@heroui/react'
import { api } from '../../../api/client'
import { Icon, LoadingState, NumberInput } from '../../../dune-ui'

type Container = {
  id: number
  name: string
  class: string
  map: string
  item_count: number
  item_templates: string[]
  item_names: string[]
  owner_name: string
}

export interface AddItemsModalProps {
  container: Container
  open: boolean
  onClose: () => void
  onSuccess: () => void
  onRefresh: () => void
}

export const AddItemsModal: React.FC<AddItemsModalProps> = ({
  container, open, onClose, onSuccess, onRefresh,
}) => {
  const { t } = useTranslation()
  const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState('')
  const [qty, setQty] = useState(1)
  const [quality, setQuality] = useState(0)
  const [staged, setStaged] = useState<{ template: string, qty: number, quality: number }[]>([])
  const [submitting, setSubmitting] = useState(false)
  type AddResult = { given: string[], skipped: { template: string, reason: string }[] } | null
  const [result, setResult] = useState<AddResult>(null)

  useEffect(() => {
    if (!open) return
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setQuery('')
        setSelected('')
        setQty(1)
        setQuality(0)
        setStaged([])
        setResult(null)
      })
      .then(() => api.players.templates())
      .then(setTemplates)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [open])

  const filtered = useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates
      .filter((tmpl) => tmpl.id.toLowerCase().includes(q) || tmpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, query])

  const pick = (tmpl: { id: string, name: string }) => {
    setSelected(tmpl.id)
    setQuery(tmpl.name ? `${tmpl.id}  —  ${tmpl.name}` : tmpl.id)
  }

  const addToStaged = () => {
    if (!selected) {
      toast.warning(t('storage.addModal.selectTemplate'))
      return
    }
    setStaged((prev) => [...prev, { template: selected, qty, quality }])
    setQuery('')
    setSelected('')
    setQty(1)
    setQuality(0)
  }

  const removeFromStaged = (idx: number) => {
    setStaged((prev) => prev.filter((_, i) => i !== idx))
  }

  const updateStaged = (idx: number, field: 'qty' | 'quality', value: number) => {
    setStaged((prev) => prev.map((item, i) => i === idx ? { ...item, [field]: value } : item))
  }

  const handleSubmit = async () => {
    if (staged.length === 0) return
    setSubmitting(true)
    try {
      const res = await api.storage.giveItems(container.id, staged)
      setResult(res)
      setStaged([])
      if (res.skipped.length === 0) onSuccess()
      else if (res.given.length > 0) onRefresh()
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {container.name || t('storage.containerTitle', { id: container.id })}
              {' '}
              —
              {' '}
              {t('storage.addItems')}
            </Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-3">
            {loading
              ? (
                  <LoadingState size="sm" />
                )
              : (
                  <>
                    <div className="flex items-end gap-3 shrink-0">
                      <TextField className="flex-1 min-w-0" aria-label={t('storage.addModal.templateLabel')}>
                        <div className="relative w-full">
                          <SearchField
                            className="w-full"
                            value={query}
                            onChange={(v) => {
                              setQuery(v)
                              setSelected('')
                            }}
                          >
                            <SearchField.Group>
                              <SearchField.SearchIcon />
                              <SearchField.Input placeholder={t('storage.addModal.searchPlaceholder')} />
                              <SearchField.ClearButton />
                            </SearchField.Group>
                          </SearchField>
                          {filtered.length > 0 && (
                            <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                              {filtered.map((tmpl) => (
                                <div
                                  key={tmpl.id}
                                  className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                                  onClick={() => pick(tmpl)}
                                >
                                  <span className="font-mono">{tmpl.id}</span>
                                  {tmpl.name
                                    ? (
                                        <span className="text-muted">
                                          {' '}
                                          —
                                          {tmpl.name}
                                        </span>
                                      )
                                    : null}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      </TextField>
                      <NumberInput
                        prefix={t('storage.addModal.qtyLabel')}
                        ariaLabel={t('storage.addModal.qtyLabel')}
                        min={1}
                        value={qty}
                        onChange={setQty}
                        className="w-56 shrink-0"
                      />
                      <NumberInput
                        prefix={t('storage.addModal.qualityLabel')}
                        ariaLabel={t('storage.addModal.qualityLabel')}
                        min={0}
                        value={quality}
                        onChange={setQuality}
                        className="w-56 shrink-0"
                      />
                      <Button size="sm" onPress={addToStaged} isDisabled={!selected} className="shrink-0">
                        <Icon name="plus" />
                        {' '}
                        {t('storage.addModal.add')}
                      </Button>
                    </div>

                    {staged.length > 0 && (
                      <>
                        <div className="flex flex-col gap-1 overflow-y-auto flex-1 min-h-0">
                          {staged.map((item, idx) => (
                            <div
                              key={idx}
                              className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                            >
                              <span className="flex-1 font-mono">{item.template}</span>
                              <NumberInput
                                ariaLabel={`Qty for ${item.template}`}
                                prefix={t('storage.addModal.qtyColLabel')}
                                min={1}
                                value={item.qty}
                                onChange={(v) => updateStaged(idx, 'qty', v)}
                                className="w-56"
                              />
                              <NumberInput
                                ariaLabel={`Quality for ${item.template}`}
                                prefix={t('storage.addModal.qualityColLabel')}
                                min={0}
                                value={item.quality}
                                onChange={(v) => updateStaged(idx, 'quality', v)}
                                className="w-56"
                              />
                              <Button
                                size="sm"
                                variant="danger-soft"
                                onPress={() => removeFromStaged(idx)}
                                aria-label="Remove"
                              >
                                <Icon name="trash" />
                              </Button>
                            </div>
                          ))}
                        </div>
                      </>
                    )}

                    {result && (
                      <div className="text-xs shrink-0 rounded-[var(--radius)] px-3 py-2 bg-surface border border-border">
                        {result.given.length > 0 && (
                          <div className="text-success">
                            ✓ Added:
                            {result.given.join(', ')}
                          </div>
                        )}
                        {result.skipped.map((s, i) => (
                          <div key={i} className="text-danger">
                            ✕ Skipped
                            {s.template}
                            :
                            {s.reason}
                          </div>
                        ))}
                      </div>
                    )}
                  </>
                )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" size="sm" slot="close">{t('common.cancel')}</Button>
            <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
              {submitting ? <Spinner size="sm" color="current" /> : <Icon name="plus" />}
              {t('storage.addModal.add')}
              {' '}
              {staged.length}
              {' '}
              Item
              {staged.length !== 1 ? 's' : ''}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
