import { useState, useEffect, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, ListBox, SearchField, Select, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { WelcomePackage, WelcomePackageConfig, WelcomePackageItem, WelcomeGrantRecord } from '../api/client'
import { DataTable, Icon, NumberInput, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'

type GrantKey = 'character' | 'fls' | 'version' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

function fmtTime(s: string): string {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export default function WelcomePackageTab() {
  const { t } = useTranslation()

  const GRANT_COLUMNS: Column<GrantKey>[] = [
    { key: 'character', label: t('welcome.columns.character'), minWidth: 130 },
    { key: 'fls', label: t('welcome.columns.flsId'), minWidth: 140 },
    { key: 'version', label: t('welcome.columns.version'), width: 90 },
    { key: 'status', label: t('welcome.columns.status'), width: 90 },
    { key: 'attempts', label: t('welcome.columns.tries'), width: 60 },
    { key: 'updated', label: t('welcome.columns.updated'), minWidth: 150 },
    { key: 'error', label: t('welcome.columns.error'), minWidth: 180 },
    { key: 'actions', label: '', width: 100, sortable: false },
  ]

  const [grants, setGrants] = useState<WelcomeGrantRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)

  const [enabled, setEnabled] = useState(false)
  const [scanSecs, setScanSecs] = useState(30)
  const [packages, setPackages] = useState<WelcomePackage[]>([])
  const [activeVersion, setActiveVersion] = useState('')
  const [selected, setSelected] = useState('') // version currently being edited
  const [newName, setNewName] = useState('')

  const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
  const [addQuery, setAddQuery] = useState('')
  const [addSelected, setAddSelected] = useState('')
  const [addQty, setAddQty] = useState(1)
  const [addQuality, setAddQuality] = useState(0)

  const applyConfig = (c: WelcomePackageConfig) => {
    setEnabled(c.enabled)
    setScanSecs(c.scan_interval_secs)
    setPackages(c.packages ?? [])
    setActiveVersion(c.active_version)
    setSelected(c.active_version || (c.packages?.[0]?.version ?? ''))
  }

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.welcomePackage.config())
      .then(applyConfig)
      .then(() => api.welcomePackage.grants(100))
      .then(setGrants)
      .catch((e: unknown) => {
        const msg = e instanceof Error ? e.message : String(e)
        toast.danger(t('welcome.failedToLoad', { message: msg }))
      })
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    api.players.templates().then(setTemplates).catch(() => {})
  }, [])

  const selectedPkg = packages.find((p) => p.version === selected)
  const items = selectedPkg?.items ?? []

  const setItems = (next: WelcomePackageItem[]) => {
    setPackages((ps) => ps.map((p) => (p.version === selected ? { ...p, items: next } : p)))
  }
  const addFiltered = useMemo(() => {
    if (!addQuery) return []
    const q = addQuery.toLowerCase()
    return templates
      .filter((tpl) => tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, addQuery])

  const pickTemplate = (tpl: { id: string, name: string }) => {
    setAddSelected(tpl.id)
    setAddQuery(tpl.name ? `${tpl.id}  —  ${tpl.name}` : tpl.id)
  }

  const addItem = () => {
    if (!addSelected) return
    setItems([...items, { template: addSelected, qty: addQty, quality: addQuality }])
    setAddQuery('')
    setAddSelected('')
    setAddQty(1)
    setAddQuality(0)
  }
  const removeItem = (i: number) => setItems(items.filter((_, idx) => idx !== i))
  const setItem = (i: number, patch: Partial<WelcomePackageItem>) =>
    setItems(items.map((it, idx) => (idx === i ? { ...it, ...patch } : it)))

  const addVersion = () => {
    const name = newName.trim()
    if (!name) return
    if (packages.some((p) => p.version === name)) {
      toast.danger(t('welcome.versionExists', { name }))
      return
    }
    setPackages([...packages, { version: name, items: [] }])
    setSelected(name)
    setNewName('')
  }

  const deleteVersion = (v: string) => {
    const next = packages.filter((p) => p.version !== v)
    setPackages(next)
    if (activeVersion === v) setActiveVersion('')
    if (selected === v) setSelected(next[0]?.version ?? '')
  }

  const save = async () => {
    setSaving(true)
    try {
      const cfg: WelcomePackageConfig = {
        enabled,
        scan_interval_secs: scanSecs,
        active_version: activeVersion,
        packages,
      }
      applyConfig(await api.welcomePackage.saveConfig(cfg))
      toast.success(enabled
        ? t('welcome.savedEnabled', { version: activeVersion })
        : t('welcome.savedDisabled'))
    }
    catch (e) {
      toast.danger(t('welcome.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    setRunning(true)
    try {
      const r = await api.welcomePackage.run()
      toast.success(t('welcome.scanComplete', { granted: r.granted, failed: r.failed, skipped: r.skipped }))
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(t('welcome.runFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setRunning(false)
    }
  }

  const retry = async (g: WelcomeGrantRecord) => {
    try {
      await api.welcomePackage.retry(g.fls_id, g.package_version, g.account_id)
      toast.success(t('welcome.retryCleared'))
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(t('welcome.retryFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0 overflow-auto pr-1">
      <PageHeader
        title={t('welcome.title')}
        subtitle={t('welcome.subtitle')}
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
      </PageHeader>

      <Panel>
        <SectionLabel>{t('welcome.configuration')}</SectionLabel>

        <label className="flex items-center gap-2 mt-1 cursor-pointer select-none w-fit">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="h-4 w-4 accent-accent"
          />
          <span className="text-sm text-foreground">{t('welcome.enabledLabel')}</span>
        </label>
        <p className="text-xs text-muted mt-1">
          {t('welcome.enabledHint')}
        </p>

        <div className="flex flex-wrap items-center gap-4 mt-3">
          <div className="flex flex-col gap-1">
            <label className="text-xs text-muted">{t('welcome.activeVersion')}</label>
            <Select
              aria-label={t('welcome.activeVersion')}
              selectedKey={activeVersion || null}
              onSelectionChange={(k) => setActiveVersion(k ? String(k) : '')}
              className="w-48"
            >
              <Select.Trigger>
                <Select.Value>{!activeVersion ? '— none —' : activeVersion}</Select.Value>
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  <ListBox.Item key="_none" id="" textValue="— none —">
                    — none —
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                  {packages.map((p) => (
                    <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
                      {p.version}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </div>
          <NumberInput
            label={t('welcome.scanInterval')}
            min={5}
            step={5}
            value={scanSecs}
            onChange={setScanSecs}
            className="w-56"
          />
        </div>
      </Panel>

      <Panel>
        <SectionLabel>{t('welcome.packages')}</SectionLabel>

        <div className="flex flex-col gap-3 mt-1">
          <div className="flex items-end gap-3">
            <Field label={t('welcome.editingVersion')}>
              <Select
                aria-label={t('welcome.editingVersion')}
                selectedKey={selected || null}
                onSelectionChange={(k) => setSelected(k ? String(k) : '')}
                className="w-56"
              >
                <Select.Trigger>
                  <Select.Value>{!selected ? '— select —' : selected + (selected === activeVersion ? ' (active)' : '')}</Select.Value>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    <ListBox.Item key="_none" id="" textValue="— select —">
                      — select —
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    {packages.map((p) => (
                      <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
                        {p.version}
                        {p.version === activeVersion ? ' (active)' : ''}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            </Field>
            {selected && (
              <Button size="sm" variant="ghost" onPress={() => deleteVersion(selected)}>
                <Icon name="trash-2" />
                {' '}
                Delete version
              </Button>
            )}
          </div>

          <div className="flex items-end gap-2">
            <Field label={t('welcome.newVersionLabel')}>
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-40"
                placeholder={t('welcome.newVersionPlaceholder')}
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
              />
            </Field>
            <Button size="sm" variant="outline" onPress={addVersion}>
              <Icon name="plus" />
              {' '}
              {t('welcome.addVersion')}
            </Button>
          </div>
        </div>

        {!selected
          ? (
              <p className="text-xs text-muted mt-3">{t('welcome.noPackageSelected')}</p>
            )
          : (
              <div className="mt-3">
                <div className="text-xs text-muted mb-2">
                  Items in
                  {' '}
                  <span className="text-foreground">{selected}</span>
                  {' '}
                  (
                  {items.length}
                  )
                </div>

                {/* Add row */}
                <div className="flex items-center gap-2 mb-3">
                  <div className="relative flex-1">
                    <SearchField
                      value={addQuery}
                      onChange={(v) => {
                        setAddQuery(v)
                        setAddSelected('')
                      }}
                      className="w-full"
                    >
                      <SearchField.Group>
                        <SearchField.SearchIcon />
                        <SearchField.Input placeholder="Search item templates…" />
                        <SearchField.ClearButton />
                      </SearchField.Group>
                    </SearchField>
                    {addFiltered.length > 0 && (
                      <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                        {addFiltered.map((tpl) => (
                          <div
                            key={tpl.id}
                            className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                            onClick={() => pickTemplate(tpl)}
                          >
                            <span className="font-mono">{tpl.id}</span>
                            {tpl.name
                              ? (
                                  <span className="text-muted">
                                    {' — '}
                                    {tpl.name}
                                  </span>
                                )
                              : null}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                  <NumberInput
                    prefix="Qty"
                    ariaLabel="Qty"
                    min={1}
                    value={addQty}
                    onChange={setAddQty}
                    className="w-56 shrink-0"
                  />
                  <NumberInput
                    prefix="Quality"
                    ariaLabel="Quality"
                    min={0}
                    value={addQuality}
                    onChange={setAddQuality}
                    className="w-56 shrink-0"
                  />
                  <Button size="sm" onPress={addItem} isDisabled={!addSelected} className="shrink-0">
                    <Icon name="plus" />
                    {' '}
                    {t('welcome.addItem')}
                  </Button>
                </div>

                {/* Item list */}
                <div className="flex flex-col gap-2">
                  {items.length === 0 && (
                    <p className="text-xs text-muted">{t('welcome.noItemsYet')}</p>
                  )}
                  {items.map((it, i) => (
                    <div
                      key={i}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                    >
                      <span className="flex-1 min-w-0 truncate font-mono text-foreground">{it.template}</span>
                      <NumberInput
                        ariaLabel="Qty"
                        prefix="Qty"
                        min={1}
                        value={it.qty}
                        onChange={(v) => setItem(i, { qty: v })}
                        className="w-56 shrink-0"
                      />
                      <NumberInput
                        ariaLabel="Quality"
                        prefix="Quality"
                        min={0}
                        value={it.quality}
                        onChange={(v) => setItem(i, { quality: v })}
                        className="w-56 shrink-0"
                      />
                      <Button size="sm" variant="danger-soft" onPress={() => removeItem(i)} aria-label={t('welcome.removeItem')}>
                        <Icon name="x" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            )}

        <div className="flex items-center gap-2 mt-4">
          <Button size="sm" variant="secondary" onPress={save} isDisabled={saving}>
            {saving
              ? (
                  <Spinner size="sm" color="current" />
                )
              : (
                  <>
                    <Icon name="save" />
                    {' '}
                    {t('welcome.saveConfig')}
                  </>
                )}
          </Button>
          <Button size="sm" variant="outline" onPress={runNow} isDisabled={running}>
            {running
              ? (
                  <Spinner size="sm" color="current" />
                )
              : (
                  <>
                    <Icon name="play" />
                    {' '}
                    {t('welcome.runNow')}
                  </>
                )}
          </Button>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>
          {t('welcome.grantsTitle')}
          {' '}
          (
          {grants.length}
          )
        </SectionLabel>
        <DataTable<WelcomeGrantRecord, GrantKey>
          aria-label={t('welcome.grantsLabel')}
          className="mt-1"
          columns={GRANT_COLUMNS}
          rows={grants}
          rowId={(g) => `${g.fls_id}:${g.package_version}:${g.account_id}`}
          initialSort={{ column: 'updated', direction: 'descending' }}
          sortValue={(g, k) => {
            switch (k) {
              case 'character': return g.character_name
              case 'fls': return g.fls_id
              case 'version': return g.package_version
              case 'status': return g.status
              case 'attempts': return g.attempts
              case 'updated': return g.updated_at
              case 'error': return g.last_error
              default: return ''
            }
          }}
          emptyState={<div className="py-8 text-center text-muted">{t('welcome.noGrants')}</div>}
          renderCell={(g, key) => {
            switch (key) {
              case 'character':
                return g.character_name || <span className="text-muted">—</span>
              case 'fls':
                return <span className="font-mono text-xs text-muted">{g.fls_id}</span>
              case 'version':
                return <span className="text-muted text-xs">{g.package_version}</span>
              case 'status':
                return (
                  <span className={g.status === 'failed' ? 'text-danger' : 'text-accent'}>
                    {g.status}
                  </span>
                )
              case 'attempts':
                return <span className="text-muted">{g.attempts}</span>
              case 'updated':
                return <span className="text-muted text-xs">{fmtTime(g.updated_at)}</span>
              case 'error':
                return g.last_error
                  ? <span className="text-danger text-xs">{g.last_error}</span>
                  : <span className="text-muted">—</span>
              case 'actions':
                return g.status === 'failed'
                  ? (
                      <Button size="sm" variant="outline" className="w-full" onPress={() => retry(g)}>
                        <Icon name="refresh-cw" />
                        {' '}
                        {t('welcome.retry')}
                      </Button>
                    )
                  : null
            }
          }}
        />
      </Panel>
    </div>
  )
}

function Field({ label, hint, children }: { label: string, hint?: string, children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <label className="text-xs text-muted">
        {label}
        {hint && (
          <span className="text-muted/60 ml-1">
            (
            {hint}
            )
          </span>
        )}
      </label>
      {children}
    </div>
  )
}
