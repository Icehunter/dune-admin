# Runtime Config Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed a full, searchable config editor in the Settings modal so operators can view and edit `~/.dune-admin/config.yaml` from the running app without touching the file directly.

**Architecture:** Extend backend masking in `handlers_config.go` for three additional secret fields, extend the `AppConfig` TypeScript type, then build `SettingsConfigForm.tsx` by adapting the existing orphaned `AppConfigTab.tsx` (add search, field descriptions, missing sections: AMP, Market Bot, broker auth, server INI), wire it into the Settings modal in `App.tsx`, and delete the now-superseded tab file.

**Tech Stack:** Go (pgx/v5, yaml.v3), React + TypeScript (strict), HeroUI v3 (`Panel`, `SearchField`, `Select`, `Button`, `Spinner`, `toast`), dune-ui wrappers.

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `cmd/dune-admin/handlers_config.go` | Modify | Extend masking for BrokerPass, BrokerJWTSecret, MarketBotRemoteToken |
| `cmd/dune-admin/main_config_test.go` | Create | Tests for the new masking behaviour |
| `web/src/api/client.ts` | Modify | Extend AppConfig type; export MASKED constant |
| `web/src/components/SettingsConfigForm.tsx` | Create | Self-contained config form with search and field descriptions |
| `web/src/App.tsx` | Modify | Modal grows to `cover`; imports and renders SettingsConfigForm |
| `web/src/tabs/AppConfigTab.tsx` | Delete | Orphaned tab superseded by SettingsConfigForm |

---

## Task 1: Backend — extend secret masking

**Files:**

- Modify: `cmd/dune-admin/handlers_config.go`
- Create: `cmd/dune-admin/main_config_test.go`

### Why

`handleGetConfig` currently only masks `DBPass`. `BrokerPass`, `BrokerJWTSecret`, and `MarketBotRemoteToken` are stored in `config.yaml` and returned to the frontend unredacted. `handleSaveConfig` calls `preserveMaskedDBPass` which only handles `DBPass` — submitting the placeholder for the other secrets would overwrite them with `"••••••••"` in the file.

### Steps

- [ ] **1.1 Write failing tests**

Create `cmd/dune-admin/main_config_test.go`:

```go
package main

import (
 "os"
 "path/filepath"
 "testing"

 "gopkg.in/yaml.v3"
)

func TestPreserveMaskedSecrets(t *testing.T) {
 t.Parallel()

 const mask = "••••••••"

 write := func(t *testing.T, cfg appConfig) string {
  t.Helper()
  dir := t.TempDir()
  p := filepath.Join(dir, "config.yaml")
  data, err := yaml.Marshal(cfg)
  if err != nil {
   t.Fatal(err)
  }
  if err := os.WriteFile(p, data, 0600); err != nil {
   t.Fatal(err)
  }
  return p
 }

 t.Run("BrokerPass placeholder is preserved from file", func(t *testing.T) {
  t.Parallel()
  path := write(t, appConfig{BrokerPass: "real-broker-pass"})
  cfg := appConfig{BrokerPass: mask}
  preserveMaskedSecrets(&cfg, os.ReadFile, path)
  if cfg.BrokerPass != "real-broker-pass" {
   t.Fatalf("expected real-broker-pass, got %q", cfg.BrokerPass)
  }
 })

 t.Run("BrokerJWTSecret placeholder is preserved from file", func(t *testing.T) {
  t.Parallel()
  path := write(t, appConfig{BrokerJWTSecret: "real-jwt-secret"})
  cfg := appConfig{BrokerJWTSecret: mask}
  preserveMaskedSecrets(&cfg, os.ReadFile, path)
  if cfg.BrokerJWTSecret != "real-jwt-secret" {
   t.Fatalf("expected real-jwt-secret, got %q", cfg.BrokerJWTSecret)
  }
 })

 t.Run("MarketBotRemoteToken placeholder is preserved from file", func(t *testing.T) {
  t.Parallel()
  path := write(t, appConfig{MarketBotRemoteToken: "real-token"})
  cfg := appConfig{MarketBotRemoteToken: mask}
  preserveMaskedSecrets(&cfg, os.ReadFile, path)
  if cfg.MarketBotRemoteToken != "real-token" {
   t.Fatalf("expected real-token, got %q", cfg.MarketBotRemoteToken)
  }
 })

 t.Run("non-masked values pass through unchanged", func(t *testing.T) {
  t.Parallel()
  path := write(t, appConfig{BrokerPass: "old", BrokerJWTSecret: "old", MarketBotRemoteToken: "old"})
  cfg := appConfig{BrokerPass: "new", BrokerJWTSecret: "new", MarketBotRemoteToken: "new"}
  preserveMaskedSecrets(&cfg, os.ReadFile, path)
  if cfg.BrokerPass != "new" || cfg.BrokerJWTSecret != "new" || cfg.MarketBotRemoteToken != "new" {
   t.Fatal("non-masked values should not be changed")
  }
 })

 t.Run("missing file does not write mask string to config", func(t *testing.T) {
  t.Parallel()
  cfg := appConfig{
   DBPass:               mask,
   BrokerPass:           mask,
   BrokerJWTSecret:      mask,
   MarketBotRemoteToken: mask,
  }
  preserveMaskedSecrets(&cfg, os.ReadFile, "/nonexistent/path/config.yaml")
  // The mask placeholder must never be written to the config file.
  if cfg.DBPass == mask || cfg.BrokerPass == mask || cfg.BrokerJWTSecret == mask || cfg.MarketBotRemoteToken == mask {
   t.Fatal("mask placeholder must never be written to config file")
  }
 })
}

func TestHandleGetConfigMasksSecrets(t *testing.T) {
 t.Parallel()
 // buildCurrentConfig is a pure function — verify it masks DBPass regardless
 // of whatever value the global holds.
 orig := dbPass
 dbPass = "supersecret"
 t.Cleanup(func() { dbPass = orig })

 cfg := buildCurrentConfig()
 if cfg.DBPass != "••••••••" {
  t.Fatalf("expected masked DBPass, got %q", cfg.DBPass)
 }
}
```

- [ ] **1.2 Run tests — expect compile failure** (function `preserveMaskedSecrets` not yet defined)

```bash
go test ./cmd/dune-admin/ -run TestPreserveMaskedSecrets -v 2>&1 | head -20
```

Expected: build error `undefined: preserveMaskedSecrets`.

- [ ] **1.3 Implement `preserveMaskedSecrets` and update masking in `handlers_config.go`**

Replace `handlers_config.go` with:

```go
package main

import (
 "fmt"
 "net/http"
 "os"

 "gopkg.in/yaml.v3"
)

const masked = "••••••••"

// handleGetConfig returns the current config with all secret fields masked.
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
 data, err := os.ReadFile(configPath())
 if err != nil {
  jsonOK(w, buildCurrentConfig())
  return
 }
 var cfg appConfig
 if err := yaml.Unmarshal(data, &cfg); err != nil {
  jsonErr(w, fmt.Errorf("parse config: %w", err), 500)
  return
 }
 maskSecrets(&cfg)
 jsonOK(w, cfg)
}

// maskSecrets replaces secret fields with the display placeholder.
func maskSecrets(cfg *appConfig) {
 if cfg.DBPass != "" {
  cfg.DBPass = masked
 }
 if cfg.BrokerPass != "" {
  cfg.BrokerPass = masked
 }
 if cfg.BrokerJWTSecret != "" {
  cfg.BrokerJWTSecret = masked
 }
 if cfg.MarketBotRemoteToken != "" {
  cfg.MarketBotRemoteToken = masked
 }
}

// preserveMaskedSecrets restores real secret values when the client sent back
// the display placeholder. Falls back to loadedConfig when the file is
// unreadable so in-memory secrets survive a mid-session config file move.
func preserveMaskedSecrets(
 cfg *appConfig,
 readFile func(string) ([]byte, error),
 path string,
) {
 needsRestore := cfg.DBPass == masked ||
  cfg.BrokerPass == masked ||
  cfg.BrokerJWTSecret == masked ||
  cfg.MarketBotRemoteToken == masked

 if !needsRestore {
  return
 }

 old := loadedConfig
 if data, err := readFile(path); err == nil {
  _ = yaml.Unmarshal(data, &old)
 }
 // dbPass global may differ from loadedConfig when set from env var
 if old.DBPass == "" {
  old.DBPass = dbPass
 }

 if cfg.DBPass == masked {
  cfg.DBPass = old.DBPass
 }
 if cfg.BrokerPass == masked {
  cfg.BrokerPass = old.BrokerPass
 }
 if cfg.BrokerJWTSecret == masked {
  cfg.BrokerJWTSecret = old.BrokerJWTSecret
 }
 if cfg.MarketBotRemoteToken == masked {
  cfg.MarketBotRemoteToken = old.MarketBotRemoteToken
 }
}

func writeConfigFile(cfg appConfig) error {
 if err := os.MkdirAll(configDir(), 0700); err != nil {
  return fmt.Errorf("create config dir: %w", err)
 }
 data, err := yaml.Marshal(cfg)
 if err != nil {
  return fmt.Errorf("marshal config: %w", err)
 }
 if err := os.WriteFile(configPath(), data, 0600); err != nil {
  return fmt.Errorf("write config: %w", err)
 }
 return nil
}

func resetRuntimeConnections() {
 if globalDB != nil {
  globalDB.Close()
  globalDB = nil
 }
 if globalExecutor != nil {
  globalExecutor.Close()
  globalExecutor = nil
 }
 globalSSH = nil
 globalControl = nil
}

func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
 var cfg appConfig
 if err := decode(r, &cfg); err != nil {
  jsonErr(w, fmt.Errorf("decode: %w", err), 400)
  return
 }

 preserveMaskedSecrets(&cfg, os.ReadFile, configPath())

 if err := writeConfigFile(cfg); err != nil {
  jsonErr(w, err, 500)
  return
 }

 applyConfig(cfg)
 resetRuntimeConnections()

 if err := connectAll(); err != nil {
  jsonErr(w, fmt.Errorf("reconnect failed: %w", err), 500)
  return
 }
 handleStatus(w, r)
}

// buildCurrentConfig constructs an appConfig from the current global vars.
func buildCurrentConfig() appConfig {
 return appConfig{
  SSHHost:          sshHost,
  SSHUser:          sshUser,
  SSHKey:           sshKeyPath,
  DBHost:           dbHost,
  DBPort:           dbPort,
  DBUser:           dbUser,
  DBPass:           masked,
  DBName:           dbName,
  DBSchema:         dbSchema,
  Control:          controlPlane,
  ControlNamespace: controlNS,
  BrokerGameAddr:   brokerGameAddr,
  BrokerAdminAddr:  brokerAdminAddr,
  BrokerTLS:        brokerTLS,
  BackupDir:        backupDir,
  ListenAddr:       listenAddr,
  ScripCurrency:    scripCurrencyID,
 }
}

// applyConfig pushes a saved appConfig back into the runtime globals so that
// connectAll() picks up the new values without requiring a process restart.
func applyConfig(cfg appConfig) {
 sshHost = cfg.SSHHost
 sshUser = cfg.SSHUser
 if cfg.SSHKey != "" {
  sshKeyPath = cfg.SSHKey
 }
 dbHost = cfg.DBHost
 if cfg.DBPort != 0 {
  dbPort = cfg.DBPort
 }
 dbUser = cfg.DBUser
 dbPass = cfg.DBPass
 dbName = cfg.DBName
 dbSchema = cfg.DBSchema
 controlPlane = cfg.Control
 controlNS = cfg.ControlNamespace
 brokerGameAddr = cfg.BrokerGameAddr
 brokerAdminAddr = cfg.BrokerAdminAddr
 brokerTLS = cfg.BrokerTLS
 backupDir = cfg.BackupDir
 loadedConfig = cfg
}
```

Note: `preserveMaskedDBPass` is removed — `handleSaveConfig` now calls `preserveMaskedSecrets` which covers all four fields including DBPass.

- [ ] **1.4 Run tests — expect pass**

```bash
go test ./cmd/dune-admin/ -run "TestPreserveMaskedSecrets|TestHandleGetConfigMasksSecrets" -v -race
```

Expected: all subtests PASS.

- [ ] **1.5 Full verify**

```bash
make verify
```

Expected: `All verification checks passed!`

- [ ] **1.6 Commit**

```bash
git add cmd/dune-admin/handlers_config.go cmd/dune-admin/main_config_test.go
git commit -m "fix: extend config masking to BrokerPass, BrokerJWTSecret, MarketBotRemoteToken"
```

---

## Task 2: Frontend — extend AppConfig type

**Files:**

- Modify: `web/src/api/client.ts` lines 118–144

### Steps

- [ ] **2.1 Replace the `AppConfig` type block**

In `web/src/api/client.ts`, replace the existing `AppConfig` type (lines 118–144) with:

```ts
export const MASKED = '••••••••'

export type AppConfig = {
  // Control plane
  control: string
  // SSH
  ssh_host: string
  ssh_user: string
  ssh_key: string
  // Database
  db_host: string
  db_port: number
  db_user: string
  db_pass: string      // masked when non-empty
  db_name: string
  db_schema: string
  // kubectl
  control_namespace: string
  // docker
  docker_gameserver: string
  docker_broker_game: string
  docker_broker_admin: string
  docker_db: string
  // local shell commands
  cmd_start: string
  cmd_stop: string
  cmd_restart: string
  cmd_status: string
  // Broker
  broker_game_addr: string
  broker_admin_addr: string
  broker_tls: boolean
  broker_user: string
  broker_pass: string        // masked when non-empty
  broker_jwt_secret: string  // masked when non-empty
  broker_exec_prefix: string
  // Server paths
  backup_dir: string
  server_ini_dir: string
  default_ini_dir: string
  // AMP
  amp_instance: string
  amp_container: string
  amp_user: string
  amp_log_path: string
  amp_use_container: boolean
  amp_data_root: string
  director_url: string
  // Market bot
  market_bot_enabled: boolean
  market_bot_cache_db: string
  market_bot_item_data: string
  market_bot_state: string
  market_bot_buy_interval: string   // duration string, e.g. "5m0s"
  market_bot_list_interval: string
  market_bot_buy_threshold: number
  market_bot_max_buys: number
  market_bot_remote_url: string
  market_bot_remote_token: string   // masked when non-empty
  // Advanced
  listen_addr: string
  scrip_currency: number
}
```

- [ ] **2.2 Build to verify no type errors**

```bash
cd web && pnpm build 2>&1 | tail -15
```

Expected: build succeeds (existing `AppConfigTab.tsx` may show type errors — those are fixed in Task 4 when the file is deleted).

- [ ] **2.3 Commit**

```bash
cd .. && git add web/src/api/client.ts
git commit -m "feat: extend AppConfig type with AMP, market bot, broker auth, server INI fields"
```

---

## Task 3: Create SettingsConfigForm component

**Files:**

- Create: `web/src/components/SettingsConfigForm.tsx`

This replaces and extends `AppConfigTab.tsx`. It is self-contained: fetches config on mount, renders all sections with search filtering and field descriptions, saves via the API.

### Steps

- [ ] **3.1 Create `web/src/components/SettingsConfigForm.tsx`**

```tsx
import { useState, useEffect } from 'react'
import { Button, SearchField, Select, ListBox, Spinner, toast } from '@heroui/react'
import { api, MASKED } from '../api/client'
import type { AppConfig } from '../api/client'
import { Panel, SectionDivider } from '../dune-ui'
import { Icon } from '../dune-ui'

// ── empty defaults ────────────────────────────────────────────────────────────

const EMPTY: AppConfig = {
  control: 'local',
  ssh_host: '', ssh_user: '', ssh_key: '',
  db_host: '127.0.0.1', db_port: 15432, db_user: 'dune',
  db_pass: '', db_name: 'dune', db_schema: 'dune',
  control_namespace: '',
  docker_gameserver: '', docker_broker_game: '', docker_broker_admin: '', docker_db: '',
  cmd_start: '', cmd_stop: '', cmd_restart: '', cmd_status: '',
  broker_game_addr: '', broker_admin_addr: '', broker_tls: false,
  broker_user: '', broker_pass: '', broker_jwt_secret: '', broker_exec_prefix: '',
  backup_dir: '', server_ini_dir: '', default_ini_dir: '',
  amp_instance: '', amp_container: '', amp_user: 'amp', amp_log_path: '',
  amp_use_container: true, amp_data_root: '',
  director_url: '',
  market_bot_enabled: false,
  market_bot_cache_db: '', market_bot_item_data: '', market_bot_state: '',
  market_bot_buy_interval: '', market_bot_list_interval: '',
  market_bot_buy_threshold: 0, market_bot_max_buys: 0,
  market_bot_remote_url: '', market_bot_remote_token: '',
  listen_addr: ':8080', scrip_currency: 1,
}

// ── search helper ─────────────────────────────────────────────────────────────

function matches(query: string, ...terms: string[]): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  return terms.some(t => t.toLowerCase().includes(q))
}

// ── field components ──────────────────────────────────────────────────────────

function Field({
  label, value, onChange, placeholder, type = 'text', hint, hidden,
}: {
  label: string
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  hint?: string
  hidden?: boolean
}) {
  if (hidden) return null
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

function CheckField({
  label, checked, onChange, hint, hidden,
}: {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
  hint?: string
  hidden?: boolean
}) {
  if (hidden) return null
  return (
    <div className="flex flex-col gap-1">
      <label className="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={checked}
          onChange={e => onChange(e.target.checked)}
          className="accent-[var(--color-accent)] w-4 h-4"
        />
        <span className="text-sm text-foreground">{label}</span>
      </label>
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

function Section({ title, hidden, children }: {
  title: string
  hidden?: boolean
  children: React.ReactNode
}) {
  if (hidden) return null
  return (
    <div className="flex flex-col gap-3">
      <SectionDivider title={title} />
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {children}
      </div>
    </div>
  )
}

// ── main component ────────────────────────────────────────────────────────────

export default function SettingsConfigForm() {
  const [cfg, setCfg] = useState<AppConfig>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')

  useEffect(() => {
    api.config.get()
      .then(c => setCfg({ ...EMPTY, ...c }))
      .catch(() => toast.danger('Could not load config'))
      .finally(() => setLoading(false))
  }, [])

  // Generic field setter — coerces numeric fields automatically
  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg(prev => ({
      ...prev,
      [key]: (key === 'db_port' || key === 'scrip_currency' || key === 'market_bot_max_buys')
        ? (Number(v) || 0)
        : key === 'market_bot_buy_threshold'
          ? (parseFloat(v) || 0)
          : v,
    }))

  const save = async () => {
    setSaving(true)
    try {
      await api.config.save(cfg)
      toast.success('Config saved — reconnecting…')
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">Loading config…</span>
      </div>
    )
  }

  const q = query
  const isKubectl = cfg.control === 'kubectl'
  const isDocker  = cfg.control === 'docker'
  const isLocal   = cfg.control === 'local'
  const isAmp     = cfg.control === 'amp'

  // Section visibility: a section shows if any of its fields match the query
  const showDB = matches(q,
    'Host', 'db_host', 'PostgreSQL host the game database is running on',
    'Port', 'db_port', 'PostgreSQL port',
    'User', 'db_user', 'Database user',
    'Password', 'db_pass', 'Database password',
    'Database name', 'db_name',
    'Schema', 'db_schema', 'Postgres schema prefix',
    'database', 'postgres',
  )
  const showSSH = matches(q,
    'SSH', 'ssh_host', 'Host', 'ssh tunnelling',
    'ssh_user', 'User', 'ssh_key', 'Private key',
    'Leave blank for local operation',
  )
  const showControl = matches(q,
    'Control', 'control', 'kubectl', 'docker', 'local', 'amp',
    'Kubernetes', 'Namespace', 'control_namespace',
    'docker_gameserver', 'Game server container',
    'docker_broker_game', 'docker_broker_admin', 'docker_db',
    'cmd_start', 'cmd_stop', 'cmd_restart', 'cmd_status',
    'Start', 'Stop', 'Restart', 'Status', 'shell command',
    'amp_instance', 'amp_container', 'amp_user', 'amp_log_path',
    'amp_use_container', 'amp_data_root',
    'ampinstmgr', 'podman', 'AMP',
  )
  const showBroker = matches(q,
    'Broker', 'RabbitMQ', 'broker_game_addr', 'broker_admin_addr', 'broker_tls',
    'broker_user', 'broker_pass', 'broker_jwt_secret', 'broker_exec_prefix',
    'Game addr', 'Admin addr', 'TLS', 'JWT', 'Exec prefix',
    'capture', 'notification',
  )
  const showMarketBot = matches(q,
    'Market', 'Bot', 'market_bot', 'market_bot_enabled', 'market_bot_remote_url',
    'market_bot_remote_token', 'market_bot_cache_db', 'market_bot_item_data',
    'market_bot_state', 'market_bot_buy_interval', 'market_bot_list_interval',
    'market_bot_buy_threshold', 'market_bot_max_buys',
    'Buy interval', 'List interval', 'Threshold', 'Max buys', 'Cache DB',
    'Remote URL', 'Remote token', 'in-process',
  )
  const showAdvanced = matches(q,
    'Advanced', 'Listen', 'listen_addr', 'Backup', 'backup_dir',
    'server_ini_dir', 'Server INI', 'default_ini_dir', 'Default INI',
    'director_url', 'Director', 'scrip_currency', 'Scrip currency',
    'restart to take effect',
  )

  return (
    <div className="flex flex-col gap-4">
      {/* Search */}
      <SearchField value={query} onChange={setQuery} aria-label="Search settings">
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder="Search fields…" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

      {/* Database */}
      <Section title="Database" hidden={!showDB}>
        <Field label="Host" value={cfg.db_host} onChange={set('db_host')}
          placeholder="127.0.0.1"
          hint="PostgreSQL host the game database is running on"
          hidden={!matches(q, 'Host', 'db_host', 'PostgreSQL host')} />
        <Field label="Port" value={cfg.db_port} onChange={set('db_port')}
          type="number" placeholder="15432"
          hint="PostgreSQL port (default 15432 for AMP, 5432 otherwise)"
          hidden={!matches(q, 'Port', 'db_port', 'PostgreSQL port')} />
        <Field label="User" value={cfg.db_user} onChange={set('db_user')}
          placeholder="dune"
          hint="Database user"
          hidden={!matches(q, 'User', 'db_user', 'Database user')} />
        <Field label="Password" value={cfg.db_pass} onChange={set('db_pass')}
          type="password" placeholder={MASKED}
          hint="Send the placeholder to keep the existing value unchanged"
          hidden={!matches(q, 'Password', 'db_pass', 'Database password')} />
        <Field label="Database name" value={cfg.db_name} onChange={set('db_name')}
          placeholder="dune"
          hint="PostgreSQL database name"
          hidden={!matches(q, 'Database name', 'db_name')} />
        <Field label="Schema" value={cfg.db_schema} onChange={set('db_schema')}
          placeholder="dune"
          hint="Postgres schema prefix — all game tables live here (typically dune)"
          hidden={!matches(q, 'Schema', 'db_schema', 'schema prefix')} />
      </Section>

      {/* SSH */}
      <Section title="SSH" hidden={!showSSH}>
        <Field label="Host : port" value={cfg.ssh_host} onChange={set('ssh_host')}
          placeholder="192.168.0.72:22"
          hint="SSH host (and optional :port). Leave blank for local operation — filling this tunnels all DB connections and executor commands through SSH"
          hidden={!matches(q, 'Host', 'ssh_host', 'SSH host', 'tunnelling')} />
        <Field label="User" value={cfg.ssh_user} onChange={set('ssh_user')}
          placeholder="dune"
          hint="SSH user on the remote host"
          hidden={!matches(q, 'User', 'ssh_user', 'SSH user')} />
        <Field label="Private key path" value={cfg.ssh_key} onChange={set('ssh_key')}
          placeholder="~/.ssh/id_ed25519"
          hint="Absolute path to the SSH private key file. Leave blank for auto-detection next to the binary"
          hidden={!matches(q, 'Private key', 'ssh_key', 'key path')} />
      </Section>

      {/* Control Plane */}
      <Section title="Control Plane" hidden={!showControl}>
        {/* Control dropdown */}
        {matches(q, 'Control', 'control', 'kubectl', 'docker', 'local', 'amp') && (
        <div className="flex flex-col gap-1 sm:col-span-2">
          <label className="text-xs text-muted font-medium uppercase tracking-wide">Control plane</label>
          <Select
            selectedKey={cfg.control || 'local'}
            onSelectionChange={k => setCfg(prev => ({ ...prev, control: String(k) }))}
            className="w-64"
          >
            <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
            <Select.Popover>
              <ListBox>
                <ListBox.Item id="kubectl" textValue="kubectl">kubectl — Kubernetes / k3s<ListBox.ItemIndicator /></ListBox.Item>
                <ListBox.Item id="docker" textValue="docker">docker — Docker containers<ListBox.ItemIndicator /></ListBox.Item>
                <ListBox.Item id="local" textValue="local">local — bare metal / LGSM / shell commands<ListBox.ItemIndicator /></ListBox.Item>
                <ListBox.Item id="amp" textValue="amp">amp — CubeCoders AMP<ListBox.ItemIndicator /></ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
          <span className="text-xs text-muted">How dune-admin manages the game server process and broker</span>
        </div>
        )}

        {/* kubectl */}
        {isKubectl && (
          <Field label="Namespace" value={cfg.control_namespace} onChange={set('control_namespace')}
            placeholder="auto-discovered"
            hint="Kubernetes namespace where the Dune workloads run. Leave blank to auto-discover"
            hidden={!matches(q, 'Namespace', 'control_namespace', 'Kubernetes')} />
        )}

        {/* docker */}
        {isDocker && (<>
          <Field label="Game server container" value={cfg.docker_gameserver} onChange={set('docker_gameserver')}
            placeholder="dune-gameserver"
            hint="Container name for the game server process"
            hidden={!matches(q, 'Game server', 'docker_gameserver', 'container')} />
          <Field label="Broker (game) container" value={cfg.docker_broker_game} onChange={set('docker_broker_game')}
            placeholder="dune-mq-game"
            hint="Container name for the game-vhost RabbitMQ broker"
            hidden={!matches(q, 'Broker', 'docker_broker_game', 'container')} />
          <Field label="Broker (admin) container" value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')}
            placeholder="dune-mq-admin"
            hint="Container name for the admin-vhost RabbitMQ broker"
            hidden={!matches(q, 'Broker', 'docker_broker_admin', 'container')} />
          <Field label="Database container" value={cfg.docker_db} onChange={set('docker_db')}
            placeholder="dune-postgres"
            hint="Container name for the PostgreSQL instance"
            hidden={!matches(q, 'Database', 'docker_db', 'postgres', 'container')} />
        </>)}

        {/* local shell commands */}
        {isLocal && (<>
          <Field label="Start command" value={cfg.cmd_start} onChange={set('cmd_start')}
            placeholder="ampinstmgr start DuneAwakening01"
            hint="Shell command to start the game server"
            hidden={!matches(q, 'Start', 'cmd_start', 'shell command')} />
          <Field label="Stop command" value={cfg.cmd_stop} onChange={set('cmd_stop')}
            placeholder="ampinstmgr stop DuneAwakening01"
            hint="Shell command to stop the game server"
            hidden={!matches(q, 'Stop', 'cmd_stop', 'shell command')} />
          <Field label="Restart command" value={cfg.cmd_restart} onChange={set('cmd_restart')}
            placeholder="ampinstmgr restart DuneAwakening01"
            hint="Shell command to restart the game server"
            hidden={!matches(q, 'Restart', 'cmd_restart', 'shell command')} />
          <Field label="Status command" value={cfg.cmd_status} onChange={set('cmd_status')}
            placeholder="ampinstmgr status DuneAwakening01"
            hint="Shell command to query game server status"
            hidden={!matches(q, 'Status', 'cmd_status', 'shell command')} />
        </>)}

        {/* AMP */}
        {isAmp && (<>
          <Field label="Instance name" value={cfg.amp_instance} onChange={set('amp_instance')}
            placeholder="DuneAwakening01"
            hint="AMP instance name used with ampinstmgr, e.g. DuneAwakening01"
            hidden={!matches(q, 'Instance', 'amp_instance', 'ampinstmgr')} />
          <Field label="Container name" value={cfg.amp_container} onChange={set('amp_container')}
            placeholder="AMP_DuneAwakening01"
            hint="Podman container name (default: AMP_<instance>)"
            hidden={!matches(q, 'Container', 'amp_container', 'podman')} />
          <Field label="AMP user" value={cfg.amp_user} onChange={set('amp_user')}
            placeholder="amp"
            hint="OS user that runs AMP — used for sudo elevation and podman exec"
            hidden={!matches(q, 'AMP user', 'amp_user')} />
          <Field label="Log path" value={cfg.amp_log_path} onChange={set('amp_log_path')}
            placeholder="/AMP/duneawakening/logs"
            hint="In-container log directory (used for log streaming)"
            hidden={!matches(q, 'Log path', 'amp_log_path')} />
          <CheckField label="Use container (podman exec)" checked={cfg.amp_use_container}
            onChange={v => setCfg(prev => ({ ...prev, amp_use_container: v }))}
            hint="Enabled: wraps commands in podman exec. Disabled: runs commands as the AMP user on the host directly"
            hidden={!matches(q, 'Use container', 'amp_use_container', 'podman exec')} />
          <Field label="Data root" value={cfg.amp_data_root} onChange={set('amp_data_root')}
            placeholder="/AMP/duneawakening"
            hint="Per-game data root inside the container (default /AMP/duneawakening — the CubeCoders convention)"
            hidden={!matches(q, 'Data root', 'amp_data_root')} />
        </>)}
      </Section>

      {/* Broker */}
      <Section title="RabbitMQ Broker (optional)" hidden={!showBroker}>
        <Field label="Game addr" value={cfg.broker_game_addr} onChange={set('broker_game_addr')}
          placeholder="10.43.48.246:5672"
          hint="RabbitMQ management address for the game vhost — enables capture and notifications"
          hidden={!matches(q, 'Game addr', 'broker_game_addr', 'RabbitMQ')} />
        <Field label="Admin addr" value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')}
          placeholder="10.43.189.193:5672"
          hint="RabbitMQ management address for the admin vhost"
          hidden={!matches(q, 'Admin addr', 'broker_admin_addr', 'RabbitMQ')} />
        <Field label="User" value={cfg.broker_user} onChange={set('broker_user')}
          placeholder="dune_cap"
          hint="RabbitMQ user for both vhosts"
          hidden={!matches(q, 'broker_user', 'RabbitMQ user', 'Broker user')} />
        <Field label="Password" value={cfg.broker_pass} onChange={set('broker_pass')}
          type="password" placeholder={MASKED}
          hint="RabbitMQ password — send the placeholder to keep the existing value"
          hidden={!matches(q, 'broker_pass', 'Password', 'RabbitMQ password')} />
        <Field label="JWT secret" value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')}
          type="password" placeholder={MASKED}
          hint="Base64-encoded HMAC key for re-signing ServiceAuthTokens. Leave blank to use the built-in default"
          hidden={!matches(q, 'JWT', 'broker_jwt_secret', 'HMAC', 'ServiceAuthToken')} />
        <CheckField label="Use TLS (amqps://)" checked={cfg.broker_tls}
          onChange={v => setCfg(prev => ({ ...prev, broker_tls: v }))}
          hint="Enable TLS for all broker connections"
          hidden={!matches(q, 'TLS', 'broker_tls', 'amqps')} />
        <Field label="Exec prefix" value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')}
          placeholder='podman exec AMP_DuneAwakening01'
          hint="Prepended to all rabbitmqctl calls — use when the broker runs inside a container not managed by the docker control plane"
          hidden={!matches(q, 'Exec prefix', 'broker_exec_prefix', 'rabbitmqctl')} />
      </Section>

      {/* Market Bot */}
      <Section title="Market Bot" hidden={!showMarketBot}>
        <CheckField label="Enable embedded bot" checked={cfg.market_bot_enabled}
          onChange={v => setCfg(prev => ({ ...prev, market_bot_enabled: v }))}
          hint="Run the market bot in-process alongside dune-admin (restart required to take effect)"
          hidden={!matches(q, 'market_bot_enabled', 'Enable', 'embedded bot')} />
        <Field label="Remote URL" value={cfg.market_bot_remote_url} onChange={set('market_bot_remote_url')}
          placeholder="http://192.168.0.10:9191"
          hint="Forward market bot API calls to a standalone bot at this URL instead of running one in-process"
          hidden={!matches(q, 'Remote URL', 'market_bot_remote_url', 'standalone')} />
        <Field label="Remote token" value={cfg.market_bot_remote_token} onChange={set('market_bot_remote_token')}
          type="password" placeholder={MASKED}
          hint="Bearer token for authenticating with the remote bot"
          hidden={!matches(q, 'Remote token', 'market_bot_remote_token', 'bearer')} />
        <Field label="Cache DB path" value={cfg.market_bot_cache_db} onChange={set('market_bot_cache_db')}
          placeholder="~/.dune-admin/market-bot-cache.db"
          hint="Path to the SQLite cache database used by the embedded bot"
          hidden={!matches(q, 'Cache DB', 'market_bot_cache_db', 'SQLite')} />
        <Field label="Item data path" value={cfg.market_bot_item_data} onChange={set('market_bot_item_data')}
          placeholder="item-data.json"
          hint="Path to item-data.json — the bot uses this for price lookups"
          hidden={!matches(q, 'Item data', 'market_bot_item_data')} />
        <Field label="State path" value={cfg.market_bot_state} onChange={set('market_bot_state')}
          placeholder="~/.dune-admin/market-bot-state.json"
          hint="Path to the JSON file where the bot persists its runtime state across restarts"
          hidden={!matches(q, 'State path', 'market_bot_state')} />
        <Field label="Buy interval" value={cfg.market_bot_buy_interval} onChange={set('market_bot_buy_interval')}
          placeholder="5m"
          hint="How often the bot checks for buy opportunities (e.g. 5m, 30s)"
          hidden={!matches(q, 'Buy interval', 'market_bot_buy_interval')} />
        <Field label="List interval" value={cfg.market_bot_list_interval} onChange={set('market_bot_list_interval')}
          placeholder="10m"
          hint="How often the bot refreshes its listings"
          hidden={!matches(q, 'List interval', 'market_bot_list_interval')} />
        <Field label="Buy threshold" value={cfg.market_bot_buy_threshold} onChange={set('market_bot_buy_threshold')}
          type="number" placeholder="0.8"
          hint="Minimum discount ratio (0–1) before the bot buys — e.g. 0.8 means buy at 80% of market price or lower"
          hidden={!matches(q, 'Buy threshold', 'market_bot_buy_threshold', 'discount')} />
        <Field label="Max buys" value={cfg.market_bot_max_buys} onChange={set('market_bot_max_buys')}
          type="number" placeholder="10"
          hint="Maximum concurrent buy orders the bot will place"
          hidden={!matches(q, 'Max buys', 'market_bot_max_buys')} />
      </Section>

      {/* Advanced */}
      <Section title="Advanced" hidden={!showAdvanced}>
        <Field label="Listen address ⚠" value={cfg.listen_addr} onChange={set('listen_addr')}
          placeholder=":8080"
          hint="HTTP listen address — changing this requires a full server restart to take effect"
          hidden={!matches(q, 'Listen', 'listen_addr', 'HTTP', 'restart')} />
        <Field label="Backup directory" value={cfg.backup_dir} onChange={set('backup_dir')}
          placeholder="/funcom/artifacts/database-dumps/mybg"
          hint="Path the executor accesses for game backup files (read over SSH when in SSH mode)"
          hidden={!matches(q, 'Backup', 'backup_dir')} />
        <Field label="Server INI directory" value={cfg.server_ini_dir} onChange={set('server_ini_dir')}
          placeholder="/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state"
          hint="Directory containing UserGame.ini and UserOverrides.ini — the Server Settings tab writes here"
          hidden={!matches(q, 'Server INI', 'server_ini_dir', 'UserGame.ini')} />
        <Field label="Default INI directory" value={cfg.default_ini_dir} onChange={set('default_ini_dir')}
          placeholder="/path/to/game/Config"
          hint="Path to DefaultGame.ini / DefaultEngine.ini — the base layer read by the Server Settings tab"
          hidden={!matches(q, 'Default INI', 'default_ini_dir', 'DefaultGame.ini')} />
        <Field label="Director URL" value={cfg.director_url} onChange={set('director_url')}
          placeholder="http://127.0.0.1:11717"
          hint="Optional Battlegroup Director URL — when set, dune-admin proxies /director/ to it"
          hidden={!matches(q, 'Director', 'director_url', 'Battlegroup')} />
        <Field label="Scrip currency ID" value={cfg.scrip_currency} onChange={set('scrip_currency')}
          type="number" placeholder="1"
          hint="Item ID used as the scrip currency in the game economy"
          hidden={!matches(q, 'Scrip', 'scrip_currency', 'currency')} />
      </Section>

      {/* Save */}
      <div className="flex justify-end pt-2">
        <Button onPress={save} isDisabled={saving} size="sm">
          {saving
            ? <><Spinner size="sm" color="current" /> Reconnecting…</>
            : <><Icon name="save" /> Save &amp; Reconnect</>}
        </Button>
      </div>
    </div>
  )
}
```

- [ ] **3.2 Lint the new file**

```bash
cd web && pnpm lint 2>&1 | grep -E "SettingsConfigForm|error"
```

Expected: no errors referencing `SettingsConfigForm.tsx`. Fix any that appear.

- [ ] **3.3 Commit**

```bash
cd .. && git add web/src/components/SettingsConfigForm.tsx
git commit -m "feat: add SettingsConfigForm — searchable config editor with field descriptions"
```

---

## Task 4: Wire into Settings modal + delete orphaned tab

**Files:**

- Modify: `web/src/App.tsx`
- Delete: `web/src/tabs/AppConfigTab.tsx`

### Steps

- [ ] **4.1 Add import to `App.tsx`**

At the top of `web/src/App.tsx`, add after the existing component imports:

```tsx
import SettingsConfigForm from "./components/SettingsConfigForm";
```

- [ ] **4.2 Grow modal to `cover` and add config form section**

In `web/src/App.tsx`, find the Settings modal and make two changes:

**Change 1** — modal container size (change `size="sm"` to `size="lg"`):

```tsx
// Before
<Modal.Container size="sm" scroll="outside">

// After
<Modal.Container size="lg" scroll="outside">
```

**Change 2** — add the config form section inside `Modal.Body`, between the About divider and the Backend URL Override section:

```tsx
{/* Server Configuration */}
<div className="flex flex-col gap-2">
  <span className="text-xs font-medium text-muted uppercase tracking-wider">Server Configuration</span>
  <SettingsConfigForm />
</div>
<div className="border-t border-border" />
```

Place this block after the existing `<div className="border-t border-border" />` that follows the About section, and before the Backend URL Override block.

- [ ] **4.3 Delete the orphaned tab**

```bash
rm web/src/tabs/AppConfigTab.tsx
```

- [ ] **4.4 Build to confirm no dangling references**

```bash
cd web && pnpm lint && pnpm build 2>&1 | tail -10
```

Expected: clean build. If any import of `AppConfigTab` surfaces, find and remove it.

- [ ] **4.5 Commit**

```bash
cd .. && git add web/src/App.tsx && git rm web/src/tabs/AppConfigTab.tsx
git commit -m "feat: wire SettingsConfigForm into Settings modal, remove orphaned AppConfigTab"
```

---

## Task 5: Final verification and local test

### Steps

- [ ] **5.1 Full verify**

```bash
make verify
```

Expected: `All verification checks passed!`

- [ ] **5.2 Run the app and open the Settings modal**

```bash
make dev
```

Open `http://localhost:5173`, click the gear icon. Verify:

- Settings modal opens at `lg` size
- About section shows version, commit, control, build time
- Server Configuration section renders all panels (Database, SSH, Control Plane, Broker, Market Bot, Advanced)
- Typing "password" in the search field shows only password fields across all sections; other panels collapse
- Typing "amp" shows only the AMP sub-fields (only visible when control = amp — switch control to `amp` first)
- Typing "broker" shows only Broker panel fields
- Clicking "Save & Reconnect" with valid values succeeds with toast; invalid DB host shows error toast
- Masked fields (db_pass, broker_pass, broker_jwt_secret, market_bot_remote_token) show `••••••••` placeholder when set
- Submitting with the placeholder in a masked field does not overwrite the real value in config.yaml
- Backend URL Override section below still works independently

- [ ] **5.3 Commit if any polish fixes were needed**

```bash
git add -A && git commit -m "fix: polish config editor after local test"
```

(Skip if no changes needed.)
