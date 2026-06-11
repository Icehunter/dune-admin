import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, Switch, ToggleButton, ToggleButtonGroup, toast } from '@heroui/react'
import { api } from '../api/client'
import type { ScheduledRestarts, RestartRule } from '../api/client'
import { Panel, SectionLabel, Icon, NumberInput, TimeInput } from '../dune-ui'
import { TimezoneSelect } from './TimezoneSelect'

const DOW = [0, 1, 2, 3, 4, 5, 6] // Sun..Sat

// ScheduledRestartsCard (#145): configure weekday+time auto-restarts with a
// native in-game countdown warning. Designed as a card to drop into the Server
// Health page (#149); lives on the Battlegroup tab until that lands.
export const ScheduledRestartsCard: React.FC = () => {
  const { t, i18n } = useTranslation()
  const [data, setData] = React.useState<ScheduledRestarts | null>(null)
  const [enabled, setEnabled] = React.useState(false)
  const [timezone, setTimezone] = React.useState('')
  const [warn, setWarn] = React.useState(10)
  const [rules, setRules] = React.useState<RestartRule[]>([])
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)

  const apply = (d: ScheduledRestarts) => {
    setData(d)
    setEnabled(d.enabled)
    setTimezone(d.timezone)
    setWarn(d.warn_minutes || 10)
    setRules(d.rules ?? [])
  }

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.scheduledRestarts.get())
      .then(apply)
      .catch((e: unknown) =>
        toast.danger(t('restarts.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  React.useEffect(() => {
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
  const setRuleDays = (i: number, days: number[]) =>
    setRules((r) => r.map((rule, idx) => (idx === i ? { ...rule, days } : rule)))

  // Localized short weekday label (Jan 1 2023 was a Sunday = day 0).
  const dowLabel = (d: number) =>
    new Intl.DateTimeFormat(i18n.language, { weekday: 'short' }).format(new Date(Date.UTC(2023, 0, 1 + d)))

  return (
    <Panel>
      <div className="flex items-center justify-between mb-2">
        <SectionLabel>{t('restarts.title')}</SectionLabel>
        <Switch isSelected={enabled} onChange={setEnabled} size="sm" className="text-xs text-muted">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{t('restarts.enable')}</Switch.Content>
        </Switch>
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
                  <ToggleButtonGroup
                    selectionMode="multiple"
                    selectedKeys={rule.days.map(String)}
                    onSelectionChange={(keys) => {
                      const days = [...keys].map(Number).sort((a, b) => a - b)
                      setRuleDays(i, days)
                    }}
                    size="sm"
                  >
                    {DOW.map((d) => (
                      <ToggleButton key={d} id={String(d)}>{dowLabel(d)}</ToggleButton>
                    ))}
                  </ToggleButtonGroup>
                  <TimeInput value={rule.time} onChange={(v) => setRuleTime(i, v)} ariaLabel="time" />
                  <Button size="sm" variant="ghost" isIconOnly aria-label={t('restarts.removeRule')} onPress={() => removeRule(i)}>
                    <Icon name="trash" />
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
                  <NumberInput
                    value={warn}
                    onChange={(v) => setWarn(v || 10)}
                    min={1}
                    ariaLabel={t('restarts.warnMinutes')}
                    className="w-16"
                    showButtons={false}
                  />
                </label>
                <label className="flex items-center gap-2 flex-1 min-w-[160px]">
                  {t('restarts.timezone')}
                  <TimezoneSelect value={timezone} onChange={setTimezone} className="flex-1" />
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
