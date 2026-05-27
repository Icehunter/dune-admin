import { useMemo, useState } from 'react'
import { Button } from '@heroui/react'
import { Icon } from '../../dune-ui'

type Props = {
  categories: string[]
  selected: string
  onSelect: (cat: string) => void
}

type Node = {
  label: string
  path: string        // full path used for filtering
  displayPath: string // path used as tree key (items/ stripped)
  children: Node[]
}

function buildTree(categories: string[]): { items: Node[]; schematics: Node[] } {
  const itemRoot: Node[] = []
  const schematicRoot: Node[] = []

  for (const cat of [...categories].sort()) {
    const isSchematic = cat.startsWith('schematics/')
    const stripped = isSchematic ? cat : cat.replace(/^items\//, '')
    const parts = stripped.split('/')
    const root = isSchematic ? schematicRoot : itemRoot

    let current = root
    let displayPath = ''
    let filterPath = ''
    for (const part of parts) {
      displayPath = displayPath ? `${displayPath}/${part}` : part
      filterPath = isSchematic
        ? (filterPath ? `${filterPath}/${part}` : `schematics/${part}`)
        : (filterPath ? `${filterPath}/${part}` : `items/${part}`)

      let node = current.find(n => n.label === part)
      if (!node) {
        node = { label: part, path: filterPath, displayPath, children: [] }
        current.push(node)
      }
      current = node.children
    }
  }

  return { items: itemRoot, schematics: schematicRoot }
}

function formatLabel(label: string): string {
  return label
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
}

function collectAncestorPaths(categories: string[], selected: string): Set<string> {
  const ancestors = new Set<string>()
  for (const cat of categories) {
    if (cat === selected || cat.startsWith(selected + '/') || selected.startsWith(cat + '/')) {
      const parts = cat.replace(/^items\//, '').split('/')
      let cur = ''
      for (const p of parts) {
        cur = cur ? `${cur}/${p}` : p
        ancestors.add(cur)
      }
    }
  }
  return ancestors
}

type TreeNodeProps = {
  node: Node
  selected: string
  depth: number
  expanded: Set<string>
  onToggle: (displayPath: string) => void
  onSelect: (path: string) => void
}

function TreeNode({ node, selected, depth, expanded, onToggle, onSelect }: TreeNodeProps) {
  const isExact = selected === node.path
  const isAncestor = !isExact && selected.startsWith(node.path + '/')
  const hasChildren = node.children.length > 0
  const isOpen = expanded.has(node.displayPath)

  return (
    <div>
      <div
        className={[
          'group flex items-center rounded transition-colors',
          isExact ? 'bg-accent/15' : 'hover:bg-surface',
        ].join(' ')}
        style={{ paddingLeft: `${depth * 12}px` }}
      >
        {/* Expand/collapse toggle — only shown for nodes with children */}
        {hasChildren ? (
          <button
            className="flex items-center justify-center w-5 h-5 shrink-0 text-muted hover:text-foreground transition-colors"
            onClick={e => { e.stopPropagation(); onToggle(node.displayPath) }}
            aria-label={isOpen ? 'Collapse' : 'Expand'}
          >
            <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} />
          </button>
        ) : (
          <span className="w-5 shrink-0 flex items-center justify-center">
            <span className="w-1 h-1 rounded-full bg-border/60" />
          </span>
        )}

        {/* Label button */}
        <button
          className={[
            'flex-1 text-left py-1 pr-2 text-sm truncate',
            isExact ? 'text-accent font-medium' : isAncestor ? 'text-foreground/80' : 'text-muted',
          ].join(' ')}
          onClick={() => { onSelect(node.path); if (hasChildren && !isOpen) onToggle(node.displayPath) }}
        >
          {formatLabel(node.label)}
        </button>
      </div>

      {/* Children with left border guide */}
      {hasChildren && isOpen && (
        <div className="relative">
          <div
            className="absolute top-0 bottom-0 border-l border-border/30"
            style={{ left: `${depth * 12 + 10}px` }}
          />
          {node.children.map(child => (
            <TreeNode
              key={child.displayPath}
              node={child}
              selected={selected}
              depth={depth + 1}
              expanded={expanded}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export default function MarketSidebar({ categories, selected, onSelect }: Props) {
  const { items, schematics } = useMemo(() => buildTree(categories), [categories])
  const [collapsed, setCollapsed] = useState(false)

  // Default: top-level nodes open. Auto-expand ancestors of selected node.
  const defaultExpanded = useMemo(() => {
    const set = new Set<string>()
    for (const node of [...items, ...schematics]) set.add(node.displayPath)
    for (const p of collectAncestorPaths(categories, selected)) set.add(p)
    return set
  }, [items, schematics, categories, selected])

  const [expanded, setExpanded] = useState<Set<string>>(defaultExpanded)

  const toggle = (displayPath: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(displayPath)) next.delete(displayPath)
      else next.add(displayPath)
      return next
    })
  }

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1 shrink-0">
        <Button size="sm" variant="ghost" isIconOnly aria-label="Expand sidebar" onPress={() => setCollapsed(false)}>
          <Icon name="chevron-right" />
        </Button>
      </div>
    )
  }

  return (
    <div className="w-48 shrink-0 flex flex-col gap-0.5 overflow-y-auto pr-1">
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-semibold text-muted uppercase tracking-wider">Categories</span>
        <Button size="sm" variant="ghost" isIconOnly aria-label="Collapse sidebar" onPress={() => setCollapsed(true)}>
          <Icon name="chevron-left" />
        </Button>
      </div>

      <Button
        size="sm"
        variant={selected === '' ? 'primary' : 'ghost'}
        className="w-full justify-start text-sm mb-1"
        onPress={() => onSelect('')}
      >
        All Items
      </Button>

      {items.map(node => (
        <TreeNode
          key={node.displayPath}
          node={node}
          selected={selected}
          depth={0}
          expanded={expanded}
          onToggle={toggle}
          onSelect={onSelect}
        />
      ))}

      {schematics.length > 0 && (
        <>
          <div className="my-2 border-t border-border/40" />
          <span className="text-[10px] font-semibold text-muted/60 uppercase tracking-wider px-1 mb-0.5">
            Schematics
          </span>
          {schematics.map(node => (
            <TreeNode
              key={node.displayPath}
              node={node}
              selected={selected}
              depth={0}
              expanded={expanded}
              onToggle={toggle}
              onSelect={node.path.startsWith('schematics/') ? onSelect : onSelect}
            />
          ))}
        </>
      )}
    </div>
  )
}
