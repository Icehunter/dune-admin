import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Modal, SearchField, Separator, Switch, TextField, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid } from '@heroui-pro/react'
import { api } from '../../../api/client'
import type { BattlepassSignal, GivePack } from '../../../api/client'
import { ActionBar, FieldInput, FieldSelect, Icon, NumberInput, SectionLabel } from '../../../dune-ui'
import { ManagePacksModal } from '../../PlayersTab/modals/ManagePacksModal'
import { CategorizedPackPicker } from '../../../components/CategorizedPackPicker'
import type { KeyedRewardItem } from '../../EventsTab/interfaces'
import { FormSection } from './FormSection'
import type { TierEditorModalProps } from './interfaces'

const SIGNAL_OPTIONS: BattlepassSignal[] = ['level', 'journey_node', 'player_tag']
const CATEGORY_OPTIONS = ['level', 'story', 'side_quest', 'faction', 'exploration', 'achievement']

/** Edit or create a battlepass tier. In edit mode tier_key is read-only (claims
 *  are keyed by it — editing would orphan them). In create mode all fields are
 *  editable, including tier_key. */
export const TierEditorModal: React.FC<TierEditorModalProps> = ({ isOpen, onClose, tier, onSaved }) => {
  const { t } = useTranslation()
  const isCreate = tier === null

  // Core fields
  const [tierKey, setTierKey] = React.useState('')
  const [category, setCategory] = React.useState('level')
  const [label, setLabel] = React.useState('')
  const [intel, setIntel] = React.useState(0)
  const [enabled, setEnabled] = React.useState(true)
  const [signal, setSignal] = React.useState<BattlepassSignal>('level')
  const [signalKey, setSignalKey] = React.useState('')
  const [threshold, setThreshold] = React.useState(1)

  // Reward item state
  const [rewardItems, setRewardItems] = React.useState<KeyedRewardItem[]>([])
  const [rewardItemKeys, setRewardItemKeys] = React.useState<Selection>(new Set())
  const [saving, setSaving] = React.useState(false)

  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])
  const [packs, setPacks] = React.useState<GivePack[]>([])
  const [managePacksOpen, setManagePacksOpen] = React.useState(false)
  const [templateQuery, setTemplateQuery] = React.useState('')
  const [selectedTemplate, setSelectedTemplate] = React.useState('')
  const [itemQty, setItemQty] = React.useState(1)
  const [itemQuality, setItemQuality] = React.useState(0)
  const keyCounter = React.useRef(0)
  const nextKey = () => `k${++keyCounter.current}`

  React.useEffect(() => {
    if (!isOpen) return
    Promise.resolve().then(() => {
      if (tier) {
        // Edit mode — populate from existing tier
        setTierKey(tier.tier_key)
        setCategory(tier.category)
        setLabel(tier.label)
        setIntel(tier.intel)
        setEnabled(tier.enabled)
        setSignal(tier.signal)
        setSignalKey(tier.signal_key)
        setThreshold(tier.threshold)
        let parsed: KeyedRewardItem[] = []
        if (tier.reward_items) {
          try {
            const raw = JSON.parse(tier.reward_items) as { template: string, qty: number, quality: number }[]
            parsed = raw.map((x) => ({ ...x, _key: nextKey() }))
          }
          catch {
            parsed = []
          }
        }
        setRewardItems(parsed)
      }
      else {
        // Create mode — reset to defaults
        setTierKey('')
        setCategory('level')
        setLabel('')
        setIntel(0)
        setEnabled(true)
        setSignal('level')
        setSignalKey('')
        setThreshold(1)
        setRewardItems([])
      }
      setTemplateQuery('')
      setSelectedTemplate('')
      setRewardItemKeys(new Set())
    })
    api.players.templates().then(setTemplates).catch(() => {})
    api.givePacks.config().then((cfg) => setPacks(cfg.packs)).catch(() => {})
  }, [isOpen, tier])

  const nameMap = new Map(templates.map((tpl) => [tpl.id, tpl.name]))

  const _tq = templateQuery.trim().toLowerCase()
  const filteredTemplates = !_tq || selectedTemplate
    ? []
    : templates
        .filter((tpl) => tpl.id.toLowerCase().includes(_tq) || tpl.name.toLowerCase().includes(_tq))
        .slice(0, 10)

  const pickTemplate = (tpl: { id: string, name: string }) => {
    setSelectedTemplate(tpl.id)
    setTemplateQuery(tpl.name ? `${tpl.name} (${tpl.id})` : tpl.id)
  }

  const addRewardItem = () => {
    if (!selectedTemplate) return
    setRewardItems((prev) => [
      ...prev,
      { template: selectedTemplate, qty: itemQty, quality: itemQuality, _key: nextKey() },
    ])
    setTemplateQuery('')
    setSelectedTemplate('')
    setItemQty(1)
    setItemQuality(0)
  }

  const updateRewardItem = (key: string, field: 'qty' | 'quality', value: number) => {
    setRewardItems((prev) => prev.map((x) => (x._key === key ? { ...x, [field]: value } : x)))
  }

  const removeRewardItem = (key: string) => {
    setRewardItems((prev) => prev.filter((x) => x._key !== key))
  }

  const rewardSelectionCount = rewardItemKeys === 'all'
    ? rewardItems.length
    : (rewardItemKeys as Set<string>).size

  const handleBulkDeleteRewardItems = () => {
    if (rewardItemKeys === 'all') {
      setRewardItems([])
    }
    else {
      const keys = rewardItemKeys as Set<string>
      setRewardItems((prev) => prev.filter((it) => !keys.has(it._key)))
    }
    setRewardItemKeys(new Set())
  }

  const rewardItemsJson = rewardItems.length > 0
    ? JSON.stringify(rewardItems.map(({ template, qty, quality }) => ({ template, qty, quality })))
    : ''

  const handleSave = () => {
    setSaving(true)
    const promise = isCreate
      ? api.battlepass.createTier({
          tier_key: tierKey.trim(),
          category,
          label: label.trim(),
          intel,
          enabled,
          signal,
          signal_key: signalKey.trim(),
          threshold,
          reward_items: rewardItemsJson,
        })
      : api.battlepass.updateTier(tier!.id, {
          label: label.trim() || tier!.label,
          intel,
          enabled,
          reward_items: rewardItemsJson,
          category,
          signal,
          signal_key: signalKey.trim(),
          threshold,
        })

    promise
      .then(() => {
        toast.success(isCreate ? t('battlepass.editor.created') : t('battlepass.editor.saved'))
        onSaved()
        onClose()
      })
      .catch((e: unknown) => {
        toast.danger(t('battlepass.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setSaving(false))
  }

  const requirement = tier
    ? tier.signal === 'level'
      ? t('battlepass.requirementLevel', { level: tier.threshold })
      : tier.signal_key
    : ''

  const fieldLabelClass = 'text-xs text-muted mb-1 block'

  const rewardItemColumns: DataGridColumn<KeyedRewardItem>[] = [
    {
      id: 'template',
      isRowHeader: true,
      header: t('players.inventory.columns.template'),
      minWidth: 200,
      allowsResizing: true,
      cell: (item) => (
        <div className="leading-tight py-0.5">
          <div className="truncate text-sm">{nameMap.get(item.template) || item.template}</div>
          {nameMap.get(item.template) && (
            <div className="font-mono text-[10px] text-muted truncate">{item.template}</div>
          )}
        </div>
      ),
    },
    {
      id: 'qty',
      header: t('players.give.qty'),
      minWidth: 130,
      maxWidth: 250,
      allowsResizing: true,
      cell: (item) => (
        <NumberInput
          ariaLabel={t('players.give.qty')}
          min={1}
          value={item.qty}
          onChange={(v) => updateRewardItem(item._key, 'qty', v)}
          className="w-full"
        />
      ),
    },
    {
      id: 'quality',
      header: t('players.give.quality'),
      minWidth: 130,
      maxWidth: 250,
      allowsResizing: true,
      cell: (item) => (
        <NumberInput
          ariaLabel={t('players.give.quality')}
          min={0}
          value={item.quality}
          onChange={(v) => updateRewardItem(item._key, 'quality', v)}
          className="w-full"
        />
      ),
    },
    {
      id: 'actions',
      header: '',
      width: 52,
      cell: (item) => (
        <Button
          size="sm"
          variant="danger-soft"
          isIconOnly
          onPress={() => removeRewardItem(item._key)}
          aria-label={t('common.remove')}
        >
          <Icon name="trash" />
        </Button>
      ),
    },
  ]

  return (
    <React.Fragment>
      {/* Hidden (not stacked) while Manage Packs is open — stacked sibling
          modals fight over the React Aria overlay and the top one goes inert.
          Component stays mounted, so unsaved edits survive the swap. */}
      <Modal.Backdrop
        variant="blur"
        className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent"
        isOpen={isOpen && !managePacksOpen}
        onOpenChange={(open) => { if (!open) onClose() }}
      >
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="p-10 dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header className="flex items-center gap-4 shrink-0">
              <Modal.Heading className="text-accent">
                {isCreate ? t('battlepass.editor.createTitle') : t('battlepass.editor.title')}
              </Modal.Heading>
              {!isCreate && (
                <span className="text-xs text-muted font-mono">
                  {tier!.tier_key}
                  {' · '}
                  {requirement}
                </span>
              )}
            </Modal.Header>

            <Modal.Body className="flex flex-col h-[80vh] min-h-0 gap-4">
              <FormSection className="shrink-0">
                <SectionLabel>{t('battlepass.editor.tierSection')}</SectionLabel>

                {/* tier_key — editable only in create mode */}
                {isCreate && (
                  <div>
                    <span className={fieldLabelClass}>{t('battlepass.editor.tierKey')}</span>
                    <FieldInput
                      value={tierKey}
                      onChange={setTierKey}
                      ariaLabel={t('battlepass.editor.tierKey')}
                      className="font-mono"
                    />
                  </div>
                )}

                {/* category + signal row */}
                <div className="flex items-end gap-4">
                  <div className="flex-1 min-w-0">
                    <span className={fieldLabelClass}>{t('battlepass.editor.category')}</span>
                    <FieldSelect
                      value={category}
                      onChange={setCategory}
                      options={CATEGORY_OPTIONS}
                    />
                  </div>
                  <div className="flex-1 min-w-0">
                    <span className={fieldLabelClass}>{t('battlepass.editor.signal')}</span>
                    <FieldSelect
                      value={signal}
                      onChange={(v) => setSignal(v as BattlepassSignal)}
                      options={SIGNAL_OPTIONS}
                    />
                  </div>
                  {signal === 'level'
                    ? (
                        <NumberInput
                          prefix={t('battlepass.editor.threshold')}
                          value={threshold}
                          onChange={setThreshold}
                          min={1}
                          ariaLabel={t('battlepass.editor.threshold')}
                          className="w-48 shrink-0"
                        />
                      )
                    : (
                        <div className="flex-1 min-w-0">
                          <span className={fieldLabelClass}>{t('battlepass.editor.signalKey')}</span>
                          <FieldInput
                            value={signalKey}
                            onChange={setSignalKey}
                            ariaLabel={t('battlepass.editor.signalKey')}
                            className="font-mono"
                          />
                        </div>
                      )}
                </div>

                {/* label + intel + enabled row */}
                <div className="flex items-end gap-4">
                  <div className="flex-1 min-w-0">
                    <span className={fieldLabelClass}>{t('battlepass.editor.label')}</span>
                    <FieldInput value={label} onChange={setLabel} ariaLabel={t('battlepass.editor.label')} />
                  </div>
                  <NumberInput
                    prefix={t('battlepass.columns.intel')}
                    value={intel}
                    onChange={setIntel}
                    min={0}
                    ariaLabel={t('battlepass.columns.intel')}
                    className="w-56 shrink-0"
                  />
                  <Switch size="sm" isSelected={enabled} onChange={() => setEnabled((v) => !v)} className="shrink-0 pb-2">
                    <Switch.Control><Switch.Thumb /></Switch.Control>
                    <Switch.Content className="text-xs">{t('battlepass.columns.enabled')}</Switch.Content>
                  </Switch>
                </div>
              </FormSection>

              <FormSection className="flex-1 min-h-0 flex flex-col">
                <SectionLabel>{t('battlepass.editor.itemRewards')}</SectionLabel>
                <div className="flex items-center gap-2 shrink-0">
                  <CategorizedPackPicker
                    packs={packs}
                    onSelectPack={(id) => {
                      const pack = packs.find((p) => p.id === id)
                      if (pack) {
                        setRewardItems((prev) => [
                          ...prev,
                          ...pack.items.map((item) => ({ ...item, _key: nextKey() })),
                        ])
                      }
                    }}
                    className="flex-1"
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => setManagePacksOpen(true)}
                    aria-label={t('players.give.managePacks')}
                  >
                    <Icon name="settings-2" />
                    {' '}
                    {t('players.give.managePacks')}
                  </Button>
                </div>
                <div className="flex items-end gap-2 shrink-0">
                  <TextField className="flex-1 min-w-0" aria-label={t('players.inventory.columns.template')}>
                    <div className="relative w-full">
                      <SearchField
                        className="w-full"
                        value={templateQuery}
                        onChange={(v) => {
                          setTemplateQuery(v)
                          setSelectedTemplate('')
                        }}
                      >
                        <SearchField.Group>
                          <SearchField.SearchIcon />
                          <SearchField.Input placeholder={t('players.give.searchTemplates')} />
                          <SearchField.ClearButton />
                        </SearchField.Group>
                      </SearchField>
                      {filteredTemplates.length > 0 && (
                        <div className="absolute z-[200] w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-48">
                          {filteredTemplates.map((tpl) => (
                            <div
                              key={tpl.id}
                              className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                              onClick={() => pickTemplate(tpl)}
                            >
                              <span className="font-mono">{tpl.id}</span>
                              {tpl.name && (
                                <span className="text-muted">
                                  {' — '}
                                  {tpl.name}
                                </span>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </TextField>
                  <NumberInput
                    prefix={t('players.give.qty')}
                    ariaLabel={t('players.give.qty')}
                    min={1}
                    value={itemQty}
                    onChange={setItemQty}
                    className="w-40 shrink-0"
                  />
                  <NumberInput
                    prefix={t('players.give.quality')}
                    ariaLabel={t('players.give.quality')}
                    min={0}
                    value={itemQuality}
                    onChange={setItemQuality}
                    className="w-40 shrink-0"
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={addRewardItem}
                    isDisabled={!selectedTemplate}
                    className="shrink-0"
                  >
                    <Icon name="plus" />
                    {' '}
                    {t('players.give.add')}
                  </Button>
                </div>
                {rewardItems.length > 0 && (
                  <DataGrid
                    aria-label={t('battlepass.editor.itemRewards')}
                    columns={rewardItemColumns}
                    data={rewardItems}
                    getRowId={(item) => item._key}
                    selectedKeys={rewardItemKeys}
                    selectionMode="multiple"
                    showSelectionCheckboxes
                    onSelectionChange={setRewardItemKeys}
                    className="mt-2 flex-1 min-h-0"
                    scrollContainerClassName="h-full overflow-y-auto"
                    allowsColumnResize
                  />
                )}
              </FormSection>
            </Modal.Body>

            <Modal.Footer className="flex items-center gap-2 shrink-0">
              <Button size="sm" variant="tertiary" slot="close" onPress={onClose}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" variant="secondary" onPress={handleSave} isDisabled={saving}>
                {t('common.save')}
              </Button>
            </Modal.Footer>

            {/* Inside the dialog: outside it, React Aria's modal underlay
                makes the bar inert. The dialog's filter creates a containing
                block, so position:fixed pins it to the dialog bottom. */}
            <ActionBar aria-label={t('battlepass.editor.itemRewards')} isOpen={rewardSelectionCount > 0}>
              <ActionBar.Prefix>
                <Chip size="sm" className="shrink-0 tabular-nums">{rewardSelectionCount}</Chip>
              </ActionBar.Prefix>
              <Separator />
              <ActionBar.Content>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-danger"
                  onPress={handleBulkDeleteRewardItems}
                  aria-label={t('common.deleteSelected')}
                >
                  <Icon name="trash-2" />
                  <span className="action-bar__label">{t('common.deleteSelected')}</span>
                </Button>
              </ActionBar.Content>
              <Separator />
              <ActionBar.Suffix>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  onPress={() => setRewardItemKeys(new Set())}
                  aria-label={t('common.clearSelection')}
                >
                  <Icon name="x" />
                </Button>
              </ActionBar.Suffix>
            </ActionBar>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>

      <ManagePacksModal
        isOpen={managePacksOpen}
        onClose={() => setManagePacksOpen(false)}
        onSaved={(savedPacks) => {
          setPacks(savedPacks)
          setManagePacksOpen(false)
        }}
        templates={templates}
      />
    </React.Fragment>
  )
}
