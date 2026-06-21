import * as React from 'react'
import { Button, Input, Spinner } from '@heroui/react'
import { Stepper } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { toast } from '@heroui/react'
import { ServerSettingsForm } from './settings/server/ServerSettingsForm'
import { DiscoveryModal } from './DiscoveryModal'
import { api } from '../api/client'
import type { AppConfig } from '../api/client'
import type { SetupWizardProps } from './interfaces'

// First-run setup configures the whole install (global + the default server).
const FULL_STEPS = [
  { id: 'control', title: 'Control Plane', description: 'How dune-admin manages your server' },
  { id: 'ssh', title: 'SSH', description: 'Host connection credentials' },
  { id: 'db', title: 'Database', description: 'PostgreSQL connection settings' },
  { id: 'broker', title: 'Broker', description: 'RabbitMQ connection (optional)' },
  { id: 'auth', title: 'Auth', description: 'Dashboard login settings' },
  { id: 'discord', title: 'Discord Bot', description: 'Notifications and role integration' },
  { id: 'advanced', title: 'Advanced', description: 'Paths, listen address, and more' },
]

// Adding another server is per-server only — global settings (auth, Discord,
// listen address) belong to the install, not to an individual server.
const ADD_STEPS = [
  { id: 'control', title: 'Control Plane', description: 'How dune-admin manages this server' },
  { id: 'ssh', title: 'SSH', description: 'Host connection credentials' },
  { id: 'db', title: 'Database', description: 'PostgreSQL connection settings' },
  { id: 'broker', title: 'Broker', description: 'RabbitMQ connection (optional)' },
  { id: 'advanced', title: 'Advanced', description: 'Market bot' },
]

export const SetupWizard: React.FC<SetupWizardProps> = ({ onDone }) => {
  const { t } = useTranslation()
  const saveRef = React.useRef<(() => Promise<void>) | null>(null)
  const [step, setStep] = React.useState(0)
  const [saving, setSaving] = React.useState(false)
  const [reconnecting, setReconnecting] = React.useState(false)
  const [discovering, setDiscovering] = React.useState(false)
  const [serverName, setServerName] = React.useState('')
  const [showDiscovery, setShowDiscovery] = React.useState(false)
  // Live form config (for add-server auto-discovery) and the discovered values
  // to merge back into the form.
  const [liveConfig, setLiveConfig] = React.useState<AppConfig | null>(null)
  const [prefill, setPrefill] = React.useState<Partial<AppConfig> | null>(null)

  const isAddMode = !!onDone
  const STEPS = isAddMode ? ADD_STEPS : FULL_STEPS
  const isLast = step === STEPS.length - 1

  // Watchdog: first-run mode shows a full-screen "Connecting…" and waits for the
  // status poll to flip needs_setup. If the DB never comes up that never happens,
  // so after a grace period fall back to the form with an error instead of
  // hanging forever.
  React.useEffect(() => {
    if (!reconnecting) return
    const id = setTimeout(() => {
      setReconnecting(false)
      toast.danger(t('setup.connectTimeout', 'Still not connected — check your settings and try again.'))
    }, 15000)
    return () => clearTimeout(id)
  }, [reconnecting, t])

  const handleNext = async () => {
    // Add-server mode: require a name on the first step (it drives the id).
    if (isAddMode && step === 0 && !serverName.trim()) {
      toast.danger(t('setup.nameRequired', 'Server name is required'))
      return
    }
    if (isLast) {
      await handleSaveAndConnect()
      return
    }

    // Add-server mode: after the SSH step, run auto-discovery against the entered
    // connection settings (an ephemeral executor — the server isn't created yet),
    // showing a stepped modal. The modal's onDone pre-fills and advances.
    if (isAddMode && STEPS[step]?.id === 'ssh') {
      setShowDiscovery(true)
      return
    }

    // After SSH step: save partial config, reconnect, then auto-discover DB/broker.
    // First-run only — auto-discover operates on the flat/global connection; in
    // add-server mode the new server isn't created until the final step.
    if (!isAddMode && STEPS[step]?.id === 'ssh') {
      setSaving(true)
      try {
        await saveRef.current?.()
      }
      catch {
        setSaving(false)
        return
      }
      setSaving(false)
      setDiscovering(true)
      try {
        await api.reconnect()
        const found = await api.config.discover(true)
        const msgs: string[] = []
        if (found.db_user) msgs.push('database')
        if (found.broker_game) msgs.push('broker')
        if (msgs.length) {
          toast.success(`Auto-discovered: ${msgs.join(', ')} — values pre-filled`)
        }
      }
      catch {
        toast.warning('Could not auto-discover — fill in manually')
      }
      finally {
        setDiscovering(false)
      }
    }

    setStep((s) => s + 1)
  }

  const handleSaveAndConnect = async () => {
    if (!saveRef.current) return
    setSaving(true)
    try {
      await saveRef.current()
    }
    catch (e) {
      // persist() throws on validation (e.g. missing name) or a failed add;
      // surface it and stay on the wizard so the user can correct.
      toast.danger(`${e instanceof Error ? e.message : String(e)}`)
      setSaving(false)
      return
    }
    setSaving(false)

    // Add-server mode: POST /servers already created and connected the new
    // server (tunneling the DB for AMP). Return to the app — no flat reconnect.
    if (isAddMode) {
      onDone?.()
      return
    }

    setReconnecting(true)
    try {
      await api.reconnect()
    }
    catch (e) {
      toast.danger(`Connect failed: ${e instanceof Error ? e.message : String(e)}`)
      setReconnecting(false)
    }
    // First-run mode: useStatus() poll picks up needs_setup=false and re-renders.
  }

  if (reconnecting) {
    return (
      <div className="h-full flex flex-col items-center justify-center gap-4 text-foreground">
        <Spinner />
        <p className="text-muted">{t('setup.connecting', 'Connecting…')}</p>
        {/* Escape hatch: connecting can hang (e.g. DB unreachable). Don't trap
            the user on this screen — let them go back and fix their settings. */}
        <Button variant="outline" size="sm" onPress={() => setReconnecting(false)}>
          {t('common.back', 'Back')}
        </Button>
      </div>
    )
  }

  return (
    // Rendered inside the Add-server modal — fill it, no full-screen chrome.
    <div className="h-full flex flex-col overflow-hidden text-foreground">
      {/* ── Stepper header ────────────────────────────────────────────── */}
      <div className="flex-shrink-0 pb-4 border-b border-border">
        {/* Stepper */}
        <Stepper
          currentStep={step}
          size="sm"
          style={
            {
              '--stepper-active-color': '#c9820a',
              '--stepper-complete-color': '#c9820a',
              '--stepper-complete-fg': '#fff',
            } as React.CSSProperties
          }
        >
          {STEPS.map((s) => (
            <Stepper.Step key={s.id}>
              <Stepper.Indicator />
              <Stepper.Content>
                <Stepper.Title>{s.title}</Stepper.Title>
              </Stepper.Content>
              <Stepper.Separator />
            </Stepper.Step>
          ))}
        </Stepper>
      </div>

      {/* ── Scrollable form area ──────────────────────────────────────── */}
      <div className="flex-1 overflow-y-auto py-4">
        {discovering
          ? (
              <div className="flex flex-col items-center gap-3 py-16">
                <Spinner />
                <p className="text-sm text-muted">{t('setup.discovering', 'Auto-discovering server settings…')}</p>
              </div>
            )
          : (
              <div className="flex flex-col gap-4">
                {/* Add-server mode: name the new server (drives its id). Shown on
                    the first step; the value persists across steps. */}
                {isAddMode && step === 0 && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted font-medium">
                      {t('setup.serverName', 'Server name')}
                      <span className="text-danger"> *</span>
                    </span>
                    <Input
                      value={serverName}
                      onChange={(e) => setServerName(e.target.value)}
                      placeholder={t('setup.serverNamePlaceholder', 'e.g. Production')}
                      aria-label={t('setup.serverName', 'Server name')}
                      autoFocus
                    />
                    {!serverName.trim() && (
                      <span className="text-xs text-muted">{t('setup.nameRequired', 'Server name is required')}</span>
                    )}
                  </div>
                )}
                <ServerSettingsForm
                  saveRef={saveRef}
                  onSavingChange={setSaving}
                  activeTab={STEPS[step]?.id}
                  hideTabBar
                  addMode={isAddMode}
                  addServerName={serverName}
                  onConfigChange={setLiveConfig}
                  prefill={prefill}
                />
              </div>
            )}
      </div>

      {/* Add-server auto-discovery: stepped modal that probes the entered
          connection and pre-fills DB/broker/namespace before the DB step. */}
      {isAddMode && showDiscovery && liveConfig && (
        <DiscoveryModal
          open={showDiscovery}
          config={{ id: 0, name: serverName, ...liveConfig }}
          onDone={(discovered) => {
            setPrefill(discovered)
            setShowDiscovery(false)
            setStep((s) => s + 1)
          }}
          onSkip={() => {
            setShowDiscovery(false)
            setStep((s) => s + 1)
          }}
        />
      )}

      {/* ── Sticky footer ─────────────────────────────────────────────── */}
      <div className="flex-shrink-0 border-t border-border py-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            isDisabled={step === 0 || saving || discovering}
            onPress={() => setStep((s) => s - 1)}
          >
            {t('common.back', 'Back')}
          </Button>
        </div>

        <span className="text-xs text-muted">
          {step + 1}
          {' / '}
          {STEPS.length}
        </span>

        <Button
          size="sm"
          isDisabled={saving || discovering || (isAddMode && step === 0 && !serverName.trim())}
          onPress={() => void handleNext()}
        >
          {saving
            ? <Spinner size="sm" />
            : isLast
              ? t('setup.saveAndConnect', 'Save & Connect')
              : t('common.next', 'Next')}
        </Button>
      </div>
    </div>
  )
}
