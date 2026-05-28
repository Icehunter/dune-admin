import { useState } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotStatus } from '../../../api/client'
import { Icon, ConfirmDialog } from '../../../dune-ui'

type Props = {
  status: BotStatus | null
  onRefresh: () => void
}

type BusyOp = 'start' | 'stop' | 'restart' | 'cleanup'

export default function BotActions({ status, onRefresh }: Props) {
  const [busy, setBusy] = useState<BusyOp | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

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

  const runCleanup = async () => {
    setConfirmOpen(false)
    setBusy('cleanup')
    try {
      const res = await api.marketBot.cleanup()
      toast.success(`Wiped ${res.orders_deleted} listings (${res.items_deleted} items)`)
      setTimeout(onRefresh, 1500)
    } catch (e: unknown) {
      toast.danger(`Cleanup failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setBusy(null)
    }
  }

  const running = status?.running ?? false

  return (
    <>
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
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy !== null}
          onPress={() => setConfirmOpen(true)}
        >
          {busy === 'cleanup' ? <Spinner size="sm" color="current" /> : <Icon name="trash-2" />}
          Wipe Listings
        </Button>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        title="Wipe all bot listings?"
        description="This deletes every active Revy listing on the exchange. Player listings, fulfilled-order history, and Revy's Solari balance are untouched. The next list tick will repopulate listings from the catalog."
        confirmLabel="Wipe Listings"
        onConfirm={runCleanup}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  )
}
