import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { ScheduledRestarts, RestartRule } from '../api/client'
import { Panel, SectionLabel, Icon } from '../dune-ui'

const DOW = [0, 1, 2, 3, 4, 5, 6] // Sun..Sat

// ScheduledRestartsCard (#145): configure weekday+time auto-restarts with a
// native in-game countdown warning. Designed as a card to drop into the Server
// Health page (#149); lives on the Battlegroup tab until that lands.
export const ScheduledRestartsCard: React.FC = () => {
  const { t, i18n } = useTranslation()
  const [data, setData] = useState<ScheduledRestarts | null>(null)
  const [enabled, setEnabled] = useState(false)
  const [timezone, setTimezone] = useState('')
  const [warn, setWarn] = useState(10)
  const [rules, setRules] = useState<RestartRule[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const apply = (d: ScheduledRestarts) => {
    setData(d)
    setEnabled(d.enabled)
    setTimezone(d.timezone)
    setWarn(d.warn_minutes || 10)
    setRules(d.rules ?? [])
  }

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.scheduledRestarts.get())
      .then(apply)
      .catch((e: unknown) =>
        toast.danger(t('restarts.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const save = () => {
    setSaving(true)
    api.scheduledRestarts.update({ enabled, timezone, rules, warn_minutes: warn })
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('restarts.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const skip = () => {
    api.scheduledRestarts.skipNext()
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('restarts.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
  }

  const addRule = () => setRules((r) => [...r, { days: [...DOW], time: '04:00' }])
  const removeRule = (i: number) => setRules((r) => r.filter((_, idx) => idx !== i))
  const setRuleTime = (i: number, time: string) =>
    setRules((r) => r.map((rule, idx) => (idx === i ? { ...rule, time } : rule)))
  const toggleDay = (i: number, d: number) =>
    setRules((r) => r.map((rule, idx) => {
      if (idx !== i) return rule
      const days = rule.days.includes(d) ? rule.days.filter((x) => x !== d) : [...rule.days, d].sort((a, b) => a - b)
      return { ...rule, days }
    }))

  // Localized short weekday label (Jan 1 2023 was a Sunday = day 0).
  const dowLabel = (d: number) =>
    new Intl.DateTimeFormat(i18n.language, { weekday: 'short' }).format(new Date(Date.UTC(2023, 0, 1 + d)))

  const inputCls = 'bg-surface text-foreground border border-border rounded px-2 py-1 text-sm'

  return (
    <Panel>
      <div className="flex items-center justify-between mb-2">
        <SectionLabel>{t('restarts.title')}</SectionLabel>
        <label className="flex items-center gap-2 text-xs text-muted cursor-pointer">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          {t('restarts.enable')}
        </label>
      </div>

      {loading
        ? <div className="py-4 flex justify-center"><Spinner size="sm" color="current" /></div>
        : (
            <>
              <div className="text-sm mb-3">
                {enabled && data?.next_restart
                  ? (
                      <span className="text-success">
                        {t('restarts.nextRestart', { when: new Date(data.next_restart).toLocaleString() })}
                      </span>
                    )
                  : <span className="text-muted">{t('restarts.noneScheduled')}</span>}
              </div>

              {rules.length === 0 && <div className="text-xs text-muted mb-2">{t('restarts.noRules')}</div>}
              {rules.map((rule, i) => (
                <div key={i} className="flex items-center gap-2 mb-2 flex-wrap">
                  <div className="flex gap-1">
                    {DOW.map((d) => (
                      <button
                        key={d}
                        type="button"
                        onClick={() => toggleDay(i, d)}
                        className={`h-7 px-1.5 rounded text-xs transition-colors ${
                          rule.days.includes(d)
                            ? 'bg-accent text-accent-foreground'
                            : 'bg-surface-secondary text-muted hover:text-foreground'
                        }`}
                      >
                        {dowLabel(d)}
                      </button>
                    ))}
                  </div>
                  <input type="time" value={rule.time} onChange={(e) => setRuleTime(i, e.target.value)} className={inputCls} />
                  <Button size="sm" variant="ghost" isIconOnly aria-label={t('restarts.removeRule')} onPress={() => removeRule(i)}>
                    <Icon name="x" />
                  </Button>
                </div>
              ))}

              <Button size="sm" variant="outline" className="mb-3" onPress={addRule}>
                <Icon name="plus" />
                {' '}
                {t('restarts.addRule')}
              </Button>

              <div className="flex items-center gap-4 mb-3 text-sm flex-wrap">
                <label className="flex items-center gap-2">
                  {t('restarts.warnMinutes')}
                  <input
                    type="number"
                    min={1}
                    value={warn}
                    onChange={(e) => setWarn(Number(e.target.value) || 10)}
                    className={`${inputCls} w-16`}
                  />
                </label>
                <label className="flex items-center gap-2 flex-1 min-w-[160px]">
                  {t('restarts.timezone')}
                  <input
                    value={timezone}
                    placeholder={t('restarts.tzPlaceholder')}
                    onChange={(e) => setTimezone(e.target.value)}
                    className={`${inputCls} flex-1`}
                  />
                </label>
              </div>

              <div className="flex gap-2">
                <Button size="sm" onPress={save} isDisabled={saving}>
                  {saving ? <Spinner size="sm" color="current" /> : t('restarts.save')}
                </Button>
                <Button size="sm" variant="outline" onPress={skip} isDisabled={!enabled || !data?.next_restart}>
                  {t('restarts.skipNext')}
                </Button>
              </div>
            </>
          )}
    </Panel>
  )
}
