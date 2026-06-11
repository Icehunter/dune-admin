import type { ChipColor } from './types'

/**
 * Map a server / battlegroup phase string to a CSS color from our semantic
 * tokens. Used for the inline-text phase label in the InfoCard and the
 * Phase column of the servers table.
 */
export const phaseColor = (phase: string): string => {
  switch (phase?.toLowerCase()) {
    case 'running': return 'var(--success)'
    case 'reconciling':
    case 'starting':
    case 'initializing': return 'var(--warning)'
    case 'stopping':
    case 'preshutdown':
    case 'terminating': return 'var(--danger)'
    case 'stopped':
    case 'terminated': return 'var(--muted)'
    default: return 'var(--muted)'
  }
}

export type { ChipColor }

/**
 * Map a phase string to a HeroUI Chip colour (the chip variant of [[phaseColor]]).
 * Used for the Server Health status chips and component-health rows.
 */
export const phaseChipColor = (phase: string): ChipColor => {
  switch (phase?.toLowerCase()) {
    case 'running':
    case 'ready':
    case 'connected':
    case 'healthy': return 'success'
    case 'reconciling':
    case 'starting':
    case 'initializing': return 'warning'
    case 'stopping':
    case 'preshutdown':
    case 'terminating':
    case 'disconnected': return 'danger'
    default: return 'default'
  }
}

/** BG uptime = the oldest running game process's age (0 when unknown). */
export const bgUptimeSeconds = (servers: { ageSeconds?: number }[]): number => {
  return servers.reduce((max, s) => Math.max(max, s.ageSeconds ?? 0), 0)
}

/** Game is "ready" only when every running server reports ready. */
export const allServersReady = (phase: string | undefined, servers: { ready: boolean }[]): boolean => {
  return servers.length > 0 && phase === 'Running' && servers.every((s) => s.ready)
}
