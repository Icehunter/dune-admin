import * as React from 'react'
import { Button, Description, Spinner, Switch } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BattlepassConfig } from '../../../api/client'
import { usePermissions } from '../../../hooks/usePermissions'
import { Icon, NumberInput, PageHeader, Panel, SectionLabel } from '../../../dune-ui'
import { ResetClaimsPanel } from './ResetClaimsPanel'

const DEFAULTS: BattlepassConfig = {
  battlepass_enabled: false,
  battlepass_award_past: false,
  battlepass_auto_grant: false,
  battlepass_poll_seconds: 60,
  battlepass_scan_pace_ms: 75,
  battlepass_scan_start_delay_ms: 3000,
}

export const ConfigView: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const [cfg, setCfg] = React.useState<BattlepassConfig>(DEFAULTS)
  const [loading, setLoading] = React.useState(false)
  const [saving, setSaving] = React.useState(false)

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.battlepass.config())
      .then(setCfg)
      .catch((e: unknown) => {
        toast.danger(t('battlepass.config.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const save = () => {
    Promise.resolve()
      .then(() => setSaving(true))
      .then(() => api.battlepass.saveConfig(cfg))
      .then(setCfg)
      .then(() => { toast.success(t('battlepass.config.saved')) })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.config.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setSaving(false))
  }

  const set = <K extends keyof BattlepassConfig>(key: K) =>
    (value: BattlepassConfig[K]) => setCfg((prev) => ({ ...prev, [key]: value }))

  const enabledBool = cfg.battlepass_enabled ?? false
  const awardPastBool = cfg.battlepass_award_past ?? false
  const autoGrantBool = cfg.battlepass_auto_grant ?? false

  return (
    <div className="flex flex-col h-full min-h-0 gap-3">
      <PageHeader
        title={t('battlepass.sections.config')}
        subtitle={t('battlepass.config.subtitle')}
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <React.Fragment>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </React.Fragment>
              )}
        </Button>
      </PageHeader>

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-3 pr-1">
        <Panel>
          <SectionLabel>{t('battlepass.config.engineSection')}</SectionLabel>

          <Switch
            isSelected={enabledBool}
            onChange={(v) => set('battlepass_enabled')(v)}
            size="sm"
          >
            <Switch.Content>
              <Switch.Control><Switch.Thumb /></Switch.Control>
              {t('battlepass.config.enabled')}
            </Switch.Content>
            <Description className="mb-3">{t('battlepass.config.enabledHint')}</Description>
          </Switch>

          <Switch
            isSelected={awardPastBool}
            onChange={(v) => set('battlepass_award_past')(v)}
            size="sm"
          >
            <Switch.Content>
              <Switch.Control><Switch.Thumb /></Switch.Control>
              {t('battlepass.config.awardPast')}
            </Switch.Content>
            <Description className="mb-3">{t('battlepass.config.awardPastHint')}</Description>
          </Switch>

          <Switch
            isSelected={autoGrantBool}
            onChange={(v) => set('battlepass_auto_grant')(v)}
            size="sm"
          >
            <Switch.Content>
              <Switch.Control><Switch.Thumb /></Switch.Control>
              {t('battlepass.config.autoGrant')}
            </Switch.Content>
            <Description>{t('battlepass.config.autoGrantHint')}</Description>
          </Switch>
        </Panel>

        <Panel>
          <SectionLabel>{t('battlepass.config.timingSection')}</SectionLabel>

          <div className="flex flex-wrap gap-4">
            <NumberInput
              label={t('battlepass.config.pollSeconds')}
              min={10}
              max={600}
              step={10}
              value={cfg.battlepass_poll_seconds}
              onChange={set('battlepass_poll_seconds')}
              className="w-56"
            />
            <NumberInput
              label={t('battlepass.config.scanPaceMs')}
              min={0}
              max={5000}
              step={25}
              value={cfg.battlepass_scan_pace_ms}
              onChange={set('battlepass_scan_pace_ms')}
              className="w-56"
            />
            <NumberInput
              label={t('battlepass.config.scanStartDelayMs')}
              min={0}
              max={30000}
              step={500}
              value={cfg.battlepass_scan_start_delay_ms}
              onChange={set('battlepass_scan_start_delay_ms')}
              className="w-56"
            />
          </div>
          <p className="text-xs text-muted mt-2">
            {t('battlepass.config.timingHint')}
          </p>
        </Panel>

        {can('battlepass:manage') && <ResetClaimsPanel />}
      </div>

      {can('battlepass:manage') && (
        <div className="flex items-center gap-3 shrink-0">
          <Button size="sm" variant="secondary" onPress={save} isDisabled={saving || loading}>
            {saving
              ? <Spinner size="sm" color="current" />
              : (
                  <React.Fragment>
                    <Icon name="save" />
                    {' '}
                    {t('battlepass.config.save')}
                  </React.Fragment>
                )}
          </Button>
        </div>
      )}
    </div>
  )
}
