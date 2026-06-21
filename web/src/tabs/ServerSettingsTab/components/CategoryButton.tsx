import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../../dune-ui'
import { CATEGORY_ICONS, CATEGORY_LABELS, USER_SOURCES } from '../constants'
import type { CategoryButtonProps } from './interfaces'

export const CategoryButton: React.FC<CategoryButtonProps> = ({
  cat, catItems, isOpen, onToggle,
}) => {
  const { t } = useTranslation()
  const overrideCount = catItems.filter((i) =>
    i.layers.some((l) => USER_SOURCES.has(l.source)),
  ).length

  return (
    <button
      onClick={() => onToggle(cat)}
      className={`flex items-center gap-2 rounded border px-3 py-2.5 text-left transition-colors w-full ${
        isOpen
          ? 'bg-accent/15 border-accent/60 text-foreground'
          : 'bg-surface border-border/60 hover:bg-surface-secondary hover:border-border text-foreground/90'
      }`}
    >
      <Icon
        name={CATEGORY_ICONS[cat] ?? 'sliders'}
        className={`w-4 h-4 shrink-0 ${isOpen ? 'text-accent' : 'text-muted'}`}
      />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate">{CATEGORY_LABELS[cat] ?? cat}</div>
        <div className="text-xs text-muted">
          {catItems.length === 1
            ? t('server.settingCount_one', { count: catItems.length })
            : t('server.settingCount_other', { count: catItems.length })}
          {overrideCount > 0 && (
            <span className="ml-1 text-warning">
              {t('server.overriddenCount', { count: overrideCount })}
            </span>
          )}
        </div>
      </div>
      <Icon
        name={isOpen ? 'chevron-up' : 'chevron-down'}
        className={`w-4 h-4 shrink-0 ${isOpen ? 'text-accent' : 'text-muted/50'}`}
      />
    </button>
  )
}
