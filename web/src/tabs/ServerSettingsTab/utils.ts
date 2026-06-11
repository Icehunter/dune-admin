import type { ServerSetting, RawSection } from '../../api/client'
import { CATEGORY_ORDER, SOURCE_FILE, LAYER_STYLE } from './constants'

const groupByCategory = (items: ServerSetting[]) => {
  const map = new Map<string, ServerSetting[]>()
  for (const item of items) {
    const arr = map.get(item.category) ?? []
    arr.push(item)
    map.set(item.category, arr)
  }
  const ordered: [string, ServerSetting[]][] = []
  for (const cat of CATEGORY_ORDER) {
    if (map.has(cat)) ordered.push([cat, map.get(cat)!])
  }
  for (const [cat, items] of map) {
    if (!CATEGORY_ORDER.includes(cat)) ordered.push([cat, items])
  }
  return ordered
}

const sourceLabel = (s: string) => {
  const file = SOURCE_FILE[s]
  const style = LAYER_STYLE[s]
  if (!file || !style) return null
  return { text: file, cls: style.cls }
}

const shortSection = (section: string) => {
  const dot = section.lastIndexOf('.')
  return dot >= 0 ? section.slice(dot + 1) : section
}

const matchesSetting = (item: ServerSetting, q: string): boolean => {
  if (!q) return true
  return (
    item.label.toLowerCase().includes(q)
    || item.description.toLowerCase().includes(q)
    || item.key.toLowerCase().includes(q)
    || item.category.toLowerCase().includes(q)
    || shortSection(item.section).toLowerCase().includes(q)
  )
}

const matchesRawSection = (sections: RawSection[], q: string): boolean => {
  if (!q) return true
  if (shortSection(sections[0].section).toLowerCase().includes(q)) return true
  return sections.some((sec) =>
    sec.lines.some((l) =>
      l.key.toLowerCase().includes(q) || l.value.toLowerCase().includes(q),
    ),
  )
}

const linesToText = (lines: RawSection['lines']) => {
  return lines.map((l) => `${l.prefix}${l.key}=${l.value}`).join('\n')
}

const trimFloat = (v: string): string => {
  if (!v.includes('.')) return v
  const n = parseFloat(v)
  return isNaN(n) ? v : n.toString()
}

const groupLinesByKey = (lines: RawSection['lines']) => {
  const grouped: { key: string, lines: typeof lines }[] = []
  const seen = new Map<string, number>()
  for (const line of lines) {
    const idx = seen.get(line.key)
    if (idx !== undefined) {
      grouped[idx].lines.push(line)
    }
    else {
      seen.set(line.key, grouped.length)
      grouped.push({ key: line.key, lines: [line] })
    }
  }
  return grouped
}

export {
  groupByCategory,
  sourceLabel,
  shortSection,
  matchesSetting,
  matchesRawSection,
  linesToText,
  trimFloat,
  groupLinesByKey,
}
