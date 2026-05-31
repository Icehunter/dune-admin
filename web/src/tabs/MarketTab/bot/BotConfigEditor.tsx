import { useState, forwardRef, useImperativeHandle } from 'react'
import { toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation()
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
      toast.success(t('market.bot.configEditor.configSaved'))
    },
    reset: () => {
      setDraft(config)
      setBuyPct(thresholdToPercent(config.buy_threshold))
    },
    getEnabled: () => draft.enabled,
    setEnabled: (v: boolean) => set('enabled', v),
  }), [draft, buyPct, config, onSaved, t])

  const GRADE_LABELS = [
    t('market.bot.configEditor.gradeStandard'),
    t('market.bot.configEditor.gradeRefined'),
    t('market.bot.configEditor.gradeSuperior'),
    t('market.bot.configEditor.gradeMasterwork'),
    t('market.bot.configEditor.gradePristine'),
    t('market.bot.configEditor.gradeFlawless'),
  ]

  return (
    <div className="flex flex-col gap-4 pr-1">

      <Panel>
        <SectionLabel>{t('market.bot.configEditor.tickIntervals')}</SectionLabel>
        <div className="grid grid-cols-2 gap-3 mt-1">
          <Field label={t('market.bot.configEditor.listTickInterval')} hint={t('market.bot.configEditor.listTickHint')}>
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.list_interval}
              onChange={(e) => set('list_interval', e.target.value)}
            />
          </Field>
          <Field label={t('market.bot.configEditor.buyTickInterval')} hint={t('market.bot.configEditor.buyTickHint')}>
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              value={draft.buy_interval}
              onChange={(e) => set('buy_interval', e.target.value)}
            />
          </Field>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>{t('market.bot.configEditor.limits')}</SectionLabel>
        <p className="text-xs text-muted -mt-1">{t('market.bot.configEditor.limitsDesc')}</p>
        <div className="grid grid-cols-3 gap-3 mt-1">
          <Field label={t('market.bot.configEditor.maxBuysPerTick')}>
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.max_buys}
              onChange={(e) => set('max_buys', Number(e.target.value))}
            />
          </Field>
          <Field label={t('market.bot.configEditor.listingsPerGrade')} hint={t('market.bot.configEditor.listingsPerGradeHint')}>
            <input
              className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
              type="number"
              value={draft.listings_per_grade}
              onChange={(e) => set('listings_per_grade', Number(e.target.value))}
            />
          </Field>
          <Field label={t('market.bot.configEditor.buyThreshold')} hint={t('market.bot.configEditor.buyThresholdHint', { pct: buyPct })}>
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
            <strong>
              {t('market.bot.configEditor.buyThreshold')}
              :
            </strong>
            {' '}
            {t('market.bot.configEditor.buyThresholdDesc')}
          </p>
          <p className="text-xs text-muted">
            <strong>
              {t('market.bot.configEditor.listingsPerGrade')}
              :
            </strong>
            {' '}
            {t('market.bot.configEditor.listingsPerGradeDesc')}
          </p>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>{t('market.bot.configEditor.gradeMultipliers')}</SectionLabel>
        <p className="text-xs text-muted -mt-1">{t('market.bot.configEditor.gradeMultipliersDesc')}</p>
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
        <SectionLabel>{t('market.bot.configEditor.rarityMultipliers')}</SectionLabel>
        <p className="text-xs text-muted -mt-1">{t('market.bot.configEditor.rarityMultipliersDesc')}</p>
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
          <SectionLabel>{t('market.bot.configEditor.vendorMultipliers')}</SectionLabel>
          <p className="text-xs text-muted -mt-1">{t('market.bot.configEditor.vendorMultipliersDesc')}</p>
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
