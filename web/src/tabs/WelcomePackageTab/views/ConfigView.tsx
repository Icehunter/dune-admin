import type React from 'react'
import { useState } from 'react'
import { Button, Input, ListBox, Spinner, Switch, TextArea } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog, Icon, NumberInput, PageHeader, Panel, SectionLabel } from '../../../dune-ui'
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
  | 'motdEnabled' | 'setMotdEnabled'
  | 'motdMessage' | 'setMotdMessage'
  | 'motdSourcePlayer' | 'setMotdSourcePlayer'
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
  motdEnabled, setMotdEnabled,
  motdMessage, setMotdMessage,
  motdSourcePlayer, setMotdSourcePlayer,
  save, saving,
  runNow, running,
  load, loading,
  configDiff,
}) => {
  const { t } = useTranslation()
  const [confirmRun, setConfirmRun] = useState(false)

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

      {/* Scrollable middle: active versions + message panels */}
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-3 pr-1">
        {/* Active versions */}
        <div className="flex flex-col gap-1">
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
                  className="max-h-48 overflow-y-auto rounded-[var(--radius)] border border-border"
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

        {/* Welcome message panel */}
        <Panel>
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
              <TextArea
                aria-label={t('welcome.message.messageLabel')}
                fullWidth
                rows={3}
                placeholder={t('welcome.message.messagePlaceholder')}
                value={welcomeMessage}
                disabled={!welcomeMessageEnabled}
                onChange={(e) => setWelcomeMessage(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1 max-w-md">
              <span className="text-xs text-muted">{t('welcome.message.senderLabel')}</span>
              <Input
                aria-label={t('welcome.message.senderLabel')}
                className="w-full"
                placeholder={t('welcome.message.senderPlaceholder')}
                value={welcomeWhisperSourcePlayer}
                disabled={!welcomeMessageEnabled}
                onChange={(e) => setWelcomeWhisperSourcePlayer(e.target.value)}
              />
            </div>
          </div>
        </Panel>

        {/* MOTD panel */}
        <Panel>
          <SectionLabel>{t('welcome.motd.title')}</SectionLabel>

          <Switch isSelected={motdEnabled} onChange={setMotdEnabled} size="sm">
            <Switch.Control><Switch.Thumb /></Switch.Control>
            <Switch.Content>{t('welcome.motd.enabledLabel')}</Switch.Content>
          </Switch>
          <p className="text-xs text-muted mt-1 mb-3">
            {t('welcome.motd.enabledHint')}
          </p>

          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted">{t('welcome.motd.messageLabel')}</span>
              <TextArea
                aria-label={t('welcome.motd.messageLabel')}
                fullWidth
                rows={3}
                placeholder={t('welcome.motd.messagePlaceholder')}
                value={motdMessage}
                disabled={!motdEnabled}
                onChange={(e) => setMotdMessage(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1 max-w-md">
              <span className="text-xs text-muted">{t('welcome.motd.senderLabel')}</span>
              <Input
                aria-label={t('welcome.motd.senderLabel')}
                className="w-full"
                placeholder={t('welcome.motd.senderPlaceholder')}
                value={motdSourcePlayer}
                disabled={!motdEnabled}
                onChange={(e) => setMotdSourcePlayer(e.target.value)}
              />
            </div>
          </div>
        </Panel>
      </div>

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
        <Button size="sm" variant="outline" onPress={() => setConfirmRun(true)} isDisabled={running}>
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

      {/* Confirm exactly which package(s) will be granted before running, so an
          accidentally-selected version isn't granted by surprise (#162). */}
      <ConfirmDialog
        open={confirmRun}
        title={t('welcome.runConfirmTitle')}
        description={t('welcome.runConfirmBody', {
          versions: activeVersions.length ? activeVersions.join(', ') : t('welcome.noPackageSelected'),
        })}
        confirmLabel={t('welcome.runNow')}
        onConfirm={() => {
          setConfirmRun(false)
          void runNow()
        }}
        onCancel={() => setConfirmRun(false)}
      />
    </div>
  )
}
