import * as React from 'react'
import { Dropdown, Button } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'
import { THEMES, applyTheme, loadTheme, type ThemeId } from '../theme'

export const ThemeSelector: React.FC = () => {
  const { t } = useTranslation()
  const [current, setCurrent] = React.useState<ThemeId>(loadTheme)

  return (
    <Dropdown>
      <Button
        isIconOnly
        variant="ghost"
        size="sm"
        aria-label={t('app.selectTheme')}
        className="w-8 h-8 min-w-0 text-muted data-[hover=true]:text-foreground data-[hover=true]:bg-surface-secondary"
      >
        <Icon name="palette" />
      </Button>
      <Dropdown.Popover>
        <Dropdown.Menu
          aria-label={t('app.selectTheme')}
          selectionMode="single"
          selectedKeys={new Set([current])}
          onSelectionChange={(keys) => {
            if (keys === 'all') return
            const id = [...keys][0] as ThemeId
            if (id) {
              applyTheme(id)
              setCurrent(id)
            }
          }}
        >
          {THEMES.map((th) => (
            <Dropdown.Item key={th.id} id={th.id} textValue={th.label}>
              <span className="flex items-center gap-2">
                {/* Literal color sample — intentional, not a themed element */}
                <span className="w-3 h-3 rounded-full border border-border shrink-0" style={{ background: th.swatch }} />
                <span>{th.label}</span>
              </span>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  )
}
