import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, Switch, ToggleButton, ToggleButtonGroup, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { ScheduledBackups, BackupRule } from '../../../api/client'
import { Panel, SectionLabel, Icon, NumberInput, TimeInput } from '../../../dune-ui'
import { dowLabel } from '../../../components/dowLabel'
import { usePermissions } from '../../../hooks/usePermissions'

const DOW = [0, 1, 2, 3, 4, 5, 6] // Sun..Sat

// ── Backup schedule card (self-contained, mirrors ScheduledRestartsCard) ──────
export const ScheduleCard: React.FC = () => {
  const { t, i18n } = useTranslation()
  const { can } = usePermissions()
  const [data, setData] = React.useState<ScheduledBackups | null>(null)
  const [enabled, setEnabled] = React.useState(false)
  const [timezone, setTimezone] = React.useState('')
  const [keepN, setKeepN] = React.useState(0)
  const [rules, setRules] = React.useState<BackupRule[]>([])
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)

  const apply = (d: ScheduledBackups) => {
    setData(d)
    setEnabled(d.enabled)
    setTimezone(d.timezone)
    setKeepN(d.keep_n || 0)
    setRules(d.rules ?? [])
  }

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.scheduledBackups.get())
      .then(apply)
      .catch((e: unknown) =>
        toast.danger(t('backups.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const save = () => {
    setSaving(true)
    api.scheduledBackups.update({ enabled, timezone, rules, keep_n: keepN })
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('backups.schedule.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const addRule = () => setRules((r) => [...r, { days: [...DOW], time: '04:00' }])
  const removeRule = (i: number) => setRules((r) => r.filter((_, idx) => idx !== i))
  const setRuleTime = (i: number, time: string) =>
    setRules((r) => r.map((rule, idx) => (idx === i ? { ...rule, time } : rule)))
  const setRuleDays = (i: number, days: number[]) =>
    setRules((r) => r.map((rule, idx) => (idx === i ? { ...rule, days } : rule)))

  const label = (d: number) => dowLabel(d, i18n.language)

  return (
    <Panel>
      <div className="flex items-center justify-between mb-1">
        <SectionLabel>{t('backups.schedule.title')}</SectionLabel>
        {can('backups:manage') && (
          <Switch isSelected={enabled} onChange={setEnabled} size="sm" className="text-xs text-muted">
            <Switch.Content>
              <Switch.Control><Switch.Thumb /></Switch.Control>
              {t('backups.schedule.enable')}
            </Switch.Content>
          </Switch>
        )}
      </div>
      <p className="text-xs text-muted mb-2">{t('backups.schedule.desc')}</p>

      {loading
        ? <div className="py-3 flex justify-center"><Spinner size="sm" color="current" /></div>
        : (
            <React.Fragment>
              <div className="text-sm mb-2">
                {enabled && data?.next_backup
                  ? (
                      <span className="text-success">
                        {t('backups.schedule.nextBackup', { when: new Date(data.next_backup).toLocaleString() })}
                      </span>
                    )
                  : <span className="text-muted">{t('backups.schedule.noneScheduled')}</span>}
              </div>

              {rules.length === 0 && <div className="text-xs text-muted mb-2">{t('backups.schedule.noRules')}</div>}
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
                      <ToggleButton key={d} id={String(d)}>{label(d)}</ToggleButton>
                    ))}
                  </ToggleButtonGroup>
                  <TimeInput value={rule.time} onChange={(v) => setRuleTime(i, v)} ariaLabel="time" />
                  {can('backups:manage') && (
                    <Button
                      size="sm"
                      variant="ghost"
                      isIconOnly
                      aria-label={t('backups.schedule.removeRule')}
                      onPress={() => removeRule(i)}
                    >
                      <Icon name="trash" />
                    </Button>
                  )}
                </div>
              ))}

              {can('backups:manage') && (
                <Button size="sm" variant="outline" className="mb-3" onPress={addRule}>
                  <Icon name="plus" />
                  {' '}
                  {t('backups.schedule.addRule')}
                </Button>
              )}

              {can('backups:manage') && (
                <div className="flex items-center gap-4 mb-3 text-sm flex-wrap">
                  <label className="flex items-center gap-2">
                    {t('backups.schedule.keepN')}
                    <NumberInput
                      value={keepN}
                      onChange={setKeepN}
                      min={0}
                      ariaLabel={t('backups.schedule.keepN')}
                      className="w-20"
                      showButtons={false}
                    />
                    <span className="text-xs text-muted">{t('backups.schedule.keepHint')}</span>
                  </label>
                  <span className="text-xs text-muted">
                    {t('backups.schedule.timezoneFromServer', 'Timezone is set in server settings.')}
                  </span>
                </div>
              )}

              {can('backups:manage') && (
                <Button size="sm" onPress={save} isDisabled={saving}>
                  {saving ? <Spinner size="sm" color="current" /> : t('backups.schedule.save')}
                </Button>
              )}
            </React.Fragment>
          )}
    </Panel>
  )
}
