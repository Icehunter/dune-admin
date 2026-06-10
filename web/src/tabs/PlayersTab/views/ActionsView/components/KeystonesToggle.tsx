import { useState } from 'react'
import { Button, Disclosure } from '@heroui/react'
import type { KeystoneRow } from '../../../../../api/client'

interface KeystonesToggleProps {
  keystones: KeystoneRow[]
}

export function KeystonesToggle({ keystones }: KeystonesToggleProps) {
  const [open, setOpen] = useState(false)
  return (
    <Disclosure className="mt-0.5" isExpanded={open} onExpandedChange={setOpen}>
      <Disclosure.Heading>
        <Button
          slot="trigger"
          variant="ghost"
          className="text-xs text-muted/70 hover:text-muted flex items-center gap-0.5 px-0 h-auto min-w-0"
        >
          <Disclosure.Indicator />
          {keystones.length}
          {' '}
          keystone
          {keystones.length !== 1 ? 's' : ''}
        </Button>
      </Disclosure.Heading>
      <Disclosure.Content>
        <Disclosure.Body className="flex flex-col gap-0.5 mt-0.5">
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
        </Disclosure.Body>
      </Disclosure.Content>
    </Disclosure>
  )
}
