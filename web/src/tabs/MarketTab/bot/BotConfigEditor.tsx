import { useState, forwardRef, useImperativeHandle } from 'react'
import { toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'

export type ConfigEditorHandle = {
  save: () => Promise<void>
  reset: () => void
  getEnabled: () => boolean
  setEnabled: (v: boolean) => void
}

type Props = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

function thresholdToPercent(t: number): number {
  return Math.round(t * 100)
}

function percentToThreshold(p: number): number {
  return Math.round(p) / 100
}

const BotConfigEditor = forwardRef<ConfigEditorHandle, Props>(function BotConfigEditor({ config, onSaved }, ref) {
  const [draft, setDraft] = useState<BotConfig>(config)
  const [buyPct, setBuyPct] = useState<number>(thresholdToPercent(config.buy_threshold))

  const set = <K extends keyof BotConfig>(key: K, val: BotConfig[K]) => {
    setDraft((d) => ({ ...d, [key]: val }))
  }

  const setRarity = (key: string, val: number) => {
    setDraft((d) => ({ ...d, rarity_multipliers: { ...d.rarity_multipliers, [key]: val } }))
  }

  const setVendor = (key: string, val: number) => {
    setDraft((d) => ({ ...d, vendor_multipliers: { ...d.vendor_multipliers, [key]: val } }))
  }

  const setGrade = (idx: number, val: number) => {
    setDraft((d) => {
      const arr = [...d.grade_multipliers]
      arr[idx] = val
      return { ...d, grade_multipliers: arr }
    })
  }

  useImperativeHandle(ref, () => ({
    save: async () => {
      const payload: BotConfig = { ...draft, buy_threshold: percentToThreshold(buyPct) }
      const saved = await api.marketBot.saveConfig(payload)
      setBuyPct(thresholdToPercent(saved.buy_threshold))
      onSaved(saved)
      toast.success('Config saved — changes apply on next tick')
    },
    reset: () => {
      setDraft(config)
      setBuyPct(thresholdToPercent(config.buy_threshold))
    },
    getEnabled: () => draft.enabled,
    setEnabled: (v: boolean) => set('enabled', v),
  }), [draft, buyPct, config, onSaved])

  const GRADE_LABELS = ['Standard', 'Refined', 'Superior', 'Masterwork', 'Pristine', 'Flawless']

  return (
    <div className="flex flex-col gap-4 pr-1">

      <Panel>
        <SectionLabel>Tick Intervals</SectionLabel>
        <div className="grid grid-cols-2 gap-3 mt-1">
          <Field label="List tick interval" hint="e.g. 30m, 1h">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.list_interval}
              onChange={(e) => set('list_interval', e.target.value)}
            />
          </Field>
          <Field label="Buy tick interval" hint="e.g. 5m">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.buy_interval}
              onChange={(e) => set('buy_interval', e.target.value)}
            />
          </Field>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>Limits</SectionLabel>
        <p className="text-xs text-muted -mt-1">Controls how many items the bot buys and lists each tick.</p>
        <div className="grid grid-cols-3 gap-3 mt-1">
          <Field label="Max buys per tick">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.max_buys}
              onChange={(e) => set('max_buys', Number(e.target.value))}
            />
          </Field>
          <Field label="Listings per grade" hint="per item per quality level">
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.listings_per_grade}
              onChange={(e) => set('listings_per_grade', Number(e.target.value))}
            />
          </Field>
          <Field label="Buy threshold" hint={`${buyPct}% of bot's reference price`}>
            <div className="flex items-center gap-2">
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-20"
                type="number"
                min={1}
                max={200}
                step={1}
                value={buyPct}
                onChange={(e) => setBuyPct(Number(e.target.value))}
              />
              <span className="text-sm text-muted">%</span>
            </div>
          </Field>
        </div>
        <div className="flex flex-col gap-0.5 mt-1">
          <p className="text-xs text-muted">
            <strong>Buy threshold:</strong>
            {' '}
            buys a listing only when its price is at or below this % of the bot's reference price.
            100% = match or below · 70% = 30%+ discount required · 110% = up to 10% above bot price.
          </p>
          <p className="text-xs text-muted">
            <strong>Listings per grade:</strong>
            {' '}
            active listings maintained per item per quality grade (0 = Standard … 5 = Flawless).
            Stackables use grade 0 only. Example: 5 × 6 grades = up to 30 listings per gradeable item.
          </p>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>Grade Multipliers</SectionLabel>
        <p className="text-xs text-muted -mt-1">Scales the listing price by quality grade. Grade 0 (Standard) is the base and should stay at 1.0.</p>
        <div className="flex flex-wrap gap-3 mt-1">
          {(draft.grade_multipliers ?? []).map((mult, i) => (
            <Field key={i} label={GRADE_LABELS[i] ?? `Grade ${i}`} hint={`×${mult.toFixed(2)}`}>
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                type="number"
                step="0.05"
                min="0.01"
                value={mult}
                onChange={(e) => setGrade(i, Number(e.target.value))}
              />
            </Field>
          ))}
        </div>
      </Panel>

      <Panel>
        <SectionLabel>Rarity Multipliers</SectionLabel>
        <p className="text-xs text-muted -mt-1">Applies to items with no NPC vendor price and to crafted Unique/Memento gear. Keyed by rarity; Common (1.0) is the baseline. Grade multipliers stack on top.</p>
        <div className="flex flex-wrap gap-3 mt-1">
          {Object.entries(draft.rarity_multipliers ?? {}).map(([rarity, mult]) => (
            <Field key={rarity} label={capitalize(rarity)} hint={`×${(mult as number).toFixed(2)}`}>
              <input
                className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                type="number"
                step="0.1"
                min="0.01"
                value={mult}
                onChange={(e) => setRarity(rarity, Number(e.target.value))}
              />
            </Field>
          ))}
        </div>
      </Panel>

      {draft.vendor_multipliers && Object.keys(draft.vendor_multipliers ?? {}).length > 0 && (
        <Panel>
          <SectionLabel>Vendor Multipliers</SectionLabel>
          <p className="text-xs text-muted -mt-1">Applies to items that have an NPC vendor price (most items). Listing price = vendor price × this multiplier, keyed by rarity. Only one of Vendor or Rarity applies to a given item.</p>
          <div className="flex flex-wrap gap-3 mt-1">
            {Object.entries(draft.vendor_multipliers ?? {}).map(([rarity, mult]) => (
              <Field key={rarity} label={capitalize(rarity)} hint={`×${(mult as number).toFixed(2)}`}>
                <input
                  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
                  type="number"
                  step="0.1"
                  min="0.01"
                  value={mult}
                  onChange={(e) => setVendor(rarity, Number(e.target.value))}
                />
              </Field>
            ))}
          </div>
        </Panel>
      )}
    </div>
  )
})

export default BotConfigEditor

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
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
