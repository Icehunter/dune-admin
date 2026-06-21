import * as React from 'react'
import type { WelcomeConfigDiff } from '../interfaces'

export const DiffStatus: React.FC<{ diff: WelcomeConfigDiff }> = ({ diff }) => {
  const parts: { key: string, text: string, cls: string }[] = []
  if (diff.settingsChanged) parts.push({ key: 'settings', text: 'settings changed', cls: 'text-warning' })
  if (diff.packageAdded > 0) parts.push({ key: 'added', text: `${diff.packageAdded} added`, cls: 'text-success' })
  if (diff.packageUpdated > 0) parts.push({ key: 'updated', text: `${diff.packageUpdated} updated`, cls: 'text-warning' })
  if (diff.packageRemoved > 0) parts.push({ key: 'removed', text: `${diff.packageRemoved} removed`, cls: 'text-danger' })
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
