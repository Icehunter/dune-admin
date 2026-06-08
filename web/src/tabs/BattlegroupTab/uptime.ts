/**
 * formatUptime turns an elapsed-seconds count into a compact human label
 * ("73s", "15m", "1h 15m", "18d 4h"). Pure — takes the value, never reads the
 * clock — so it's stable in render and trivially testable. Returns "—" for 0 /
 * missing values (e.g. control planes that don't source process age).
 */
export function formatUptime(seconds?: number): string {
  if (!seconds || seconds <= 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return h > 0 ? `${d}d ${h}h` : `${d}d`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m`
  return `${s}s`
}

/**
 * portRange collapses the running servers' UDP ports into a single label —
 * "7777" for one, "7777–7810" for a span, "—" when none are known.
 */
export function portRange(ports: number[]): string {
  const valid = ports.filter((p) => p > 0).sort((a, b) => a - b)
  if (valid.length === 0) return '—'
  const lo = valid[0]
  const hi = valid[valid.length - 1]
  return lo === hi ? `${lo}` : `${lo}–${hi}`
}
