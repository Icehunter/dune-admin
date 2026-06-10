// Appearance theme presets (#144). Each preset recolours a curated set of
// hue-carrying CSS custom properties over the shared dark Dune base; 'spice' is
// the default :root palette (no overrides). Themes are applied by writing the
// vars onto document.documentElement and persisted in localStorage, so the choice
// survives reloads and applies before first paint (see main.tsx).

export type ThemeId = 'spice' | 'atreides' | 'harkonnen' | 'fremen'

// Display list + a preview swatch (the theme's accent colour) for the selector.
export const THEMES: { id: ThemeId, label: string, swatch: string }[] = [
  { id: 'spice', label: 'Spice', swatch: '#c9820a' },
  { id: 'atreides', label: 'Atreides', swatch: '#2f9e5e' },
  { id: 'harkonnen', label: 'Harkonnen', swatch: '#cf2b2b' },
  { id: 'fremen', label: 'Fremen', swatch: '#2f88c9' },
]

const THEME_KEY = 'dune_admin_theme'

// Every token any preset may override — cleared before applying a new theme so
// switching back to 'spice' (which overrides nothing) reverts to the :root values.
// Covers the full surface ramp (background → surface → elevated panels) + borders
// + accent family, so the whole UI (page, cards, modals, inputs) tints with the
// theme rather than just the accent. --foreground is left at the :root cream for
// readability on every dark tint.
const OVERRIDE_KEYS = [
  '--accent', '--accent-foreground', '--focus', '--link',
  '--background', '--surface', '--surface-alt', '--overlay', '--field-background',
  '--surface-secondary', '--surface-tertiary', '--surface-hover',
  '--border', '--separator', '--field-border',
  '--accent-soft-bg', '--accent-soft-border', '--scrollbar', '--muted',
  '--default',
]

const PALETTES: Record<ThemeId, Record<string, string>> = {
  spice: {},
  atreides: {
    '--accent': '#2f9e5e', '--accent-foreground': '#06140c', '--focus': '#46c47e', '--link': '#46c47e',
    '--background': '#07120b', '--surface': '#0f1c14', '--surface-alt': '#0b160f',
    '--overlay': '#0f1c14', '--field-background': '#07120b',
    '--surface-secondary': '#13241b', '--surface-tertiary': '#1c4631', '--surface-hover': '#13241b',
    '--border': '#1c4631', '--separator': '#13241b', '--field-border': '#2b6b48',
    '--accent-soft-bg': '#13241b', '--accent-soft-border': '#2b6b48', '--scrollbar': '#2b6b48', '--muted': '#7c917f',
    '--default': '#0f1c14',
  },
  harkonnen: {
    '--accent': '#cf2b2b', '--accent-foreground': '#ffffff', '--focus': '#e85555', '--link': '#e85555',
    '--background': '#120707', '--surface': '#1c0f0f', '--surface-alt': '#160b0b',
    '--overlay': '#1c0f0f', '--field-background': '#120707',
    '--surface-secondary': '#241010', '--surface-tertiary': '#4a1818', '--surface-hover': '#241010',
    '--border': '#4a1818', '--separator': '#241010', '--field-border': '#6b2b2b',
    '--accent-soft-bg': '#241010', '--accent-soft-border': '#6b2b2b', '--scrollbar': '#6b2b2b', '--muted': '#9a7a7a',
    '--default': '#1c0f0f',
  },
  fremen: {
    '--accent': '#2f88c9', '--accent-foreground': '#06121c', '--focus': '#4aa6e8', '--link': '#4aa6e8',
    '--background': '#07101a', '--surface': '#0f1a26', '--surface-alt': '#0b1420',
    '--overlay': '#0f1a26', '--field-background': '#07101a',
    '--surface-secondary': '#101e2a', '--surface-tertiary': '#18374e', '--surface-hover': '#101e2a',
    '--border': '#18374e', '--separator': '#101e2a', '--field-border': '#2b577b',
    '--accent-soft-bg': '#101e2a', '--accent-soft-border': '#2b577b', '--scrollbar': '#2b577b', '--muted': '#708a9a',
    '--default': '#0f1a26',
  },
}

export function loadTheme(): ThemeId {
  const id = localStorage.getItem(THEME_KEY)
  return id === 'atreides' || id === 'harkonnen' || id === 'fremen' ? id : 'spice'
}

export function applyTheme(id: ThemeId): void {
  const root = document.documentElement
  for (const k of OVERRIDE_KEYS) root.style.removeProperty(k)
  for (const [k, v] of Object.entries(PALETTES[id] ?? {})) root.style.setProperty(k, v)
  localStorage.setItem(THEME_KEY, id)
}
