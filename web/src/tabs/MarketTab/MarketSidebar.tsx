import * as React from 'react'
import { Button, SearchField } from '@heroui/react'
import { FileTree } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../dune-ui'
import type { MarketSidebarProps, Node } from './types'

const buildTree = (categories: string[]): { items: Node[], schematics: Node[] } => {
  const itemRoot: Node[] = []
  const schematicRoot: Node[] = []

  for (const cat of [...categories].sort()) {
    const isSchematic = cat.startsWith('schematics/')
    // Strip the top-level prefix before splitting so we don't create a spurious
    // "Schematics" parent node inside the schematics section (or "Items" inside items).
    const stripped = isSchematic
      ? cat.replace(/^schematics\//, '')
      : cat.replace(/^items\//, '')
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

      let node = current.find((n) => n.label === part)
      if (!node) {
        node = { label: part, path: filterPath, displayPath, children: [] }
        current.push(node)
      }
      current = node.children
    }
  }

  return { items: itemRoot, schematics: schematicRoot }
}

const formatLabel = (label: string): string =>
  label
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())

const collectAncestorPaths = (categories: string[], selected: string): Set<string> => {
  const ancestors = new Set<string>()
  for (const cat of categories) {
    if (cat === selected || cat.startsWith(selected + '/') || selected.startsWith(cat + '/')) {
      const parts = cat.replace(/^items\//, '').replace(/^schematics\//, '').split('/')
      let cur = ''
      for (const p of parts) {
        cur = cur ? `${cur}/${p}` : p
        ancestors.add(cur)
      }
    }
  }
  return ancestors
}

// Recursively map a Node tree into nested FileTree.Item elements. The item key
// (`id`) is the full filter path so selection maps straight back onto onSelect.
const renderNode = (node: Node) => (
  <FileTree.Item key={node.displayPath} id={node.path} textValue={node.label} title={formatLabel(node.label)}>
    {node.children.map((child) => renderNode(child))}
  </FileTree.Item>
)

// Prune a tree to nodes whose label (or a descendant's label) matches the query.
// A matching branch keeps all of its descendants so the user can drill in.
const filterNodes = (nodes: Node[], q: string): Node[] => {
  const out: Node[] = []
  for (const node of nodes) {
    const selfMatch = formatLabel(node.label).toLowerCase().includes(q) || node.label.toLowerCase().includes(q)
    if (selfMatch) {
      out.push(node)
      continue
    }
    const kids = filterNodes(node.children, q)
    if (kids.length) out.push({ ...node, children: kids })
  }
  return out
}

const flattenKeys = (nodes: Node[]): string[] =>
  nodes.flatMap((n) => [n.path, ...flattenKeys(n.children)])

export const MarketSidebar: React.FC<MarketSidebarProps> = ({ categories, selected, onSelect }: MarketSidebarProps) => {
  const { t } = useTranslation()
  const { items: allItems, schematics: allSchematics } = React.useMemo(() => buildTree(categories), [categories])
  const [collapsed, setCollapsed] = React.useState(false)
  const [search, setSearch] = React.useState('')

  const q = search.trim().toLowerCase()
  const items = React.useMemo(() => (q ? filterNodes(allItems, q) : allItems), [allItems, q])
  const schematics = React.useMemo(() => (q ? filterNodes(allSchematics, q) : allSchematics), [allSchematics, q])

  // While searching, expand every surviving branch so matches are visible.
  // Otherwise: open top-level nodes plus the ancestors of the selected node.
  const expandedProps = React.useMemo(() => {
    if (q) {
      return { expandedKeys: [...flattenKeys(items), ...flattenKeys(schematics)] }
    }
    const set = new Set<string>()
    for (const node of [...allItems, ...allSchematics]) set.add(node.path)
    const ancestors = collectAncestorPaths(categories, selected)
    const all = (function flatten(nodes: Node[]): Node[] {
      return nodes.flatMap((n) => [n, ...flatten(n.children)])
    })([...allItems, ...allSchematics])
    for (const n of all) {
      if (ancestors.has(n.displayPath)) set.add(n.path)
    }
    return { defaultExpandedKeys: [...set] }
  }, [q, items, schematics, allItems, allSchematics, categories, selected])

  const onSelectionChange = (keys: 'all' | Set<React.Key>) => {
    if (keys === 'all') return
    const k = [...keys][0]
    if (k != null) onSelect(String(k))
  }

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1 shrink-0">
        <Button size="sm" variant="ghost" isIconOnly aria-label={t('market.sidebar.expandAriaLabel')} onPress={() => setCollapsed(false)}>
          <Icon name="chevron-right" />
        </Button>
      </div>
    )
  }

  return (
    <div className="w-56 shrink-0 flex flex-col gap-1 overflow-hidden pr-1">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-muted uppercase tracking-wider">{t('market.sidebar.categories')}</span>
        <Button size="sm" variant="ghost" isIconOnly aria-label={t('market.sidebar.collapseAriaLabel')} onPress={() => setCollapsed(true)}>
          <Icon name="chevron-left" />
        </Button>
      </div>

      <SearchField aria-label={t('market.sidebar.categories')} value={search} onChange={setSearch}>
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={t('market.sidebar.categories')} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

      <Button
        size="sm"
        variant="ghost"
        className={
          'w-full justify-start rounded-[var(--radius)] px-3 font-medium '
          + (selected === ''
            ? 'text-accent'
            : 'text-foreground hover:bg-default/60')
        }
        style={selected === '' ? { backgroundColor: 'color-mix(in srgb, var(--accent) 14%, var(--surface))' } : undefined}
        onPress={() => onSelect('')}
      >
        {t('market.sidebar.allItems')}
      </Button>

      <div className="flex-1 overflow-y-auto">
        {items.length > 0 && (
          <FileTree
            aria-label={t('market.sidebar.categories')}
            selectionMode="single"
            selectedKeys={selected ? new Set([selected]) : new Set()}
            onSelectionChange={onSelectionChange}
            {...expandedProps}
            showGuideLines
            size="sm"
          >
            {items.map((node) => renderNode(node))}
          </FileTree>
        )}

        {schematics.length > 0 && (
          <>
            <div className="my-2 border-t border-border/40" />
            <span className="text-[10px] font-semibold text-muted/60 uppercase tracking-wider px-1 mb-0.5 block">
              {t('market.sidebar.schematics')}
            </span>
            <FileTree
              aria-label={t('market.sidebar.schematics')}
              selectionMode="single"
              selectedKeys={selected ? new Set([selected]) : new Set()}
              onSelectionChange={onSelectionChange}
              {...expandedProps}
              showGuideLines
              size="sm"
            >
              {schematics.map((node) => renderNode(node))}
            </FileTree>
          </>
        )}
      </div>
    </div>
  )
}
