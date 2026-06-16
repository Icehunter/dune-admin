import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { CloseButton, Select, ListBox } from '@heroui/react'
import { FieldRow } from './FieldRow'
import type { RolePickerProps } from '../../types'

export const RolePicker: React.FC<RolePickerProps> = ({ value, onChange, roles, label, hint }) => {
  const { t } = useTranslation()
  const [pickKey, setPickKey] = React.useState(0)

  const selectedIds = value ? value.split(',').map((s) => s.trim()).filter(Boolean) : []
  const nameOf = (id: string) => roles.find((r) => r.id === id)?.name ?? id
  const available = roles.filter((r) => !selectedIds.includes(r.id))

  const addRole = (id: string) => {
    if (id && !selectedIds.includes(id)) {
      onChange([...selectedIds, id].join(','))
    }
    setPickKey((k) => k + 1)
  }

  const removeRole = (id: string) => onChange(selectedIds.filter((s) => s !== id).join(','))

  return (
    <FieldRow label={label} hint={hint}>
      <div className="flex flex-col gap-1.5">
        {available.length > 0
          ? (
              <Select
                key={pickKey}
                selectedKey=""
                aria-label={t('settings.discord.addRole')}
                onSelectionChange={(k) => addRole(String(k))}
              >
                <Select.Trigger>
                  <span className="text-sm text-muted flex-1">{t('settings.discord.addRole')}</span>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {available.map((r) => (
                      <ListBox.Item key={r.id} id={r.id} textValue={r.name}>
                        {r.name}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            )
          : (
              roles.length === 0 && selectedIds.length === 0 && (
                <p className="text-xs text-muted">{t('settings.discord.rolesNotLoaded')}</p>
              )
            )}
        {selectedIds.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {selectedIds.map((id) => (
              <span key={id} className="inline-flex items-center gap-1 rounded-full bg-accent/15 text-accent px-2 py-0.5 text-xs font-medium">
                {nameOf(id)}
                <CloseButton aria-label={`Remove ${nameOf(id)}`} className="size-4 opacity-60 hover:opacity-100" onPress={() => removeRole(id)} />
              </span>
            ))}
          </div>
        )}
      </div>
    </FieldRow>
  )
}
