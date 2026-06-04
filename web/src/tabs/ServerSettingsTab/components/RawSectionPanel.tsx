import { useState, useRef } from 'react'
import { Button, Spinner, Tooltip, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { RawSection } from '../../../api/client'
import { api } from '../../../api/client'
import { Panel, SectionLabel, Icon } from '../../../dune-ui'
import { SOURCE_FILE, LAYER_STYLE, SOURCE_PRIORITY } from '../constants'
import { linesToText, groupLinesByKey, shortSection } from '../utils'

interface RawSectionPanelProps {
  sections: RawSection[]
  onSaved: () => void
}

export const RawSectionPanel: React.FC<RawSectionPanelProps> = ({ sections, onSaved }) => {
  const { t } = useTranslation()
  const sectionName = sections[0].section
  const userSec = sections.find((s) => s.source === 'userGameOverrides')
    ?? sections.find((s) => s.source === 'userGame')
    ?? sections.find((s) => s.source === 'userEngine')

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [collapsed, setCollapsed] = useState(true)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const toggle = () => {
    if (editing) return
    setCollapsed((v) => !v)
  }

  const startEdit = () => {
    setDraft(userSec ? linesToText(userSec.lines) : '')
    setEditing(true)
    setTimeout(() => textareaRef.current?.focus(), 0)
  }

  const cancel = () => setEditing(false)

  const save = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, draft)
      toast.success(t('server.savedTo', { file: userSec ? SOURCE_FILE[userSec.source] : 'UserGame.ini' }))
      setEditing(false)
      onSaved()
    }
    catch (e: unknown) {
      toast.danger(t('server.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const deleteUserEntry = async () => {
    setSaving(true)
    try {
      await api.serverSettings.updateRaw(sectionName, '')
      toast.success(t('server.removedFrom', { file: userSec ? SOURCE_FILE[userSec.source] : 'UserGame.ini' }))
      onSaved()
    }
    catch (e: unknown) {
      toast.danger(t('server.deleteFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const sorted = [...sections].sort((a, b) => {
    const ai = SOURCE_PRIORITY.indexOf(a.source as typeof SOURCE_PRIORITY[number])
    const bi = SOURCE_PRIORITY.indexOf(b.source as typeof SOURCE_PRIORITY[number])
    return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi)
  })
  const multiSource = sorted.length > 1

  return (
    <Panel>
      <div
        className={`flex items-center gap-2 flex-wrap ${collapsed && !editing ? 'cursor-pointer select-none' : 'mb-2'}`}
        onClick={collapsed && !editing ? toggle : undefined}
      >
        <Icon
          name={collapsed && !editing ? 'chevron-right' : 'chevron-down'}
          className="w-4 h-4 shrink-0 text-muted/70"
        />
        <SectionLabel>{shortSection(sectionName)}</SectionLabel>
        {sorted.map((s) => (
          <span key={s.source} className={`text-xs ${LAYER_STYLE[s.source]?.cls ?? 'text-muted'}`}>
            {SOURCE_FILE[s.source] ?? s.source}
          </span>
        ))}
        {userSec && collapsed && !editing && (
          <span className="text-xs text-warning">{t('server.userOverride')}</span>
        )}
        <div
          className="ml-auto flex items-center gap-1 min-w-[2rem]"
          onClick={(e) => e.stopPropagation()}
        >
          {editing
            ? (
                <>
                  <Button size="sm" variant="ghost" onPress={cancel} isDisabled={saving}>{t('server.collapse')}</Button>
                  <Button size="sm" onPress={save} isDisabled={saving}>
                    {saving ? <Spinner size="sm" color="current" /> : t('common.save')}
                  </Button>
                </>
              )
            : !collapsed && (
                <>
                  {userSec && (
                    <Tooltip>
                      <Tooltip.Trigger>
                        <Button
                          isIconOnly
                          variant="ghost"
                          size="sm"
                          className="text-muted/50 hover:text-danger"
                          onPress={deleteUserEntry}
                          isDisabled={saving}
                          aria-label={`Remove from ${SOURCE_FILE[userSec.source]}`}
                        >
                          <Icon name="trash-2" className="w-3.5 h-3.5" />
                        </Button>
                      </Tooltip.Trigger>
                      <Tooltip.Content>{`Remove from ${SOURCE_FILE[userSec.source]}`}</Tooltip.Content>
                    </Tooltip>
                  )}
                  <Button size="sm" variant="ghost" onPress={startEdit} isDisabled={saving}>
                    <Icon name="pencil" className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => setCollapsed(true)}
                    aria-label={t('server.collapseSection')}
                  >
                    <Icon name="x" className="w-3.5 h-3.5" />
                  </Button>
                </>
              )}
        </div>
      </div>

      {!collapsed && (editing
        ? (
            <textarea
              ref={textareaRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={Math.max(4, draft.split('\n').length + 1)}
              className="w-full bg-surface border border-border rounded px-3 py-2 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 resize-y"
              spellCheck={false}
              placeholder={t('server.rawPlaceholder')}
            />
          )
        : (
            <div className="flex flex-col gap-2">
              {sorted.map((sec) => {
                const style = LAYER_STYLE[sec.source] ?? { cls: 'text-muted' }
                const isActive = sec.source === sorted[sorted.length - 1].source
                return (
                  <div
                    key={sec.source}
                    className={multiSource ? `pl-2 border-l-2 ${isActive ? 'border-accent/40' : 'border-border/30'}` : ''}
                  >
                    {multiSource && (
                      <span className={`text-xs ${style.cls} block mb-1`}>
                        {SOURCE_FILE[sec.source] ?? sec.source}
                        {isActive ? ' ✓' : ''}
                      </span>
                    )}
                    <div className="flex flex-col gap-0.5">
                      {groupLinesByKey(sec.lines).map(({ key, lines }) => (
                        <div key={key} className="py-1 border-b border-border/30 last:border-0">
                          <span className="text-xs font-mono text-muted">{key}</span>
                          {lines.map((l, i) => (
                            <div key={i} className="flex items-baseline gap-1.5 mt-0.5 ml-3">
                              {l.prefix && (
                                <span className={`text-xs font-mono w-3 shrink-0 ${l.prefix === '+' ? 'text-success' : 'text-danger'}`}>
                                  {l.prefix}
                                </span>
                              )}
                              <span className={`text-xs font-mono break-all ${isActive ? 'text-foreground/80' : 'text-muted/50'}`}>{l.value}</span>
                            </div>
                          ))}
                        </div>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
    </Panel>
  )
}
