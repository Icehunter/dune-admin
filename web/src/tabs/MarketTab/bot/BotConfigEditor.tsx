import { useState } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotConfig } from '../../../api/client'

type Props = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

export default function BotConfigEditor({ config, onSaved }: Props) {
  const [draft, setDraft] = useState<BotConfig>(config)
  const [saving, setSaving] = useState(false)

  const set = <K extends keyof BotConfig>(key: K, val: BotConfig[K]) => {
    setDraft(d => ({ ...d, [key]: val }))
  }

  const setRarity = (key: string, val: number) => {
    setDraft(d => ({ ...d, rarity_multipliers: { ...d.rarity_multipliers, [key]: val } }))
  }

  const setVendor = (key: string, val: number) => {
    setDraft(d => ({ ...d, vendor_multipliers: { ...d.vendor_multipliers, [key]: val } }))
  }

  const setGrade = (idx: number, val: number) => {
    setDraft(d => {
      const arr = [...d.grade_multipliers]
      arr[idx] = val
      return { ...d, grade_multipliers: arr }
    })
  }

  const save = async () => {
    setSaving(true)
    try {
      const saved = await api.marketBot.saveConfig(draft)
      onSaved(saved)
      toast.success('Config saved — changes apply on next tick')
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  const GRADE_LABELS = ['Standard', 'Refined', 'Superior', 'Masterwork', 'Pristine', 'Flawless']

  return (
    <div className="flex flex-col gap-5">
      <Section label="Tick Intervals">
        <div className="grid grid-cols-2 gap-3">
          <Field label="List tick interval" hint="e.g. 30m, 1h">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.list_tick_interval}
              onChange={e => set('list_tick_interval', e.target.value)}
            />
          </Field>
          <Field label="Buy tick interval" hint="e.g. 5m">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.buy_tick_interval}
              onChange={e => set('buy_tick_interval', e.target.value)}
            />
          </Field>
        </div>
      </Section>

      <Section label="Limits">
        <div className="grid grid-cols-3 gap-3">
          <Field label="Max buys per tick">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.max_buys_per_tick}
              onChange={e => set('max_buys_per_tick', Number(e.target.value))}
            />
          </Field>
          <Field label="Listings per grade">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.listings_per_grade}
              onChange={e => set('listings_per_grade', Number(e.target.value))}
            />
          </Field>
          <Field label="Buy threshold" hint="1.05 = 5% below market">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              step="0.01"
              value={draft.buy_threshold}
              onChange={e => set('buy_threshold', Number(e.target.value))}
            />
          </Field>
        </div>
      </Section>

      <Section label="Rarity Multipliers">
        <div className="flex flex-wrap gap-3">
          {Object.entries(draft.rarity_multipliers ?? {}).map(([rarity, mult]) => (
            <Field key={rarity} label={rarity}>
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                type="number" step="0.1" value={mult}
                onChange={e => setRarity(rarity, Number(e.target.value))}
              />
            </Field>
          ))}
        </div>
      </Section>

      {draft.vendor_multipliers && Object.keys(draft.vendor_multipliers ?? {}).length > 0 && (
        <Section label="Vendor Multipliers">
          <div className="flex flex-wrap gap-3">
            {Object.entries(draft.vendor_multipliers ?? {}).map(([rarity, mult]) => (
              <Field key={rarity} label={rarity}>
                <input
                  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                  type="number" step="0.1" value={mult}
                  onChange={e => setVendor(rarity, Number(e.target.value))}
                />
              </Field>
            ))}
          </div>
        </Section>
      )}

      <Section label="Grade Multipliers">
        <div className="flex flex-wrap gap-3">
          {(draft.grade_multipliers ?? []).map((mult, i) => (
            <Field key={i} label={GRADE_LABELS[i] ?? `Grade ${i}`}>
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                type="number" step="0.01" value={mult}
                onChange={e => setGrade(i, Number(e.target.value))}
              />
            </Field>
          ))}
        </div>
      </Section>

      <div className="flex items-center gap-3 pt-1">
        <Button size="sm" onPress={save} isDisabled={saving}>
          {saving ? <Spinner size="sm" color="current" /> : null}
          Save Config
        </Button>
        <Button size="sm" variant="ghost" onPress={() => setDraft(config)}>
          Reset
        </Button>
        <label className="flex items-center gap-2 text-sm cursor-pointer select-none ml-2">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={e => set('enabled', e.target.checked)}
            className="accent-[var(--color-accent)]"
          />
          Ticking enabled
        </label>
      </div>
    </div>
  )
}

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <span className="text-xs font-semibold text-muted uppercase tracking-wider">{label}</span>
      {children}
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <label className="text-xs text-muted">
        {label}
        {hint && <span className="text-muted/60 ml-1">({hint})</span>}
      </label>
      {children}
    </div>
  )
}
