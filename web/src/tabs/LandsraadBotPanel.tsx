import * as React from 'react'
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Switch,
  Input,
  Select,
  SelectItem,
  toast,
  Divider,
} from '@heroui/react'
import { api } from '../api/client'
import type { LandsraadBotConfig, LandsraadDecree, LandsraadTask, GuildSummary } from '../api/client'
import { CreateGuildModal } from './CreateGuildModal'

export interface LandsraadBotPanelProps {
  decrees: LandsraadDecree[]
  tasks: LandsraadTask[]
}

export const LandsraadBotPanel: React.FC<LandsraadBotPanelProps> = ({ decrees, tasks }) => {
  const [config, setConfig] = React.useState<LandsraadBotConfig | null>(null)
  const [guilds, setGuilds] = React.useState<GuildSummary[]>([])
  const [saving, setSaving] = React.useState(false)
  const [modalOpen, setModalOpen] = React.useState(false)

  const load = React.useCallback(async () => {
    try {
      const [cfg, gs] = await Promise.all([
        api.landsraad.bot.getConfig(),
        api.guilds.list()
      ])
      setConfig(cfg)
      setGuilds(gs)
    } catch (e: any) {
      toast.danger(e.message || 'Failed to load bot config')
    }
  }, [])

  React.useEffect(() => {
    load()
  }, [load])

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    try {
      await api.landsraad.bot.saveConfig({
        ...config,
        progress_rate: Number(config.progress_rate) || 100,
        simultaneous_targets: Number(config.simultaneous_targets) || 1,
        target_completion_days: Number(config.target_completion_days) || 3.0,
      })
      toast.success('Bot configuration saved')
      load()
    } catch (e: any) {
      toast.danger(e.message || 'Failed to save config')
    } finally {
      setSaving(false)
    }
  }

  const handleGuildCreated = (id: number) => {
    load()
  }

  if (!config) return null

  return (
    <Card>
      <CardHeader className="flex justify-between">
        <h3 className="text-lg font-bold">Landsraad Bot Configuration</h3>
        <Switch 
          isSelected={config.enabled} 
          onValueChange={v => setConfig(c => c ? { ...c, enabled: v } : null)}
        >
          {config.enabled ? 'Enabled' : 'Disabled'}
        </Switch>
      </CardHeader>
      <Divider />
      <CardBody className="space-y-6">
        <div className="flex items-center gap-4">
          <Input 
            label="Progress Rate (Base %)" 
            type="number"
            value={config.progress_rate.toString()} 
            onChange={e => setConfig(c => c ? { ...c, progress_rate: Number(e.target.value) } : null)}
            className="max-w-xs"
          />
          <Input 
            label="Simultaneous Targets" 
            type="number"
            value={config.simultaneous_targets.toString()} 
            onChange={e => setConfig(c => c ? { ...c, simultaneous_targets: Number(e.target.value) } : null)}
            className="max-w-xs"
          />
          <Input 
            label="Target Completion Days" 
            type="number"
            value={config.target_completion_days.toString()} 
            onChange={e => setConfig(c => c ? { ...c, target_completion_days: Number(e.target.value) } : null)}
            className="max-w-xs"
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          {/* Atreides Settings */}
          <div className="space-y-4 border p-4 rounded-xl border-default-200">
            <h4 className="font-semibold text-primary">House Atreides Bot</h4>
            <div className="flex gap-2">
              <Select 
                label="Bot Guild Identity" 
                selectedKeys={config.atreides_guild_id ? new Set([config.atreides_guild_id.toString()]) : new Set()}
                onSelectionChange={keys => {
                  const k = Array.from(keys)[0] as string
                  if (k) setConfig(c => c ? { ...c, atreides_guild_id: parseInt(k, 10) } : null)
                }}
              >
                {guilds.map(g => <SelectItem key={g.guild_id.toString()} textValue={g.name}>{g.name}</SelectItem>)}
              </Select>
              <Button onPress={() => setModalOpen(true)}>+</Button>
            </div>
            <Select 
              label="Strategy" 
              selectedKeys={new Set([config.atreides_strategy])}
              onSelectionChange={keys => {
                const k = Array.from(keys)[0] as string
                if (k) setConfig(c => c ? { ...c, atreides_strategy: k } : null)
              }}
            >
              <SelectItem key="auto" textValue="Auto (Adaptive)">Auto (Adaptive)</SelectItem>
              <SelectItem key="assist" textValue="Focus Assist Players">Focus Assist Players</SelectItem>
              <SelectItem key="block" textValue="Focus Block Enemy">Focus Block Enemy</SelectItem>
              <SelectItem key="manual" textValue="Manual Override">Manual Override</SelectItem>
            </Select>
            {config.atreides_strategy === 'manual' && (
              <Select 
                label="Target Task" 
                selectedKeys={config.atreides_target_task ? new Set([config.atreides_target_task.toString()]) : new Set()}
                onSelectionChange={keys => {
                  const k = Array.from(keys)[0] as string
                  if (k) setConfig(c => c ? { ...c, atreides_target_task: parseInt(k, 10) } : null)
                }}
              >
                {tasks.map(t => <SelectItem key={t.id.toString()} textValue={`Task ${t.id}`}>Task {t.id} (Board: {t.board_index})</SelectItem>)}
              </Select>
            )}
            <Select 
              label="Force Vote Decree (Optional)" 
              selectedKeys={config.atreides_target_decree ? new Set([config.atreides_target_decree.toString()]) : new Set()}
              onSelectionChange={keys => {
                const k = Array.from(keys)[0] as string
                if (k) setConfig(c => c ? { ...c, atreides_target_decree: parseInt(k, 10) } : null)
              }}
            >
              <SelectItem key="0" textValue="None">None</SelectItem>
              {decrees.map(d => <SelectItem key={d.id.toString()} textValue={d.name}>{d.name}</SelectItem>)}
            </Select>
          </div>

          {/* Harkonnen Settings */}
          <div className="space-y-4 border p-4 rounded-xl border-default-200">
            <h4 className="font-semibold text-danger">House Harkonnen Bot</h4>
            <div className="flex gap-2">
              <Select 
                label="Bot Guild Identity" 
                selectedKeys={config.harkonnen_guild_id ? new Set([config.harkonnen_guild_id.toString()]) : new Set()}
                onSelectionChange={keys => {
                  const k = Array.from(keys)[0] as string
                  if (k) setConfig(c => c ? { ...c, harkonnen_guild_id: parseInt(k, 10) } : null)
                }}
              >
                {guilds.map(g => <SelectItem key={g.guild_id.toString()} textValue={g.name}>{g.name}</SelectItem>)}
              </Select>
              <Button onPress={() => setModalOpen(true)}>+</Button>
            </div>
            <Select 
              label="Strategy" 
              selectedKeys={new Set([config.harkonnen_strategy])}
              onSelectionChange={keys => {
                const k = Array.from(keys)[0] as string
                if (k) setConfig(c => c ? { ...c, harkonnen_strategy: k } : null)
              }}
            >
              <SelectItem key="auto" textValue="Auto (Adaptive)">Auto (Adaptive)</SelectItem>
              <SelectItem key="assist" textValue="Focus Assist Players">Focus Assist Players</SelectItem>
              <SelectItem key="block" textValue="Focus Block Enemy">Focus Block Enemy</SelectItem>
              <SelectItem key="manual" textValue="Manual Override">Manual Override</SelectItem>
            </Select>
            {config.harkonnen_strategy === 'manual' && (
              <Select 
                label="Target Task" 
                selectedKeys={config.harkonnen_target_task ? new Set([config.harkonnen_target_task.toString()]) : new Set()}
                onSelectionChange={keys => {
                  const k = Array.from(keys)[0] as string
                  if (k) setConfig(c => c ? { ...c, harkonnen_target_task: parseInt(k, 10) } : null)
                }}
              >
                {tasks.map(t => <SelectItem key={t.id.toString()} textValue={`Task ${t.id}`}>Task {t.id} (Board: {t.board_index})</SelectItem>)}
              </Select>
            )}
            <Select 
              label="Force Vote Decree (Optional)" 
              selectedKeys={config.harkonnen_target_decree ? new Set([config.harkonnen_target_decree.toString()]) : new Set()}
              onSelectionChange={keys => {
                const k = Array.from(keys)[0] as string
                if (k) setConfig(c => c ? { ...c, harkonnen_target_decree: parseInt(k, 10) } : null)
              }}
            >
              <SelectItem key="0" textValue="None">None</SelectItem>
              {decrees.map(d => <SelectItem key={d.id.toString()} textValue={d.name}>{d.name}</SelectItem>)}
            </Select>
          </div>
        </div>
        
        <div className="flex justify-end mt-4">
          <Button color="primary" isLoading={saving} onPress={handleSave}>Save Config</Button>
        </div>
      </CardBody>

      <CreateGuildModal isOpen={modalOpen} onClose={() => setModalOpen(false)} onSuccess={handleGuildCreated} />
    </Card>
  )
}
