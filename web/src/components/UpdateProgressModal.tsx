import * as React from 'react'
import { Button, Modal, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'

export type UpdatePhase
  = 'downloading'
    | 'verifying'
    | 'extracting'
    | 'restarting'
    | 'waiting'
    | 'waitingLong'
    | 'ready'
    | 'error'

interface Props {
  isOpen: boolean
  phase: UpdatePhase
  errorMessage?: string
  onDismiss?: () => void
}

const PHASE_ORDER: UpdatePhase[] = [
  'downloading',
  'verifying',
  'extracting',
  'restarting',
  'waiting',
  'ready',
]

const phaseIndex = (phase: UpdatePhase): number => {
  if (phase === 'waitingLong') return PHASE_ORDER.indexOf('waiting')
  if (phase === 'error') return -1
  return PHASE_ORDER.indexOf(phase)
}

export const UpdateProgressModal: React.FC<Props> = ({ isOpen, phase, errorMessage, onDismiss }) => {
  const { t } = useTranslation()
  const isError = phase === 'error'
  const currentIndex = phaseIndex(phase)

  return (
    <Modal.Backdrop
      variant="blur"
      className="bg-linear-to-t from-(--background)/90 via-(--background)/50 to-transparent"
      isOpen={isOpen}
      isDismissable={false}
      onOpenChange={() => {}}
    >
      <Modal.Container size="sm">
        <Modal.Dialog className="p-10">
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {t('app.updateProgress.title')}
            </Modal.Heading>
          </Modal.Header>

          <Modal.Body className="flex flex-col gap-4">
            {/* Phase step list */}
            <div className="flex flex-col gap-2">
              {PHASE_ORDER.map((step, i) => {
                const isActive = !isError && (
                  step === phase
                  || (step === 'waiting' && phase === 'waitingLong')
                )
                const isDoneStep = !isError && i < currentIndex
                const isPending = !isError && i > currentIndex

                return (
                  <div
                    key={step}
                    className={[
                      'flex items-center gap-2.5 text-sm transition-opacity',
                      isPending ? 'opacity-30' : '',
                      isDoneStep ? 'text-success' : '',
                      isActive ? 'text-foreground font-medium' : '',
                    ].filter(Boolean).join(' ')}
                  >
                    <span className="w-4 h-4 flex items-center justify-center shrink-0">
                      {isDoneStep && <Icon name="check" className="size-4 text-success" />}
                      {isActive && <Spinner size="sm" color="current" className="size-4" />}
                      {isPending && <span className="w-1.5 h-1.5 rounded-full bg-current mx-auto" />}
                    </span>
                    <span>{t(`app.updateProgress.${step}`)}</span>
                  </div>
                )
              })}
            </div>

            {/* Error message */}
            {isError && (
              <div className="rounded-md bg-danger/10 border border-danger/30 px-3 py-2.5 text-sm text-danger">
                {errorMessage ?? t('app.updateProgress.error')}
              </div>
            )}

            {/* Long-wait note */}
            {phase === 'waitingLong' && (
              <p className="text-xs text-muted">
                {t('app.updateProgress.waitingLong')}
              </p>
            )}
          </Modal.Body>

          {onDismiss && (
            <Modal.Footer className="flex justify-end">
              <Button
                size="sm"
                variant="ghost"
                onPress={onDismiss}
                isDisabled={!isError && phase !== 'waitingLong' && phase !== 'ready'}
              >
                {t('app.updateProgress.dismiss')}
              </Button>
            </Modal.Footer>
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
