import { useTranslation } from 'react-i18next'
import { Button, Chip, CloseButton, SearchField } from '@heroui/react'
import { SectionLabel } from '../../../../../dune-ui'
import { api } from '../../../../../api/client'
import type { Player } from '../../../../../api/client'

interface ContractsSectionProps {
  player: Player
  busy: boolean
  contractCatalog: { id: string, alias: string, tag_count: number }[]
  contractCatalogLoaded: boolean
  contractCatalogError: string
  contractSearch: string
  setContractSearch: (v: string) => void
  selectedContracts: string[]
  setSelectedContracts: (updater: ((prev: string[]) => string[]) | string[]) => void
  onNodesInvalidate: () => void
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
}

export function ContractsSection({
  player,
  busy,
  contractCatalog,
  contractCatalogLoaded,
  contractCatalogError,
  contractSearch,
  setContractSearch,
  selectedContracts,
  setSelectedContracts,
  onNodesInvalidate,
  run,
}: ContractsSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="flex-1 min-h-0 overflow-hidden flex flex-col gap-3">
      <div className="flex items-center gap-2 min-h-8">
        <SectionLabel>{t('players.actions.contracts.title')}</SectionLabel>
        <div className="text-xs text-muted">
          {contractCatalogError
            ? (
                <span className="text-danger">
                  {t('players.actions.contracts.loadFailed', { error: contractCatalogError })}
                </span>
              )
            : contractCatalogLoaded
              ? t('players.actions.contracts.count', { count: contractCatalog.length })
              : t('players.actions.contracts.loadingContracts')}
        </div>
      </div>
      <div className="text-xs text-muted">{t('players.actions.contracts.desc')}</div>

      {selectedContracts.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {selectedContracts.map((id) => (
            <Chip key={id} size="sm" variant="soft">
              <span className="font-mono">{id}</span>
              <CloseButton
                aria-label={`Remove ${id}`}
                onPress={() => setSelectedContracts((prev) => prev.filter((x) => x !== id))}
                className="ml-1"
              />
            </Chip>
          ))}
          <Button
            variant="ghost"
            size="sm"
            className="text-xs text-muted px-0 h-auto min-w-0"
            onPress={() => setSelectedContracts([])}
          >
            {t('players.actions.contracts.clearAll')}
          </Button>
        </div>
      )}

      <div className="flex items-center gap-2 flex-wrap">
        <SearchField
          aria-label={t('players.actions.contracts.filterLabel')}
          className="flex-1 min-w-48"
          value={contractSearch}
          onChange={setContractSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder={t('players.actions.contracts.filterPlaceholder')} />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        <Button
          size="sm"
          variant="secondary"
          isDisabled={busy || selectedContracts.length === 0}
          onPress={() =>
            run(
              () => api.players.completeContracts(player.account_id, selectedContracts),
              `Completed ${selectedContracts.length} contract(s) for ${player.name}`,
            ).then(() => {
              setSelectedContracts([])
              onNodesInvalidate()
            })}
        >
          {t('players.actions.contracts.applyContracts', { count: selectedContracts.length })}
        </Button>
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy || selectedContracts.length === 0}
          onPress={() =>
            run(
              () => api.players.reverseContracts(player.account_id, selectedContracts),
              `Reversed ${selectedContracts.length} contract(s) for ${player.name}`,
            ).then(() => {
              setSelectedContracts([])
              onNodesInvalidate()
            })}
        >
          {t('players.actions.contracts.reverseContracts', { count: selectedContracts.length })}
        </Button>
      </div>

      {contractCatalogLoaded && !contractCatalogError && (
        <div className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-surface-alt">
          {(() => {
            const q = contractSearch.trim().toLowerCase()
            const matches = contractCatalog.filter(
              (c) => q === '' || c.id.toLowerCase().includes(q) || (c.alias && c.alias.toLowerCase().includes(q)),
            )
            if (matches.length === 0) {
              return <div className="px-2 py-3 text-xs text-center text-muted">{t('players.actions.contracts.noMatching')}</div>
            }
            return matches.map((c) => {
              const id = c.alias || c.id
              const picked = selectedContracts.includes(id)
              return (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => setSelectedContracts((prev) => picked ? prev.filter((x) => x !== id) : [...prev, id])}
                  className={
                    'flex w-full items-center justify-between px-2 py-1 text-xs font-mono hover:bg-surface '
                    + (picked ? 'bg-surface text-accent' : 'bg-transparent text-foreground')
                  }
                >
                  <span>
                    {picked ? '✓ ' : '  '}
                    {id}
                  </span>
                  <span className="text-muted">
                    {c.tag_count === 1
                      ? t('players.actions.contracts.tagCount', { count: c.tag_count })
                      : t('players.actions.contracts.tagCountPlural', { count: c.tag_count })}
                  </span>
                </button>
              )
            })
          })()}
        </div>
      )}
    </div>
  )
}
