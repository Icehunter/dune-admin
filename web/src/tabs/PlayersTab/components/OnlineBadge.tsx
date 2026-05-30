import { Chip } from '@heroui/react'

export function OnlineBadge({ status }: { status: string }) {
  const color
    = status === 'Online'
      ? 'success'
      : status === 'LoggingOut'
        ? 'warning'
        : 'default'
  const label
    = status === 'Online'
      ? 'Online'
      : status === 'LoggingOut'
        ? 'LoggingOut'
        : status || 'Offline'
  return <Chip size="sm" color={color} variant="soft">{label}</Chip>
}
