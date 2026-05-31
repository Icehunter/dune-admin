import { useState, useEffect, useCallback } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { WelcomePackageConfig, WelcomePackageItem, WelcomeGrantRecord } from '../api/client'
import { DataTable, Icon, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'

const INPUT_CLS = 'bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground'

type GrantKey = 'character' | 'fls' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

const GRANT_COLUMNS: Column<GrantKey>[] = [
  { key: 'character', label: 'Character', minWidth: 140 },
  { key: 'fls', label: 'FLS ID', minWidth: 150 },
  { key: 'status', label: 'Status', width: 100 },
  { key: 'attempts', label: 'Tries', width: 70 },
  { key: 'updated', label: 'Updated', minWidth: 160 },
  { key: 'error', label: 'Error', minWidth: 200 },
  { key: 'actions', label: '', width: 110, sortable: false },
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
  const [version, setVersion] = useState('v1')
  const [scanSecs, setScanSecs] = useState(30)
  const [items, setItems] = useState<WelcomePackageItem[]>([])

  const applyConfig = (c: WelcomePackageConfig) => {
    setEnabled(c.enabled)
    setVersion(c.version)
    setScanSecs(c.scan_interval_secs)
    setItems(c.items ?? [])
  }

  const load = useCallback(async () => {
    setLoading(true)
    try {
      applyConfig(await api.welcomePackage.config())
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(`Failed to load welcome package: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const addItem = () => setItems(xs => [...xs, { template: '', qty: 1, quality: 0 }])
  const removeItem = (i: number) => setItems(xs => xs.filter((_, idx) => idx !== i))
  const setItem = (i: number, patch: Partial<WelcomePackageItem>) =>
    setItems(xs => xs.map((it, idx) => (idx === i ? { ...it, ...patch } : it)))

  const save = async () => {
    setSaving(true)
    try {
      const saved = await api.welcomePackage.saveConfig({ enabled, version, scan_interval_secs: scanSecs, items })
      applyConfig(saved)
      toast.success(saved.enabled
        ? 'Welcome package enabled — applies within one scan tick'
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
    <div className="flex flex-col h-full gap-3 min-h-0 overflow-auto">
      <PageHeader
        title="Welcome Package"
        subtitle="Auto-grants a configured item package to every player once, on first login."
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading ? <Spinner size="sm" color="current" /> : (<><Icon name="refresh-cw" /> Refresh</>)}
        </Button>
      </PageHeader>

      <Panel>
        <SectionLabel>Configuration</SectionLabel>

        <label className="flex items-center gap-2 mt-1 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={enabled}
            onChange={e => setEnabled(e.target.checked)}
            className="h-4 w-4 accent-accent"
          />
          <span className="text-sm text-foreground">Enabled</span>
          <span className="text-xs text-muted">
            — grants the package to online players who haven't received this version.
          </span>
        </label>

        <div className="grid grid-cols-2 gap-3 mt-3 max-w-md">
          <Field label="Package version" hint="bump to re-issue to everyone">
            <input className={`${INPUT_CLS} w-full`} value={version} onChange={e => setVersion(e.target.value)} />
          </Field>
          <Field label="Scan interval (seconds)" hint="min 5s">
            <input
              className={`${INPUT_CLS} w-full`}
              type="number"
              min={5}
              value={scanSecs}
              onChange={e => setScanSecs(Number(e.target.value))}
            />
          </Field>
        </div>

        <div className="mt-4">
          <div className="flex items-center justify-between max-w-2xl">
            <span className="text-xs text-muted">
              Items (
              {items.length}
              )
            </span>
            <Button size="sm" variant="outline" onPress={addItem}>
              <Icon name="plus" />
              {' '}
              Add item
            </Button>
          </div>

          <div className="flex flex-col gap-2 mt-2 max-w-2xl">
            {items.length === 0 && (
              <p className="text-xs text-muted">No items yet. Add at least one to enable the package.</p>
            )}
            {items.map((it, i) => (
              // eslint-disable-next-line react/no-array-index-key
              <div key={i} className="flex items-center gap-2">
                <input
                  className={`${INPUT_CLS} flex-1`}
                  placeholder="Item template (e.g. PlantFiber)"
                  value={it.template}
                  onChange={e => setItem(i, { template: e.target.value })}
                />
                <input
                  className={`${INPUT_CLS} w-20`}
                  type="number"
                  min={1}
                  title="Quantity"
                  value={it.qty}
                  onChange={e => setItem(i, { qty: Number(e.target.value) })}
                />
                <input
                  className={`${INPUT_CLS} w-20`}
                  type="number"
                  min={0}
                  title="Quality (0 = base, live RMQ grant)"
                  value={it.quality}
                  onChange={e => setItem(i, { quality: Number(e.target.value) })}
                />
                <Button size="sm" variant="ghost" onPress={() => removeItem(i)} aria-label="Remove item">
                  <Icon name="trash-2" />
                </Button>
              </div>
            ))}
          </div>
        </div>

        <div className="flex items-center gap-2 mt-4">
          <Button size="sm" variant="secondary" onPress={save} isDisabled={saving}>
            {saving ? <Spinner size="sm" color="current" /> : (<><Icon name="save" /> Save</>)}
          </Button>
          <Button size="sm" variant="outline" onPress={runNow} isDisabled={running}>
            {running ? <Spinner size="sm" color="current" /> : (<><Icon name="play" /> Run now</>)}
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
          rowId={g => `${g.fls_id}:${g.package_version}:${g.account_id}`}
          initialSort={{ column: 'updated', direction: 'descending' }}
          sortValue={(g, k) => {
            switch (k) {
              case 'character': return g.character_name
              case 'fls': return g.fls_id
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
        {hint && <span className="text-muted/60 ml-1">{`(${hint})`}</span>}
      </label>
      {children}
    </div>
  )
}
