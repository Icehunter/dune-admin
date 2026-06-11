import * as React from 'react'
import { StatusDot } from './StatusDot'
import type { PlayerCardProps } from './types'

export const PlayerCard: React.FC<PlayerCardProps> = ({ player, selected, onSelect }) => {
  return (
    <button
      type="button"
      onClick={() => onSelect(player)}
      className={[
        'w-full text-left px-3 py-2 rounded-[var(--radius)] flex items-center gap-3',
        'text-sm transition-colors cursor-pointer',
        selected
          ? 'bg-accent text-accent-foreground font-semibold'
          : 'text-foreground hover:bg-surface-hover',
      ].join(' ')}
    >
      <StatusDot status={player.online_status} />
      <div className="flex-1 min-w-0">
        <div className="font-semibold text-sm truncate">{player.name}</div>
        <div className={`text-xs truncate ${selected ? 'opacity-80' : 'text-muted'}`}>
          {player.class}
          {' · '}
          {player.map}
        </div>
      </div>
    </button>
  )
}
