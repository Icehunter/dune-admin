import * as React from 'react'
import { Button, ListBox, Select, Tooltip } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { FieldInput, NumberInput, Icon } from '../../../dune-ui'
import { SOURCE_FILE, LAYER_STYLE, USER_SOURCES } from '../constants'
import { sourceLabel, trimFloat } from '../utils'
import type { SettingRowProps } from './types'
import type { ServerSetting } from '../../../api/client'

export const SettingRow: React.FC<SettingRowProps> = ({
  item, pending, onChange, onDelete, ampManaged,
}) => {
  const { t } = useTranslation()
  const rawDisplay = pending !== undefined ? pending : item.current
  const display = item.type === 'bool'
    ? (/^(true|1|yes)$/i.test(rawDisplay) ? 'True' : /^(false|0|no)$/i.test(rawDisplay) ? 'False' : rawDisplay)
    : rawDisplay
  const dirty = pending !== undefined && rawDisplay !== item.current
  const src = sourceLabel(item.source)

  return (
    <div className="flex items-start gap-3 py-2.5 border-b border-border/40 last:border-0">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium text-foreground">{item.label}</span>
          {ampManaged && (
            <span
              title={t('server.ampManagedTooltip')}
              className="text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-accent/15 text-accent border border-accent/30"
            >
              AMP
            </span>
          )}
          {src && <span className={`text-xs ${src.cls}`}>{src.text}</span>}
          {dirty && <span className="text-xs text-warning">{t('server.unsaved')}</span>}
        </div>
        <p className="text-xs text-muted mt-0.5 leading-relaxed">{item.description}</p>
        {item.layers.length > 1 && (
          <div className="flex items-center gap-1 mt-1.5 flex-wrap">
            {item.layers.map((layer: ServerSetting['layers'][number], i: number) => {
              const style = LAYER_STYLE[layer.source] ?? { cls: 'text-muted' }
              const isActive = i === item.layers.length - 1
              return (
                <span key={layer.source} className="flex items-center gap-1">
                  <span className={`text-xs font-mono px-1.5 py-0.5 rounded border border-border/30 bg-surface/60 ${style.cls} ${isActive ? 'font-semibold' : 'opacity-50'}`}>
                    {SOURCE_FILE[layer.source] ?? layer.source}
                    :
                    {trimFloat(layer.value)}
                    {isActive ? ' ✓' : ''}
                  </span>
                  {i < item.layers.length - 1 && (
                    <span className="text-muted/30 text-xs select-none">→</span>
                  )}
                </span>
              )
            })}
          </div>
        )}
      </div>

      <div className="flex items-center gap-1.5 shrink-0">
        {item.type === 'bool'
          ? (
              <Select selectedKey={display} onSelectionChange={(k) => onChange(String(k))} className="w-32" aria-label={item.label}>
                <Select.Trigger className="h-7 text-xs">
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    <ListBox.Item id="True" textValue="True">
                      True
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="False" textValue="False">
                      False
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  </ListBox>
                </Select.Popover>
              </Select>
            )
          : item.type === 'string'
            ? (
                <FieldInput
                  ariaLabel={item.label}
                  value={display}
                  onChange={onChange}
                  className="w-40 font-mono"
                />
              )
            : (
                <NumberInput
                  ariaLabel={item.key}
                  step={item.type === 'float' ? 0.01 : 1}
                  value={Number(display) || 0}
                  onChange={(v: number) => onChange(String(v))}
                  showButtons={false}
                  className="w-32"
                  formatOptions={item.type === 'float' ? { minimumFractionDigits: 1 } : undefined}
                />
              )}
        {USER_SOURCES.has(item.source) && (
          <Tooltip>
            <Tooltip.Trigger>
              <Button
                isIconOnly
                variant="ghost"
                size="sm"
                className="text-muted/50 hover:text-danger"
                onPress={onDelete}
                aria-label={t('server.removeFrom', { file: SOURCE_FILE[item.source] })}
              >
                <Icon name="trash-2" className="w-3.5 h-3.5" />
              </Button>
            </Tooltip.Trigger>
            <Tooltip.Content>{t('server.removeFrom', { file: SOURCE_FILE[item.source] })}</Tooltip.Content>
          </Tooltip>
        )}
      </div>
    </div>
  )
}
