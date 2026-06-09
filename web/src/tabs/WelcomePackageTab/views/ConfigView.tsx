import type React from 'react'
import { Button, ListBox, Spinner, Switch } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon, NumberInput, PageHeader, Panel, SectionLabel } from '../../../dune-ui'
import type { WelcomeSharedProps } from '../types'
import { DiffStatus } from '../components/DiffStatus'

type ConfigViewProps = Pick<
  WelcomeSharedProps,
  | 'enabled' | 'setEnabled'
  | 'scanSecs' | 'setScanSecs'
  | 'packages'
  | 'activeVersions' | 'setActiveVersions'
  | 'welcomeMessageEnabled' | 'setWelcomeMessageEnabled'
  | 'welcomeMessage' | 'setWelcomeMessage'
  | 'welcomeWhisperSourcePlayer' | 'setWelcomeWhisperSourcePlayer'
  | 'save' | 'saving'
  | 'runNow' | 'running'
  | 'load' | 'loading'
  | 'configDiff'
>

export const ConfigView: React.FC<ConfigViewProps> = ({
  enabled, setEnabled,
  scanSecs, setScanSecs,
  packages,
  activeVersions, setActiveVersions,
  welcomeMessageEnabled, setWelcomeMessageEnabled,
  welcomeMessage, setWelcomeMessage,
  welcomeWhisperSourcePlayer, setWelcomeWhisperSourcePlayer,
  save, saving,
  runNow, running,
  load, loading,
  configDiff,
}) => {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col h-full min-h-0 gap-3">
      {/* Header */}
      <PageHeader title={t('welcome.sections.config')} subtitle={t('welcome.configSubtitle')}>
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      {/* Unsaved changes banner */}
      {configDiff.isDirty && (
        <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-warning/10 border border-warning/40 text-warning flex items-center gap-2">
          <Icon name="triangle-alert" />
          <span>You have unsaved changes — click Save Config to persist them.</span>
        </div>
      )}

      {/* Compact one-liner: enabled toggle + scan interval */}
      <div className="flex items-center gap-6 shrink-0">
        <Switch isSelected={enabled} onChange={setEnabled} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{t('welcome.enabledLabel')}</Switch.Content>
        </Switch>
        <span className="text-xs text-muted">{t('welcome.enabledHint')}</span>
        <NumberInput
          label={t('welcome.scanInterval')}
          min={5}
          step={5}
          value={scanSecs}
          onChange={setScanSecs}
          className="w-56 ml-auto"
        />
      </div>

      {/* Active versions — flex-1 fills remaining space */}
      <div className="flex flex-col flex-1 min-h-0 gap-1">
        <SectionLabel>{t('welcome.activeVersionGranted')}</SectionLabel>
        {packages.length === 0
          ? <p className="text-xs text-muted mt-1">{t('welcome.noPackageSelected')}</p>
          : (
              <ListBox
                aria-label={t('welcome.activeVersionGranted')}
                selectionMode="multiple"
                selectedKeys={new Set(activeVersions)}
                onSelectionChange={(keys) => {
                  setActiveVersions(Array.from(keys).map(String))
                }}
                className="flex-1 min-h-0 overflow-y-auto rounded-[var(--radius)] border border-border"
              >
                {packages.map((p) => (
                  <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
                    {p.version}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                ))}
              </ListBox>
            )}
      </div>

      {/* Welcome message panel — fixed height */}
      <Panel className="shrink-0">
        <SectionLabel>{t('welcome.message.title')}</SectionLabel>

        <Switch isSelected={welcomeMessageEnabled} onChange={setWelcomeMessageEnabled} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{t('welcome.message.enabledLabel')}</Switch.Content>
        </Switch>
        <p className="text-xs text-muted mt-1 mb-3">
          {t('welcome.message.enabledHint')}
        </p>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted">{t('welcome.message.messageLabel')}</span>
            <textarea
              className="w-full rounded-[var(--radius)] border border-border bg-surface text-foreground text-sm px-3 py-2 resize-none focus:outline-none focus:border-accent disabled:opacity-50"
              rows={3}
              placeholder={t('welcome.message.messagePlaceholder')}
              value={welcomeMessage}
              disabled={!welcomeMessageEnabled}
              onChange={(e) => setWelcomeMessage(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1 max-w-md">
            <span className="text-xs text-muted">{t('welcome.message.senderLabel')}</span>
            <input
              className="w-full rounded-[var(--radius)] border border-border bg-surface text-foreground text-sm px-3 py-2 focus:outline-none focus:border-accent disabled:opacity-50"
              placeholder={t('welcome.message.senderPlaceholder')}
              value={welcomeWhisperSourcePlayer}
              disabled={!welcomeMessageEnabled}
              onChange={(e) => setWelcomeWhisperSourcePlayer(e.target.value)}
            />
          </div>
        </div>
      </Panel>

      {/* Action bar — fixed at bottom */}
      <div className="flex items-center gap-3 shrink-0">
        <Button size="sm" variant="secondary" onPress={save} isDisabled={saving}>
          {saving
            ? <Spinner size="sm" color="current" />
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
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="play" />
                  {' '}
                  {t('welcome.runNow')}
                </>
              )}
        </Button>
        <DiffStatus diff={configDiff} />
      </div>
    </div>
  )
}
