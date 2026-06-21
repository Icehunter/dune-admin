import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import { Icon } from '../../dune-ui'
import type { HelpCardProps } from './interfaces'

export const HelpCard: React.FC<HelpCardProps> = ({ icon, title, body, cta, onAction, onDismiss }) => {
  const { t } = useTranslation()
  return (
    // Horizontal layout: an accent icon medallion on the left, content + actions
    // on the right. `dune-lift` gives the themed HUD plate with corner SVG art.
    <div className="dune-lift flex items-start gap-4 p-4">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-[var(--radius)] bg-accent/15 text-accent">
        <Icon name={icon} />
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="text-sm font-semibold text-foreground">{title}</span>
        <p className="text-xs text-muted">{body}</p>
        <div className="mt-2 flex items-center gap-2">
          <Button size="sm" variant="outline" onPress={onAction}>{cta}</Button>
          <Button size="sm" variant="ghost" onPress={onDismiss}>{t('common.dismiss', 'Dismiss')}</Button>
        </div>
      </div>
    </div>
  )
}
