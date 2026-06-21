import * as React from 'react'
import type { ItemDetailCardRowProps } from './interfaces'

export const Row: React.FC<ItemDetailCardRowProps> = ({ label, value, accent, wrap }) => (
  <div className={`flex text-xs py-0.5 ${wrap ? 'flex-col gap-0.5' : 'items-center justify-between'}`}>
    <span className="text-muted shrink-0">{label}</span>
    <span className={accent ? 'font-mono text-accent font-semibold' : 'text-foreground'}>{value}</span>
  </div>
)
