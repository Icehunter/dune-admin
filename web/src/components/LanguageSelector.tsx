import * as React from 'react'
import { Dropdown, Button } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { LANGUAGES, setLocale, LOCALE_KEY, DEFAULT_LOCALE } from '../i18n'

export const LanguageSelector: React.FC = () => {
  const { t } = useTranslation()
  const [current, setCurrent] = React.useState(
    localStorage.getItem(LOCALE_KEY) ?? DEFAULT_LOCALE,
  )

  const selected = LANGUAGES.find((l) => l.code === current) ?? LANGUAGES[0]

  return (
    <Dropdown>
      <Button
        isIconOnly
        variant="ghost"
        size="sm"
        aria-label={t('app.selectLanguage')}
        className="w-8 h-8 min-w-0 text-base data-[hover=true]:bg-surface-secondary"
      >
        {selected.flag}
      </Button>
      <Dropdown.Popover>
        <Dropdown.Menu
          aria-label={t('app.selectLanguage')}
          selectionMode="single"
          selectedKeys={new Set([current])}
          onSelectionChange={(keys) => {
            if (keys === 'all') return
            const code = [...keys][0] as string
            if (code) {
              setLocale(code)
              setCurrent(code)
            }
          }}
        >
          {LANGUAGES.map((lang) => (
            <Dropdown.Item key={lang.code} id={lang.code} textValue={lang.label}>
              <span className="flex items-center gap-2">
                <span>{lang.flag}</span>
                <span>{lang.label}</span>
              </span>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  )
}
