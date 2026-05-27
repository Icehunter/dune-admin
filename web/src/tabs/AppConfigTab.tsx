import { useState, useEffect } from 'react'
import { Button, ListBox, Select, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { AppConfig } from '../api/client'
import { PageHeader, Panel, SectionDivider } from '../dune-ui'

const EMPTY: AppConfig = {
  control: 'kubectl',
  ssh_host: '', ssh_user: 'dune', ssh_key: '',
  db_host: '127.0.0.1', db_port: 15432, db_user: 'dune',
  db_pass: '', db_name: 'dune', db_schema: 'dune',
  control_namespace: '',
  docker_gameserver: '', docker_broker_game: '', docker_broker_admin: '',
  cmd_start: '', cmd_stop: '', cmd_restart: '', cmd_status: '',
  broker_game_addr: '', broker_admin_addr: '', broker_tls: false,
  broker_exec_prefix: '',
  backup_dir: '', listen_addr: ':8080', scrip_currency: 1,
}

function Field({ label, value, onChange, placeholder, type = 'text', hint }: {
  label: string
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  hint?: string
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-muted font-medium uppercase tracking-wide">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="bg-surface border border-border rounded px-3 py-1.5 text-sm text-foreground placeholder:text-muted/50 focus:outline-none focus:border-accent/60 font-mono"
      />
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-3">
      <SectionDivider title={title} />
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {children}
      </div>
    </div>
  )
}

export default function SettingsTab() {
  const [cfg, setCfg] = useState<AppConfig>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.config.get()
      .then(c => setCfg({ ...EMPTY, ...c }))
      .catch(() => toast.danger('Could not load config'))
      .finally(() => setLoading(false))
  }, [])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg(prev => ({ ...prev, [key]: key === 'db_port' || key === 'scrip_currency' ? Number(v) || 0 : v }))

  const save = async () => {
    setSaving(true)
    try {
      await api.config.save(cfg)
      toast.success('Settings saved — reconnecting…')
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">Loading config…</span>
      </div>
    )
  }

  const isKubectl = cfg.control === 'kubectl'
  const isDocker  = cfg.control === 'docker'
  const isLocal   = cfg.control === 'local'

  return (
    <div className="flex flex-col h-full gap-4 min-h-0 overflow-y-auto pb-6">
      <PageHeader title="Settings">
        <Button size="sm" onPress={save} isDisabled={saving}>
          {saving ? <Spinner size="sm" color="current" /> : 'Save & Reconnect'}
        </Button>
      </PageHeader>

      <Panel>
        <div className="flex flex-col gap-5 p-4">

          {/* Provider */}
          <Section title="Provider">
            <div className="flex flex-col gap-1 sm:col-span-2">
              <label className="text-xs text-muted font-medium uppercase tracking-wide">Control plane</label>
              <Select
                selectedKey={cfg.control || 'kubectl'}
                onSelectionChange={k => setCfg(prev => ({ ...prev, control: String(k) }))}
                className="w-56"
              >
                <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    <ListBox.Item id="kubectl" textValue="kubectl">kubectl — K8s / k3s over SSH<ListBox.ItemIndicator /></ListBox.Item>
                    <ListBox.Item id="docker" textValue="docker">docker — Docker containers<ListBox.ItemIndicator /></ListBox.Item>
                    <ListBox.Item id="local" textValue="local">local — AMP / LGSM / bare metal<ListBox.ItemIndicator /></ListBox.Item>
                  </ListBox>
                </Select.Popover>
              </Select>
              <span className="text-xs text-muted">
                {isKubectl && 'All commands run over SSH. Namespace and DB credentials auto-discovered.'}
                {isDocker  && 'Uses docker CLI. Configure container names below.'}
                {isLocal   && 'Runs shell commands on the local machine.'}
              </span>
            </div>
          </Section>

          {/* SSH */}
          {(isKubectl || cfg.ssh_host) && (
            <Section title={isKubectl ? 'SSH' : 'SSH (optional — tunnels all traffic)'}>
              <Field label="Host : port" value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="192.168.0.72:22" />
              <Field label="User" value={cfg.ssh_user} onChange={set('ssh_user')} placeholder="dune" />
              <Field label="Private key path" value={cfg.ssh_key} onChange={set('ssh_key')} placeholder="auto-detected" hint="Leave blank to use auto-detection" />
            </Section>
          )}
          {!isKubectl && !cfg.ssh_host && (
            <Section title="SSH (optional)">
              <Field label="Host : port" value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="leave blank for local" hint="Set to tunnel all DB + exec traffic through SSH" />
            </Section>
          )}

          {/* Database */}
          <Section title="Database">
            <Field label="Host / DNS" value={cfg.db_host} onChange={set('db_host')}
              placeholder={isKubectl ? 'auto-discovered' : isDocker ? 'database' : '127.0.0.1'}
              hint={isKubectl ? 'Ignored for kubectl — DB tunnels to discovered pod IP' : undefined} />
            <Field label="Port" value={cfg.db_port} onChange={set('db_port')} placeholder="15432" type="number" />
            <Field label="User" value={cfg.db_user} onChange={set('db_user')} placeholder="dune" />
            <Field label="Password" value={cfg.db_pass} onChange={set('db_pass')} type="password" placeholder="••••••••" />
            <Field label="Database name" value={cfg.db_name} onChange={set('db_name')} placeholder="dune" />
            <Field label="Schema" value={cfg.db_schema} onChange={set('db_schema')} placeholder="dune" />
          </Section>

          {/* kubectl-specific */}
          {isKubectl && (
            <Section title="Kubernetes">
              <Field label="Namespace" value={cfg.control_namespace} onChange={set('control_namespace')}
                placeholder="auto-discovered" hint="Leave blank to auto-discover from kubectl" />
            </Section>
          )}

          {/* docker-specific */}
          {isDocker && (
            <Section title="Docker containers">
              <Field label="Game server container" value={cfg.docker_gameserver} onChange={set('docker_gameserver')} placeholder="dune-gameserver" />
              <Field label="mq-game broker container" value={cfg.docker_broker_game} onChange={set('docker_broker_game')} placeholder="dune-mq-game (optional)" />
              <Field label="mq-admin broker container" value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')} placeholder="dune-mq-admin (optional)" />
            </Section>
          )}

          {/* local-specific */}
          {isLocal && (
            <Section title="Server commands (optional)">
              <Field label="Start" value={cfg.cmd_start} onChange={set('cmd_start')} placeholder='ampinstmgr start DuneAwakening01' />
              <Field label="Stop" value={cfg.cmd_stop} onChange={set('cmd_stop')} placeholder='ampinstmgr stop DuneAwakening01' />
              <Field label="Restart" value={cfg.cmd_restart} onChange={set('cmd_restart')} placeholder='ampinstmgr restart DuneAwakening01' />
              <Field label="Status" value={cfg.cmd_status} onChange={set('cmd_status')} placeholder='ampinstmgr status DuneAwakening01' />
            </Section>
          )}

          {/* Broker */}
          <Section title="RabbitMQ broker (optional — for capture mode)">
            <Field label="mq-game addr" value={cfg.broker_game_addr} onChange={set('broker_game_addr')} placeholder="10.43.48.246:5672" />
            <Field label="mq-admin addr" value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')} placeholder="10.43.189.193:5672" />
            <div className="flex flex-col gap-1">
              <label className="text-xs text-muted font-medium uppercase tracking-wide">TLS</label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={cfg.broker_tls}
                  onChange={e => setCfg(prev => ({ ...prev, broker_tls: e.target.checked }))}
                  className="accent-[var(--color-accent)] w-4 h-4"
                />
                <span className="text-sm text-foreground">Use TLS (amqps://)</span>
              </label>
            </div>
            {isLocal && (
              <Field label="Exec prefix" value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')}
                placeholder='podman exec AMP_MehDune01'
                hint="Prepended to rabbitmqctl — use when broker runs inside a container" />
            )}
          </Section>

          {/* Misc */}
          <Section title="Advanced">
            <Field label="Backup directory" value={cfg.backup_dir} onChange={set('backup_dir')} placeholder="/funcom/artifacts/database-dumps/mybg" />
            <Field label="Listen address" value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" hint="Requires restart to take effect" />
            <Field label="Scrip currency ID" value={cfg.scrip_currency} onChange={set('scrip_currency')} type="number" placeholder="1" />
          </Section>

        </div>
      </Panel>
    </div>
  )
}
