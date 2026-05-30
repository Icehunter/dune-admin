/**
 * Map a server / battlegroup phase string to a CSS color from our semantic
 * tokens. Used for the inline-text phase label in the InfoCard and the
 * Phase column of the servers table.
 */
export function phaseColor(phase: string): string {
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
