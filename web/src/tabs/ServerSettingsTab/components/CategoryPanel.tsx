import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import type { ServerSetting } from '../../../api/client'
import { Panel, SectionLabel, Icon } from '../../../dune-ui'
import { SettingRow } from './SettingRow'
import { CATEGORY_LABELS } from '../constants'
import type { CategoryPanelProps } from './interfaces'

export const CategoryPanel: React.FC<CategoryPanelProps> = ({
  cat, catItems, searching, pending, onChange, onDelete, onToggle, isAmpManaged,
}) => {
  const { t } = useTranslation()
  const pendingKey = (item: ServerSetting) => `${item.section}|${item.key}`

  return (
    <Panel>
      <div className="flex items-center justify-between mb-2">
        <SectionLabel>{CATEGORY_LABELS[cat] ?? cat}</SectionLabel>
        {!searching && (
          <Button
            size="sm"
            variant="ghost"
            onPress={() => onToggle(cat)}
            aria-label={t('server.collapseCategory')}
          >
            <Icon name="x" className="w-3.5 h-3.5" />
          </Button>
        )}
      </div>
      <div>
        {catItems.map((item) => (
          <SettingRow
            key={`${item.section}|${item.key}`}
            item={item}
            ampManaged={isAmpManaged(item)}
            pending={pending.get(pendingKey(item))}
            onChange={(v) => onChange(item, v)}
            onDelete={() => onDelete(item)}
          />
        ))}
      </div>
    </Panel>
  )
}
