import { useState } from 'react'
import type { KeystoneRow } from '../../../../../api/client'

interface KeystonesToggleProps {
  keystones: KeystoneRow[]
}

export function KeystonesToggle({ keystones }: KeystonesToggleProps) {
  const [open, setOpen] = useState(false)
  return (
    <div className="mt-0.5">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="text-xs text-muted/70 hover:text-muted flex items-center gap-0.5"
      >
        <span>{open ? '▾' : '▸'}</span>
        {keystones.length}
        {' '}
        keystone
        {keystones.length !== 1 ? 's' : ''}
      </button>
      {open && (
        <div className="flex flex-col gap-0.5 mt-0.5">
          {keystones.map((k) => (
            <span key={k.id} className="text-xs font-mono text-muted">
              ↳
              {' '}
              {k.name.replace(/^DA_\w+Keystone_/, '').replace(/_/g, ' ')}
              {k.cost > 0 && (
                <span className="ml-1 text-muted/60">
                  {k.cost}
                  m
                </span>
              )}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
