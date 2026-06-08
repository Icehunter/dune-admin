import type React from 'react'
import { useState, useEffect, useRef } from 'react'
import { ListBox } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'
import { THEMES, applyTheme, loadTheme, type ThemeId } from '../theme'

// Header appearance-theme picker (#144). Mirrors LanguageSelector: an icon button
// that opens a ListBox of presets, with click-outside to close.
export const ThemeSelector: React.FC = () => {
  const { t } = useTranslation()
  const [current, setCurrent] = useState<ThemeId>(loadTheme)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        className="flex items-center justify-center w-8 h-8 rounded text-muted hover:text-foreground hover:bg-surface-secondary transition-colors"
        aria-label={t('app.selectTheme')}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="palette" />
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[170px] rounded-[var(--radius)] border border-border bg-surface shadow-lg overflow-hidden">
          <ListBox
            aria-label={t('app.selectTheme')}
            onAction={(key) => {
              const id = String(key) as ThemeId
              applyTheme(id)
              setCurrent(id)
              setOpen(false)
            }}
          >
            {THEMES.map((th) => (
              <ListBox.Item
                key={th.id}
                id={th.id}
                textValue={th.label}
                className={`flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-surface-hover ${current === th.id ? 'text-accent font-medium' : 'text-foreground'}`}
              >
                {/* Swatch shows the theme's literal accent colour — an intentional
                    colour sample, not a themed UI element. */}
                <span className="w-3 h-3 rounded-full border border-border shrink-0" style={{ background: th.swatch }} />
                <span>{th.label}</span>
              </ListBox.Item>
            ))}
          </ListBox>
        </div>
      )}
    </div>
  )
}
