import * as React from 'react'
import { Button, Modal, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { useAtom, useAtomValue } from 'jotai'
import { Icon } from '../../dune-ui'
import { GlobalSettingsForm } from '../settings/global/GlobalSettingsForm'
import {
  settingsOpenAtom,
  settingsTabAtom,
  updateApplyingAtom,
  updateCheckingAtom,
  updateInfoAtom,
} from '../../atoms/app'
import { useAppUpdate } from './useAppUpdate'
import type { SettingsModalProps } from './interfaces'

export const SettingsModal: React.FC<SettingsModalProps> = ({ status, can, onClose }) => {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useAtom(settingsOpenAtom)
  const settingsTab = useAtomValue(settingsTabAtom)
  const updateInfo = useAtomValue(updateInfoAtom)
  const updateChecking = useAtomValue(updateCheckingAtom)
  const updateApplying = useAtomValue(updateApplyingAtom)
  const { checkUpdate, applyUpdate } = useAppUpdate()

  const [formSaving, setFormSaving] = React.useState(false)
  const formSaveRef = React.useRef<(() => Promise<void>) | null>(null)

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={settingsOpen} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10 dialog-surface-alt">
          <Modal.CloseTrigger />
          <Modal.Header>
            <div className="flex items-baseline gap-6 flex-wrap">
              <Modal.Heading className="text-accent">{t('app.settings')}</Modal.Heading>
              {status && (
                <div className="flex items-center gap-4 text-xs text-muted">
                  {status.version && (
                    <span className="font-mono">
                      v
                      {status.version}
                    </span>
                  )}
                  {/* Control plane is per-server — it belongs on Manage server,
                      not the global dune-admin Settings modal. */}
                  {status.commit && status.commit !== 'unknown' && (
                    <span className="font-mono opacity-60">{status.commit}</span>
                  )}
                </div>
              )}
            </div>
          </Modal.Header>

          {/* Body scrolls; form fills it with its own internal tab scroll */}
          <Modal.Body className="flex flex-col overflow-y-auto h-[80vh] min-h-0 pr-1">
            {settingsOpen && (
              <GlobalSettingsForm
                saveRef={formSaveRef}
                onSavingChange={setFormSaving}
                initialTab={settingsTab}
              />
            )}
          </Modal.Body>

          <Modal.Footer className="flex items-center gap-2">
            {/* Left: update controls — fixed positions so buttons don't shift */}
            <Button
              size="sm"
              variant="ghost"
              onPress={checkUpdate}
              isDisabled={updateChecking || updateApplying}
            >
              {updateChecking
                ? (
                    <React.Fragment>
                      <Spinner size="sm" color="current" />
                      {' '}
                      {t('common.checking')}
                    </React.Fragment>
                  )
                : t('app.checkUpdates')}
            </Button>
            {can('server:control') && updateInfo && !updateInfo.needs_update && (
              <Button
                size="sm"
                variant="ghost"
                onPress={() => applyUpdate(true)}
                isDisabled={updateApplying}
              >
                {t('app.reinstall')}
              </Button>
            )}
            {can('server:control') && updateInfo?.needs_update && (
              <Button size="sm" onPress={() => applyUpdate()} isDisabled={updateApplying}>
                <span className="font-mono text-xs">
                  v
                  {updateInfo.current}
                  {' → '}
                  v
                  {updateInfo.latest.replace(/^v/, '')}
                </span>
              </Button>
            )}

            {/* Spacer */}
            <span className="flex-1" />

            {/* Right: save + close */}
            <span className="text-xs text-muted">{t('app.changesNote')}</span>
            <Button
              size="sm"
              onPress={() => {
                // Save & apply, then close. Don't block on slow background work
                // (e.g. the Discord bot can take ~10s to connect). An auth
                // toggle reloads the page inside save(), so this resolves only
                // for non-toggle saves — close the modal then.
                void formSaveRef.current?.().then(() => onClose())
              }}
              isDisabled={formSaving}
            >
              {formSaving
                ? (
                    <React.Fragment>
                      <Spinner size="sm" color="current" />
                      {' '}
                      {t('common.saving')}
                    </React.Fragment>
                  )
                : (
                    <React.Fragment>
                      <Icon name="save" />
                      {' '}
                      {t('app.saveApply')}
                    </React.Fragment>
                  )}
            </Button>
            <Button
              size="sm"
              variant="tertiary"
              onPress={() => setSettingsOpen(false)}
            >
              {t('common.close')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
