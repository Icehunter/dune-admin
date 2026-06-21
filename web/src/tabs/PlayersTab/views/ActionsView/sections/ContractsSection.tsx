import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom, useSetAtom } from 'jotai'
import { Button, Chip, CloseButton, SearchField } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { SectionLabel } from '../../../../../dune-ui'
import { api } from '../../../../../api/client'
import { busyAtom, contractCatalogAtom, contractCatalogLoadedAtom, contractCatalogErrorAtom, nodesLoadedAtom } from '../store'
import { useRun } from '../hooks/useActions'
import type { ContractsSectionProps } from './interfaces'

export const ContractsSection: React.FC<ContractsSectionProps> = ({ player }) => {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const [contractCatalog, setContractCatalog] = useAtom(contractCatalogAtom(player.id))
  const [contractCatalogLoaded, setContractCatalogLoaded] = useAtom(contractCatalogLoadedAtom(player.id))
  const [contractCatalogError, setContractCatalogError] = useAtom(contractCatalogErrorAtom(player.id))
  const setNodesLoaded = useSetAtom(nodesLoadedAtom(player.id))
  const run = useRun(player.id)

  const [contractSearch, setContractSearch] = React.useState('')
  const [selectedContracts, setSelectedContracts] = React.useState<string[]>([])

  React.useEffect(() => {
    if (contractCatalogLoaded) return
    api.contracts.list()
      .then((c) => {
        setContractCatalog(c)
        setContractCatalogLoaded(true)
        setContractCatalogError('')
      })
      .catch((e: unknown) => {
        setContractCatalogError(e instanceof Error ? e.message : String(e))
        setContractCatalogLoaded(true)
      })
  }, [contractCatalogLoaded, setContractCatalog, setContractCatalogLoaded, setContractCatalogError])

  const handleCompleteContracts = () => {
    run(() => api.players.completeContracts(player.account_id, selectedContracts),
      `Completed ${selectedContracts.length} contract(s) for ${player.name}`)
      .then(() => {
        setSelectedContracts([])
        setNodesLoaded(false)
      })
  }

  const handleReverseContracts = () => {
    run(() => api.players.reverseContracts(player.account_id, selectedContracts),
      `Reversed ${selectedContracts.length} contract(s) for ${player.name}`)
      .then(() => {
        setSelectedContracts([])
        setNodesLoaded(false)
      })
  }

  const handleContractToggle = (id: string, picked: boolean) => {
    setSelectedContracts((prev) =>
      picked ? prev.filter((x) => x !== id) : [...prev, id])
  }

  const renderContractList = (): React.ReactNode => {
    const q = contractSearch.trim().toLowerCase()
    const matches = contractCatalog.filter((c) =>
      q === '' || c.id.toLowerCase().includes(q)
      || (c.alias && c.alias.toLowerCase().includes(q)))
    if (matches.length === 0) {
      return (
        <EmptyState size="sm">
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <IconifyIcon icon="gravity-ui:magnifier" className="size-5" />
            </EmptyState.Media>
            <EmptyState.Title>{t('players.actions.contracts.noMatching')}</EmptyState.Title>
          </EmptyState.Header>
        </EmptyState>
      )
    }
    return matches.map((c) => {
      const id = c.alias || c.id
      const picked = selectedContracts.includes(id)
      return (
        <button
          key={c.id}
          type="button"
          onClick={() => handleContractToggle(id, picked)}
          className={'flex w-full items-center justify-between px-2 py-1 '
            + 'text-xs font-mono hover:bg-surface '
            + (picked ? 'bg-surface text-accent' : 'bg-transparent text-foreground')}
        >
          <span>
            {picked ? '✓ ' : '  '}
            {id}
          </span>
          <span className="text-muted">
            {c.tag_count === 1
              ? t('players.actions.contracts.tagCount', { count: c.tag_count })
              : t('players.actions.contracts.tagCountPlural',
                  { count: c.tag_count })}
          </span>
        </button>
      )
    })
  }

  return (
    <div className="flex-1 min-h-0 overflow-hidden flex flex-col gap-3">
      <div className="flex items-center gap-2 min-h-8">
        <SectionLabel>{t('players.actions.contracts.title')}</SectionLabel>
        <div className="text-xs text-muted">
          {contractCatalogError
            ? (
                <span className="text-danger">
                  {t('players.actions.contracts.loadFailed',
                    { error: contractCatalogError })}
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
                onPress={() => setSelectedContracts((prev) =>
                  prev.filter((x) => x !== id))}
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
          onPress={handleCompleteContracts}
        >
          {t('players.actions.contracts.applyContracts', { count: selectedContracts.length })}
        </Button>
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy || selectedContracts.length === 0}
          onPress={handleReverseContracts}
        >
          {t('players.actions.contracts.reverseContracts', { count: selectedContracts.length })}
        </Button>
      </div>

      {contractCatalogLoaded && !contractCatalogError && (
        <div className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-surface-alt">
          {renderContractList()}
        </div>
      )}
    </div>
  )
}
