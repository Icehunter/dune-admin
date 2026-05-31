import { useState, useEffect, useCallback, useMemo } from 'react'
import { Button, ListBox, SearchField, Select, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { WelcomePackage, WelcomePackageConfig, WelcomePackageItem, WelcomeGrantRecord } from '../api/client'
import { DataTable, Icon, NumberInput, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'

type GrantKey = 'character' | 'fls' | 'version' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

const GRANT_COLUMNS: Column<GrantKey>[] = [
  { key: 'character', label: 'Character', minWidth: 130 },
  { key: 'fls', label: 'FLS ID', minWidth: 140 },
  { key: 'version', label: 'Version', width: 90 },
  { key: 'status', label: 'Status', width: 90 },
  { key: 'attempts', label: 'Tries', width: 60 },
  { key: 'updated', label: 'Updated', minWidth: 150 },
  { key: 'error', label: 'Error', minWidth: 180 },
  { key: 'actions', label: '', width: 100, sortable: false },
]

function fmtTime(s: string): string {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export default function WelcomePackageTab() {
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
        toast.danger(`Failed to load welcome package: ${e instanceof Error ? e.message : String(e)}`)
      })
      .finally(() => setLoading(false))
  }, [])

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
      .filter((t) => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q))
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
      toast.danger(`Version "${name}" already exists`)
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
        ? `Enabled — granting "${activeVersion}" within one scan tick`
        : 'Welcome package saved (disabled)')
    }
    catch (e) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    setRunning(true)
    try {
      const r = await api.welcomePackage.run()
      toast.success(`Scan complete — granted ${r.granted}, failed ${r.failed}, skipped ${r.skipped}`)
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(`Run failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setRunning(false)
    }
  }

  const retry = async (g: WelcomeGrantRecord) => {
    try {
      await api.welcomePackage.retry(g.fls_id, g.package_version, g.account_id)
      toast.success('Cleared — will re-attempt on the next scan')
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(`Retry failed: ${e instanceof Error ? e.message : String(e)}`)
    }
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0 overflow-auto pr-1">
      <PageHeader
        title="Welcome Kits"
        subtitle="Auto-grants a configured item package to every player once, on first login."
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
      </PageHeader>

      <Panel>
        <SectionLabel>Configuration</SectionLabel>

        <label className="flex items-center gap-2 mt-1 cursor-pointer select-none w-fit">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="h-4 w-4 accent-accent"
          />
          <span className="text-sm text-foreground">Enabled</span>
        </label>
        <p className="text-xs text-muted mt-1">
          Grants the active package to online players who haven't received that version.
        </p>

        <div className="flex flex-wrap items-end gap-4 mt-3">
          <div className="flex flex-col gap-1">
            <label className="text-xs text-muted">Active version</label>
            <Select
              aria-label="Active version"
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
            label="Scan interval (sec)"
            min={5}
            step={5}
            value={scanSecs}
            onChange={setScanSecs}
            className="w-28"
          />
        </div>
      </Panel>

      <Panel>
        <SectionLabel>Packages</SectionLabel>

        <div className="flex flex-col gap-3 mt-1">
          <div className="flex items-end gap-3">
            <Field label="Editing version">
              <Select
                aria-label="Editing version"
                selectedKey={selected || null}
                onSelectionChange={(k) => setSelected(k ? String(k) : '')}
                className="w-44"
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
            <Field label="New version name">
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-40"
                placeholder="e.g. v2 or season2"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
              />
            </Field>
            <Button size="sm" variant="outline" onPress={addVersion}>
              <Icon name="plus" />
              {' '}
              Add version
            </Button>
          </div>
        </div>

        {!selected
          ? (
              <p className="text-xs text-muted mt-3">No package selected. Add a version to start.</p>
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
                    ariaLabel="Qty"
                    min={1}
                    value={addQty}
                    onChange={setAddQty}
                    className="w-32 shrink-0"
                  />
                  <NumberInput
                    ariaLabel="Quality"
                    min={0}
                    value={addQuality}
                    onChange={setAddQuality}
                    className="w-32 shrink-0"
                  />
                  <Button size="sm" onPress={addItem} isDisabled={!addSelected} className="shrink-0">
                    <Icon name="plus" />
                    {' '}
                    Add
                  </Button>
                </div>

                {/* Item list */}
                <div className="flex flex-col gap-2">
                  {items.length === 0 && (
                    <p className="text-xs text-muted">No items in this version yet.</p>
                  )}
                  {items.map((it, i) => (
                    <div
                      key={i}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                    >
                      <span className="flex-1 min-w-0 truncate font-mono text-foreground">{it.template}</span>
                      <NumberInput
                        ariaLabel="Qty"
                        min={1}
                        value={it.qty}
                        onChange={(v) => setItem(i, { qty: v })}
                        className="w-32 shrink-0"
                      />
                      <NumberInput
                        ariaLabel="Quality"
                        min={0}
                        value={it.quality}
                        onChange={(v) => setItem(i, { quality: v })}
                        className="w-32 shrink-0"
                      />
                      <Button size="sm" variant="danger-soft" onPress={() => removeItem(i)} aria-label="Remove item">
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
                    Save
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
                    Run now
                  </>
                )}
          </Button>
        </div>
      </Panel>

      <Panel className="min-h-0 flex flex-col">
        <SectionLabel>
          Grant Ledger (
          {grants.length}
          )
        </SectionLabel>
        <DataTable<WelcomeGrantRecord, GrantKey>
          aria-label="Welcome package grants"
          className="min-h-0 max-h-full mt-1"
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
          emptyState={<div className="py-8 text-center text-muted">No grants yet.</div>}
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
                        Retry
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
