import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../../api/client'
import type { ServerSetting, ServerSettingUpdate, RawSection } from '../../api/client'
import { PageHeader, Panel, SectionLabel, Icon } from '../../dune-ui'
import { SettingRow } from './components/SettingRow'
import { RawSectionPanel } from './components/RawSectionPanel'
import { CategorySection } from './components/CategorySection'
import {
  ADVANCED_CATEGORIES, COMMON_KEYS, SOURCE_FILE,
  SOURCE_PRIORITY, USER_SOURCES,
} from './constants'
import {
  groupByCategory, matchesSetting, matchesRawSection,
} from './utils'

export const ServerSettingsTab: React.FC = () => {
  const { t } = useTranslation()
  const [items, setItems] = React.useState<ServerSetting[]>([])
  const [raw, setRaw] = React.useState<RawSection[]>([])
  const [control, setControl] = React.useState('')
  const [pending, setPending] = React.useState<Map<string, string>>(new Map())
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [search, setSearch] = React.useState('')
  const [showAll, setShowAll] = React.useState(() =>
    localStorage.getItem('serverSettings.showAll') === 'true',
  )
  const [expandedCategory, setExpandedCategory] = React.useState<string | null>(() =>
    localStorage.getItem('serverSettings.expandedCategory') || null,
  )

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setError(null)
      })
      .then(() => api.serverSettings.get())
      .then((data) => {
        setItems(data.settings ?? [])
        setRaw(data.raw ?? [])
        setControl(data.control ?? '')
        setPending(new Map())
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  React.useEffect(() => {
    load()
  }, [load])

  const pendingKey = (item: ServerSetting) => `${item.section}|${item.key}`

  const handleChange = (item: ServerSetting, value: string) => {
    setPending((prev) => {
      const n = new Map(prev)
      n.set(pendingKey(item), value)
      return n
    })
  }

  const handleDelete = async (item: ServerSetting) => {
    try {
      await api.serverSettings.update([{ section: item.section, key: item.key, value: '' }])
      toast.success(t('server.removedFrom', { file: SOURCE_FILE[item.source] ?? item.source }))
      load()
    }
    catch (e: unknown) {
      toast.danger(t('server.deleteFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
  }

  const save = async () => {
    const updates: ServerSettingUpdate[] = []
    for (const [k, v] of pending) {
      const [section, key] = k.split('|')
      updates.push({ section, key, value: v })
    }
    if (updates.length === 0) return
    setSaving(true)
    try {
      const res = await api.serverSettings.update(updates)
      toast.success(res.ok)
      load()
    }
    catch (e: unknown) {
      toast.danger(t('server.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const dirtyCount = pending.size

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">{t('server.loading')}</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col h-full gap-3">
        <PageHeader title={t('server.title')} />
        <div className="rounded px-4 py-3 text-sm bg-danger/10 border border-danger/40 text-danger">
          {error.includes('server_ini_dir') || error.includes('ini dir')
            ? t('server.iniNotFound', { error })
            : error}
        </div>
      </div>
    )
  }

  const toggleShowAll = () => setShowAll((v) => {
    localStorage.setItem('serverSettings.showAll', String(!v))
    return !v
  })

  const visibleItems = showAll
    ? items
    : items.filter((item) => item.layers.some((l) => USER_SOURCES.has(l.source)))

  const q = search.trim().toLowerCase()
  const searching = q.length > 0

  const commonItems = items
    .filter((item) => COMMON_KEYS.has(`${item.section}|${item.key}`))
    .filter((item) => matchesSetting(item, q))
  const advancedItems = visibleItems
    .filter((item) => !COMMON_KEYS.has(`${item.section}|${item.key}`))
    .filter((item) => matchesSetting(item, q))
  const categories = groupByCategory(advancedItems)
  // Split into the curated gameplay set (Advanced) and the long engine/system
  // tail (Expert, hidden behind a toggle). Searching reveals everything so a
  // match in an Expert category isn't silently hidden.
  const advancedCategories = categories.filter(([cat]) => ADVANCED_CATEGORIES.has(cat))
  const expertCategories = categories.filter(([cat]) => !ADVANCED_CATEGORIES.has(cat))
  const ampManaged = (item: ServerSetting) => control === 'amp' && !!item.field_name

  const toggleCategory = (cat: string) => {
    setExpandedCategory((prev) => {
      const next = prev === cat ? null : cat
      if (next === null) localStorage.removeItem('serverSettings.expandedCategory')
      else localStorage.setItem('serverSettings.expandedCategory', next)
      return next
    })
  }

  const rawBySection = new Map<string, RawSection[]>()
  for (const src of SOURCE_PRIORITY) {
    for (const sec of raw) {
      if (sec.source !== src) continue
      const arr = rawBySection.get(sec.section) ?? []
      arr.push(sec)
      rawBySection.set(sec.section, arr)
    }
  }

  const visibleRawSections = (showAll
    ? [...rawBySection.values()]
    : [...rawBySection.values()].filter((secs) =>
        secs.some((s) => USER_SOURCES.has(s.source)),
      )
  ).filter((secs) => matchesRawSection(secs, q))

  const hasResults
    = commonItems.length > 0 || categories.length > 0 || visibleRawSections.length > 0

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('server.title')}>
        <div className="flex items-center gap-2">
          <SearchField
            aria-label={t('server.searchLabel')}
            className="w-56"
            value={search}
            onChange={setSearch}
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder={t('server.searchPlaceholder')} />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading || saving}>
            <Icon name="refresh-cw" />
          </Button>
          <Button
            size="sm"
            variant={showAll ? 'primary' : 'ghost'}
            onPress={toggleShowAll}
            aria-label={showAll ? t('server.showAllAriaLabel') : t('server.showUserAriaLabel')}
          >
            <Icon name={showAll ? 'eye' : 'eye-off'} className="w-3.5 h-3.5" />
            <span className="ml-1">{showAll ? t('server.showAll') : t('server.showUser')}</span>
          </Button>
          <Button size="sm" onPress={save} isDisabled={dirtyCount === 0 || saving}>
            {saving
              ? <Spinner size="sm" color="current" />
              : dirtyCount > 0 ? t('server.saveWithCount', { count: dirtyCount }) : t('server.save')}
          </Button>
        </div>
      </PageHeader>

      <p className="text-xs text-muted shrink-0">
        Changes are saved to the server configuration — written to
        {' '}
        <span className="font-mono">UserGame.ini</span>
        {' / '}
        <span className="font-mono">UserEngine.ini</span>
        {' '}
        directly, or via the AMP API under the AMP control plane.
        A server restart is required for them to take effect.
      </p>

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6 pr-1">

        {searching && !hasResults && (
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:magnifier" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('server.noMatchSettings', { query: search.trim() })}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}

        {commonItems.length > 0 && (
          <Panel>
            <SectionLabel>{t('server.commonSettings')}</SectionLabel>
            <div className="text-xs text-muted mb-2">
              {t('server.commonSettingsDesc')}
            </div>
            <div>
              {commonItems.map((item) => (
                <SettingRow
                  key={`common|${item.section}|${item.key}`}
                  item={item}
                  ampManaged={ampManaged(item)}
                  pending={pending.get(pendingKey(item))}
                  onChange={(v) => handleChange(item, v)}
                  onDelete={() => handleDelete(item)}
                />
              ))}
            </div>
          </Panel>
        )}

        <CategorySection
          title={t('server.advancedCategories')}
          description={t('server.advancedCategoriesDesc')}
          categories={advancedCategories}
          expandedCategory={expandedCategory}
          onToggle={toggleCategory}
          searching={searching}
          pending={pending}
          onChange={handleChange}
          onDelete={handleDelete}
          isAmpManaged={ampManaged}
        />

        <CategorySection
          title={t('server.expertCategories')}
          description={t('server.expertCategoriesDesc')}
          categories={expertCategories}
          expandedCategory={expandedCategory}
          onToggle={toggleCategory}
          searching={searching}
          pending={pending}
          onChange={handleChange}
          onDelete={handleDelete}
          isAmpManaged={ampManaged}
        />

        {visibleRawSections.length > 0 && (
          <div>
            <SectionLabel>{t('server.rawIniSections')}</SectionLabel>
            <div className="text-xs text-muted mb-2">
              {t('server.rawIniDesc')}
            </div>
            <div className="flex flex-col gap-3 mt-2">
              {visibleRawSections.map((sections) => (
                <RawSectionPanel
                  key={sections[0].section}
                  sections={sections}
                  onSaved={load}
                />
              ))}
            </div>
          </div>
        )}

      </div>
    </div>
  )
}
