import * as React from 'react'
import type { DiffStatusProps } from './interfaces'

export const DiffStatus: React.FC<DiffStatusProps> = ({ diff }) => {
  const parts: { key: string, text: string, cls: string }[] = []
  if (diff.added > 0) parts.push({ key: 'added', text: `${diff.added} added`, cls: 'text-success' })
  if (diff.updated > 0) parts.push({ key: 'updated', text: `${diff.updated} updated`, cls: 'text-warning' })
  if (diff.removed > 0) parts.push({ key: 'removed', text: `${diff.removed} removed`, cls: 'text-danger' })
  if (parts.length === 0) return null
  return (
    <span className="text-xs flex items-center gap-1">
      {parts.map((p, i) => (
        <span key={p.key} className="flex items-center gap-1">
          {i > 0 && <span className="text-muted">·</span>}
          <span className={p.cls}>{p.text}</span>
        </span>
      ))}
    </span>
  )
}
