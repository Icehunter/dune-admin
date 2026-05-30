// VITE_ICON_BASE_URL: base URL for item icons served from R2 (e.g. https://icons.example.com).
// When set, icons load from <base>/<template_id>.webp.
// When unset, no icon URL is produced and components fall back to category placeholders.
const ICON_BASE = ((import.meta.env.VITE_CDN_BASE_URL as string) ?? 'https://assets.dune.layout.tools')?.replace(
  /\/$/,
  '',
)

export function iconUrl(templateId: string, variant: 'detail' | 'thumb' = 'detail'): string | null {
  if (!ICON_BASE) return null
  return `${ICON_BASE}/${variant}/${templateId}.webp`
}

// Category → colour used when no icon image is available.
const CATEGORY_COLORS: Record<string, string> = {
  'schematics': '#7c6f3e',
  'items/weapons': '#8b3030',
  'items/garment': '#2d5a3d',
  'items/augment': '#1e4a7c',
  'items/utility': '#5a3d7c',
  'items/misc': '#4a4a4a',
  'items/vehicles': '#7c4a1e',
}

export function categoryColor(category: string): string {
  for (const [prefix, color] of Object.entries(CATEGORY_COLORS)) {
    if (category.startsWith(prefix)) return color
  }
  return '#3a3a3a'
}

const QUALITY_LABELS = ['Standard', 'Refined', 'Superior', 'Masterwork', 'Pristine', 'Flawless']

export function qualityLabel(q: number): string {
  return QUALITY_LABELS[q] ?? `Q${q}`
}
