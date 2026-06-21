import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, Spinner } from '@heroui/react'
import { Icon } from '../dune-ui'
import { api } from '../api/client'
import type { DiscoveryModalProps } from './interfaces'

// Per-control-plane step labels. The backend does the work in one call; these
// steps are revealed sequentially with a short delay so the process reads as
// deliberate rather than instant.
const STEP_KEYS: Record<string, string[]> = {
  kubectl: ['connect', 'namespace', 'dbPod', 'dbCreds', 'brokers', 'finalize'],
  amp: ['connect', 'gameProc', 'dbCreds', 'finalize'],
  docker: ['connect', 'containers', 'dbCreds', 'finalize'],
  local: ['gameProc', 'dbCreds', 'finalize'],
}

const STEP_MS = 650 // artificial per-step delay (within the requested 500ms–1s)

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

export const DiscoveryModal: React.FC<DiscoveryModalProps> = ({ open, config, onDone, onSkip }) => {
  const { t } = useTranslation()
  const control = config.control || 'local'
  const steps = STEP_KEYS[control] ?? STEP_KEYS.local
  const [current, setCurrent] = React.useState(0)
  const [failed, setFailed] = React.useState(false)

  // Stable refs so the effect only re-triggers on `open` change, not on prop
  // identity churn (config/onDone/steps all change on each render).
  const configRef = React.useRef(config)
  const onDoneRef = React.useRef(onDone)
  const stepsRef = React.useRef(steps)
  React.useLayoutEffect(() => {
    configRef.current = config
    onDoneRef.current = onDone
    stepsRef.current = steps
  })

  // Run discovery once per open. The single API call runs while the step list
  // animates; we apply results after both finish.
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    void Promise.resolve().then(() => {
      setCurrent(0)
      setFailed(false)
    })

    const run = async () => {
      const apiCall = api.servers.discover(configRef.current).catch(() => null)
      for (let i = 0; i < stepsRef.current.length; i++) {
        if (cancelled) return
        await Promise.resolve().then(() => setCurrent(i))
        await sleep(STEP_MS)
      }
      const result = await apiCall
      if (cancelled) return
      if (!result) {
        setFailed(true)
        // Still let the wizard proceed — values can be filled manually.
        await sleep(800)
        if (!cancelled) onDoneRef.current({})
        return
      }
      onDoneRef.current(result)
    }
    void run()
    return () => {
      cancelled = true
    }
  }, [open])

  return (
    <Modal.Backdrop
      variant="blur"
      className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent"
      isOpen={open}
      onOpenChange={(v) => !v && onSkip()}
    >
      <Modal.Container size="sm">
        <Modal.Dialog className="p-8">
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('discovery.title', 'Auto-discovering settings')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body>
            <p className="text-xs text-muted mb-3">{t('discovery.subtitle', 'Reading values from your server…')}</p>
            <ul className="flex flex-col gap-2">
              {steps.map((key, i) => (
                <li key={key} className="flex items-center gap-2 text-sm">
                  <span className="w-4 flex justify-center">
                    {i < current
                      ? <Icon name="check" className="text-success" />
                      : i === current
                        ? <Spinner size="sm" color="current" />
                        : <span className="text-muted opacity-40">•</span>}
                  </span>
                  <span className={i <= current ? 'text-foreground' : 'text-muted'}>
                    {t(`discovery.steps.${key}`, key)}
                  </span>
                </li>
              ))}
            </ul>
            {failed && (
              <p className="text-xs text-warning mt-3">
                {t('discovery.failed', 'Could not auto-discover — fill in the values manually.')}
              </p>
            )}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
