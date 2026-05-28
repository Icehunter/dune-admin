import { useState } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotStatus } from '../../../api/client'
import { Icon } from '../../../dune-ui'

type Props = {
  status: BotStatus | null
  onRefresh: () => void
}

export default function BotActions({ status, onRefresh }: Props) {
  const [busy, setBusy] = useState<'start' | 'stop' | 'restart' | null>(null)

  const run = async (cmd: 'start' | 'stop' | 'restart') => {
    setBusy(cmd)
    try {
      const res = await api.marketBot.lifecycle(cmd)
      const actionLabel = cmd === 'start' ? 'resume' : cmd === 'stop' ? 'pause' : 'reinitialize'
      toast.success(`Bot ${actionLabel}: ${res.output || 'ok'}`)
      setTimeout(onRefresh, 1500)
    } catch (e: unknown) {
      const actionLabel = cmd === 'start' ? 'resume' : cmd === 'stop' ? 'pause' : 'reinitialize'
      toast.danger(`Failed to ${actionLabel} bot: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setBusy(null)
    }
  }

  const running = status?.running ?? false

  return (
    <div className="flex items-center gap-2">
      <Button
        size="sm"
        variant="outline"
        isDisabled={running || busy !== null}
        onPress={() => run('start')}
      >
        {busy === 'start' ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
        Resume
      </Button>
      <Button
        size="sm"
        variant="danger-soft"
        isDisabled={!running || busy !== null}
        onPress={() => run('stop')}
      >
        {busy === 'stop' ? <Spinner size="sm" color="current" /> : <Icon name="square" />}
        Pause
      </Button>
      <Button
        size="sm"
        variant="ghost"
        isDisabled={busy !== null}
        onPress={() => run('restart')}
      >
        {busy === 'restart' ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        Reinitialize
      </Button>
    </div>
  )
}
