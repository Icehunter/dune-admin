import * as React from 'react'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'
import { api } from '../api/client'
import type { RestoreStatus, RestoreStep } from '../api/client'
import type { RestoreProgressModalProps } from './interfaces'

const POLL_MS = 1500

// runBattlegroupCommand posts a lifecycle command through the battlegroup API.
const runBattlegroupCommand = api.battlegroup.exec

// RestoreProgressModal renders the database-restore step dialog. Unlike the
// UpdateProgressModal it visually mirrors, every step state here is REAL —
// polled from the backend restore job (GET /db-backups/restore/status), not
// advanced on client-side timers. Stopping shards can take up to a minute and
// pg_restore minutes more; the checkmarks reflect what actually happened.
export const RestoreProgressModal: React.FC<RestoreProgressModalProps> = ({ open, onClose }) => {
  const { t } = useTranslation()
  const [status, setStatus] = React.useState<RestoreStatus | null>(null)
  const [starting, setStarting] = React.useState(false)

  React.useEffect(() => {
    if (!open) {
      void Promise.resolve().then(() => setStatus(null))
      return
    }
    let cancelled = false
    let id: ReturnType<typeof setInterval> | undefined
    const tick = (): void => {
      api.dbBackups.restoreStatus()
        .then((s) => {
          if (cancelled) return
          setStatus(s)
          // Once the job reaches a terminal state there is nothing left to
          // poll — stop hitting the endpoint while the result dialog is read.
          if (s.done && id !== undefined) {
            clearInterval(id)
            id = undefined
          }
        })
        .catch(() => {})
    }
    tick()
    id = setInterval(tick, POLL_MS)
    return () => {
      cancelled = true
      if (id !== undefined) clearInterval(id)
    }
  }, [open])

  const done = status?.done ?? false
  const failed = status?.failed ?? false

  const handleStartBattlegroup = (): void => {
    setStarting(true)
    runBattlegroupCommand('start')
      .then(() => {
        toast.success(t('backups.restoreProgress.battlegroupStarted'))
        onClose()
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setStarting(false))
  }

  const renderStepIcon = (step: RestoreStep): React.ReactNode => {
    if (step.status === 'done') return <Icon name="check" className="size-4 text-success" />
    if (step.status === 'skipped') return <Icon name="check" className="size-4 text-muted" />
    if (step.status === 'failed') return <Icon name="x" className="size-4 text-danger" />
    if (step.status === 'running') return <Spinner size="sm" color="current" className="size-4" />
    return <span className="w-1.5 h-1.5 rounded-full bg-current mx-auto" />
  }

  const stepClass = (step: RestoreStep): string => {
    const classes = ['flex items-center gap-2.5 text-sm transition-opacity']
    if (step.status === 'pending') classes.push('opacity-30')
    if (step.status === 'done') classes.push('text-success')
    if (step.status === 'skipped') classes.push('text-muted')
    if (step.status === 'failed') classes.push('text-danger font-medium')
    if (step.status === 'running') classes.push('text-foreground font-medium')
    return classes.join(' ')
  }

  const renderStepLabel = (step: RestoreStep): string => {
    if (step.key === 'check') return t('backups.restoreProgress.check')
    if (step.key === 'stop') {
      return step.status === 'skipped'
        ? t('backups.restoreProgress.stopSkipped')
        : t('backups.restoreProgress.stop')
    }
    if (step.key === 'restore') return t('backups.restoreProgress.restore')
    if (step.key === 'finalize') return t('backups.restoreProgress.finalize')
    return step.key
  }

  const renderSteps = (): React.ReactNode => {
    if (!status) {
      return (
        <div className="py-3 flex justify-center">
          <Spinner size="sm" color="current" />
        </div>
      )
    }
    return (
      <div className="flex flex-col gap-2">
        {status.steps.map((step) => (
          <div key={step.key} className={stepClass(step)}>
            <span className="w-4 h-4 flex items-center justify-center shrink-0">
              {renderStepIcon(step)}
            </span>
            <span>{renderStepLabel(step)}</span>
          </div>
        ))}
      </div>
    )
  }

  const renderIgnoredNote = (): React.ReactNode => {
    if (!status || status.ignored_errors === 0) return null
    return <p className="text-xs text-muted">{t('backups.restoreIgnoredNote', { n: status.ignored_errors })}</p>
  }

  const renderStoppedNote = (): React.ReactNode => {
    if (!status?.servers_stopped) return null
    return <p className="text-xs text-muted">{t('backups.restoreProgress.leftStopped')}</p>
  }

  const renderSuccess = (): React.ReactNode => {
    if (!status || !done || failed) return null
    return (
      <div className="flex flex-col gap-2">
        <div className="rounded-md bg-success/10 border border-success/30 px-3 py-2.5 text-sm text-success">
          {t('backups.restoreProgress.success')}
        </div>
        {renderIgnoredNote()}
        {renderStoppedNote()}
      </div>
    )
  }

  const renderFailure = (): React.ReactNode => {
    if (!status || !failed) return null
    return (
      <div className="rounded-md bg-danger/10 border border-danger/30 px-3 py-2.5 text-sm text-danger break-words">
        {status.error || t('backups.restoreProgress.failure')}
      </div>
    )
  }

  const renderStartButton = (): React.ReactNode => {
    if (!done || failed || !status?.servers_stopped) return null
    return (
      <Button size="sm" isDisabled={starting} onPress={handleStartBattlegroup}>
        {starting ? <Spinner size="sm" color="current" /> : t('backups.restoreProgress.startBattlegroup')}
      </Button>
    )
  }

  return (
    <Modal.Backdrop
      variant="blur"
      className="bg-linear-to-t from-(--background)/90 via-(--background)/50 to-transparent"
      isOpen={open}
      isDismissable={false}
      onOpenChange={() => {}}
    >
      <Modal.Container size="sm">
        <Modal.Dialog className="p-10">
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {t('backups.restoreProgress.title')}
            </Modal.Heading>
          </Modal.Header>

          <Modal.Body className="flex flex-col gap-4">
            {renderSteps()}
            {renderSuccess()}
            {renderFailure()}
          </Modal.Body>

          <Modal.Footer className="flex justify-end gap-2">
            {renderStartButton()}
            <Button size="sm" variant="ghost" onPress={onClose} isDisabled={!done}>
              {t('backups.restoreProgress.close')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
