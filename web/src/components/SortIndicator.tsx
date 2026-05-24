import type { SortDir } from '../hooks/useTableSort'

export function SortIndicator({ active, dir }: { active: boolean; dir: SortDir }) {
  return (
    <span style={{ marginLeft: 4, opacity: active ? 1 : 0.25 }}>
      {active ? (dir === 'asc' ? '▲' : '▼') : '▲'}
    </span>
  )
}
