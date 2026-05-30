import { Chip } from '@heroui/react'
import type { BotStatus } from '../../../api/client'

function fmt(ts: string | null | undefined): string {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleTimeString()
  }
  catch {
    return ts
  }
}

function fmtBalance(n: number | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString()
}

export default function BotStatusCard({ status }: { status: BotStatus }) {
  const statusLabel = status.running ? '● Running' : '⏸ Paused'
  const statusColor = status.running ? 'success' : 'warning'

  return (
    <div className="flex flex-wrap gap-4 items-start">
      <div className="flex flex-col gap-1 min-w-[120px]">
        <span className="text-xs text-muted uppercase tracking-wider">Status</span>
        <Chip
          size="sm"
          color={statusColor}
          variant="soft"
        >
          {statusLabel}
        </Chip>
      </div>

      <Stat label="Uptime" value={status.uptime || '—'} />
      <Stat label="Listings" value={status.listing_count?.toLocaleString() ?? '—'} />
      <Stat label="Balance" value={fmtBalance(status.balance)} />
      <Stat label="Errors" value={String(status.error_count ?? 0)} accent={status.error_count > 0 ? 'danger' : undefined} />
      <Stat label="Last List Tick" value={fmt(status.last_list_tick)} />
      <Stat label="Last Buy Tick" value={fmt(status.last_buy_tick)} />
      {status.next_list_tick != null && <Stat label="Next List Tick" value={fmt(status.next_list_tick)} />}
      {status.next_buy_tick != null && <Stat label="Next Buy Tick" value={fmt(status.next_buy_tick)} />}
    </div>
  )
}

function Stat({ label, value, accent }: { label: string, value: string, accent?: 'danger' }) {
  return (
    <div className="flex flex-col gap-1 min-w-[100px]">
      <span className="text-xs text-muted uppercase tracking-wider">{label}</span>
      <span className={`text-sm font-mono ${accent === 'danger' ? 'text-danger' : 'text-foreground'}`}>{value}</span>
    </div>
  )
}
