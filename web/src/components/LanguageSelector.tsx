import { useState, useEffect, useRef } from 'react'
import { ListBox } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { LANGUAGES, setLocale, LOCALE_KEY, DEFAULT_LOCALE } from '../i18n'

export function LanguageSelector() {
  const { t } = useTranslation()
  const [current, setCurrent] = useState(
    localStorage.getItem(LOCALE_KEY) ?? DEFAULT_LOCALE,
  )
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const selected = LANGUAGES.find((l) => l.code === current) ?? LANGUAGES[0]

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        className="flex items-center justify-center w-8 h-8 rounded hover:bg-surface-secondary transition-colors text-base"
        aria-label={t('app.selectLanguage')}
        onClick={() => setOpen((v) => !v)}
      >
        {selected.flag}
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[180px] rounded-[var(--radius)] border border-border bg-surface shadow-lg overflow-hidden">
          <ListBox
            aria-label={t('app.selectLanguage')}
            onAction={(key) => {
              const code = String(key)
              setLocale(code)
              setCurrent(code)
              setOpen(false)
            }}
          >
            {LANGUAGES.map((lang) => (
              <ListBox.Item
                key={lang.code}
                id={lang.code}
                textValue={lang.label}
                className={`flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-surface-hover ${current === lang.code ? 'text-accent font-medium' : 'text-foreground'}`}
              >
                <span>{lang.flag}</span>
                <span>{lang.label}</span>
              </ListBox.Item>
            ))}
          </ListBox>
        </div>
      )}
    </div>
  )
}
