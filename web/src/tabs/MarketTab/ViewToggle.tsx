import { Icon } from '../../dune-ui'

export type MarketView = 'grid' | 'table'

type Props = {
  view: MarketView
  onChange: (v: MarketView) => void
}

export default function ViewToggle({ view, onChange }: Props) {
  return (
    <div className="flex rounded border border-border overflow-hidden shrink-0">
      <button
        className={`px-2 py-1 text-sm transition-colors ${view === 'grid' ? 'bg-accent text-background' : 'text-muted hover:text-foreground'}`}
        onClick={() => onChange('grid')}
        aria-label="Grid view"
      >
        <Icon name="layout-grid" />
      </button>
      <button
        className={`px-2 py-1 text-sm transition-colors ${view === 'table' ? 'bg-accent text-background' : 'text-muted hover:text-foreground'}`}
        onClick={() => onChange('table')}
        aria-label="Table view"
      >
        <Icon name="list" />
      </button>
    </div>
  )
}
