import { Button } from '@heroui/react'
import type { Player } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import { StatusDot } from './StatusDot'

interface Props {
  player: Player
  selected: boolean
  onSelect: (player: Player) => void
  onAction: (player: Player, action: 'inventory' | 'give' | 'actions') => void
}

export function PlayerCard({ player, selected, onSelect, onAction }: Props) {
  return (
    <button
      type="button"
      onClick={() => onSelect(player)}
      className={[
        'w-full text-left px-3 py-2 rounded-[var(--radius)] flex items-center gap-3',
        'border transition-colors cursor-pointer',
        selected
          ? 'bg-surface border-accent/60'
          : 'bg-surface-secondary border-border hover:border-border/80 hover:bg-surface',
      ].join(' ')}
    >
      <StatusDot status={player.online_status} />
      <div className="flex-1 min-w-0">
        <div className="font-semibold text-sm truncate">{player.name}</div>
        <div className="text-xs text-muted truncate">
          {player.class}
          {' · '}
          {player.map}
        </div>
      </div>
      <div className="flex gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'inventory')}>
          <Icon name="package" />
        </Button>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'give')}>
          <Icon name="gift" />
        </Button>
        <Button size="sm" variant="ghost" isIconOnly onPress={() => onAction(player, 'actions')}>
          <Icon name="settings" />
        </Button>
      </div>
    </button>
  )
}
