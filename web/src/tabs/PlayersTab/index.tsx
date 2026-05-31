import { useState, useEffect, useMemo, useCallback } from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { Icon, SideNav } from '../../dune-ui'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { StatusDot } from './components/StatusDot'
import { InventoryView } from './views/InventoryView'
import { VehiclesView } from './views/VehiclesView'
import { GiveItemsView } from './views/GiveItemsView'
import { ActionsView } from './views/ActionsView'

type DetailTab = 'overview' | 'inventory' | 'vehicles' | 'give' | 'actions'

const POLL_MS = 30_000

export default function PlayersTab({ isActive = false }: { isActive?: boolean }) {
  const { t } = useTranslation()

  const DETAIL_TABS: { key: DetailTab, label: string }[] = [
    { key: 'overview', label: t('players.tabs.overview') },
    { key: 'inventory', label: t('players.tabs.inventory') },
    { key: 'vehicles', label: t('players.tabs.vehicles') },
    { key: 'give', label: t('players.tabs.give') },
    { key: 'actions', label: t('players.tabs.actions') },
  ]

  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Player | null>(null)
  const [activeTab, setActiveTab] = useState<DetailTab>('overview')

  const loadPlayers = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.list())
      .then((list) => {
        setPlayers(list)
        setSelected((prev) => prev ?? list[0] ?? null)
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadPlayers()
  }, [loadPlayers])

  const { countdown, refresh } = useAutoRefresh(loadPlayers, POLL_MS, isActive)

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return q
      ? players.filter((p) =>
          p.name.toLowerCase().includes(q)
          || p.class.toLowerCase().includes(q)
          || p.map.toLowerCase().includes(q),
        )
      : players
  }, [players, search])

  const navItems = useMemo(() => filtered.map((p) => ({
    key: String(p.id),
    label: (
      <span className="flex items-center gap-2 min-w-0">
        <StatusDot status={p.online_status} />
        <span className="truncate">{p.name}</span>
      </span>
    ),
    sublabel: `${p.class} · ${p.map}`,
  })), [filtered])

  return (
    <div className="flex h-full min-h-0 gap-3">
      <SideNav
        items={navItems}
        active={selected ? String(selected.id) : null}
        onSelect={(id) => {
          const p = players.find((x) => String(x.id) === id)
          if (p) setSelected(p)
        }}
        title={`${t('players.title')} (${players.length})`}
        titleAction={(
          <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading}>
            {loading
              ? <Spinner size="sm" color="current" />
              : (
                  <>
                    {isActive && (
                      <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                        {countdown}
                        s
                      </span>
                    )}
                    <Icon name="refresh-cw" />
                  </>
                )}
          </Button>
        )}
        width="w-80"
      >
        <SearchField
          aria-label={t('players.searchLabel')}
          className="w-full"
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder={t('players.searchPlaceholder')} />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
      </SideNav>

      {/* Right detail panel */}
      <div className="flex-1 min-w-0 flex flex-col min-h-0">
        {selected
          ? (
              <>
                {/* Fixed header: name + status + tab nav */}
                <div className="shrink-0 flex items-center gap-2 pr-3 py-2">
                  <span className="font-semibold text-accent">{selected.name}</span>
                  <StatusDot status={selected.online_status} />
                  <span className="text-muted text-xs">{selected.online_status}</span>
                  <div className="ml-auto flex gap-1">
                    {DETAIL_TABS.map((tab) => (
                      <Button
                        key={tab.key}
                        size="sm"
                        variant={activeTab === tab.key ? 'secondary' : 'ghost'}
                        onPress={() => setActiveTab(tab.key)}
                      >
                        {tab.label}
                      </Button>
                    ))}
                  </div>
                </div>

                {/* Tab content — each tab owns its own scroll/height context */}
                <div className="flex-1 min-h-0 overflow-hidden">
                  {activeTab === 'overview' && (
                    <div className="h-full overflow-y-auto pr-3">
                      <PlayerDetailPanel player={selected} />
                    </div>
                  )}
                  {activeTab === 'inventory' && (
                    <div className="h-full flex flex-col pr-3">
                      <InventoryView player={selected} />
                    </div>
                  )}
                  {activeTab === 'vehicles' && (
                    <div className="h-full flex flex-col pr-3">
                      <VehiclesView player={selected} />
                    </div>
                  )}
                  {activeTab === 'give' && (
                    <div className="h-full flex flex-col pt-4 pb-4">
                      <GiveItemsView player={selected} />
                    </div>
                  )}
                  {activeTab === 'actions' && (
                    <div className="h-full flex flex-col pr-3">
                      <ActionsView player={selected} />
                    </div>
                  )}
                </div>
              </>
            )
          : (
              <div className="flex flex-1 items-center justify-center">
                <p className="text-muted text-sm">{t('players.selectPlayer')}</p>
              </div>
            )}
      </div>
    </div>
  )
}
