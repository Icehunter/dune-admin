import { useState, useEffect, useMemo, type ReactNode } from 'react'
import { Button, Modal, Spinner, toast, Select, ListBox } from '@heroui/react'
import { api } from '../api/client'
import type {
  Player, InventoryItem, JourneyNode,
  CurrencyRow, FactionRep, SpecTrack, KeystoneRow, OnlineRow,
  VehicleRow, TeleportLocation, GameEvent, DungeonRecord
} from '../api/client'

type Sidebar = 'players' | 'currency' | 'factions' | 'specs' | 'online'
type ActionSection = 'resources' | 'specs' | 'journey' | 'admin' | 'history'

const ACTION_SECTIONS: { key: ActionSection; label: string }[] = [
  { key: 'resources', label: 'Stats' },
  { key: 'specs', label: 'Specs' },
  { key: 'journey', label: 'Journey' },
  { key: 'admin', label: 'Admin' },
  { key: 'history', label: 'History' },
]

const XP_TRACKS = ['Combat', 'Crafting', 'Gathering', 'Exploration', 'Sabotage']
const FACTIONS = [{ id: 1, name: 'Atreides' }, { id: 2, name: 'Harkonnen' }, { id: 3, name: 'None' }, { id: 4, name: 'Smuggler' }]

function StatusDot({ status }: { status: string }) {
  const color = status === 'Online' ? '#27ae60' : status === 'LoggingOut' ? '#f0a830' : '#555'
  return (
    <span
      style={{
        display: 'inline-block',
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: color,
        marginRight: 6,
        flexShrink: 0,
      }}
    />
  )
}

function OnlineBadge({ status }: { status: string }) {
  const color = status === 'Online' ? '#27ae60' : status === 'LoggingOut' ? '#f0a830' : '#555'
  const label = status === 'Online' ? 'Online' : status === 'LoggingOut' ? 'LoggingOut' : status || 'Offline'
  return (
    <span className="text-xs px-1.5 py-0.5 rounded font-semibold" style={{ background: color + '22', color, border: `1px solid ${color}44` }}>
      {label}
    </span>
  )
}

export default function PlayersTab() {
  const [active, setActive] = useState<Sidebar>('players')
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [selectedPlayer, setSelectedPlayer] = useState<Player | null>(null)
  const [showInventory, setShowInventory] = useState(false)
  const [showGiveItem, setShowGiveItem] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const [sideLoading, setSideLoading] = useState(false)

  // Typed sidebar state
  const [currencyData, setCurrencyData] = useState<CurrencyRow[]>([])
  const [factionData, setFactionData] = useState<FactionRep[]>([])
  const [specData, setSpecData] = useState<SpecTrack[]>([])
  const [onlineData, setOnlineData] = useState<OnlineRow[]>([])
  const [sideSearch, setSideSearch] = useState('')

  useEffect(() => { loadPlayers() }, [])

  const loadPlayers = async () => {
    setLoading(true)
    try {
      setPlayers(await api.players.list())
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const loadSideData = async (section: Sidebar) => {
    setActive(section)
    setSideSearch('')
    if (section === 'players') return
    setSideLoading(true)
    try {
      if (section === 'online') {
        setOnlineData(await api.players.online())
      } else if (section === 'currency') {
        setCurrencyData(await api.players.currency())
      } else if (section === 'factions') {
        setFactionData(await api.players.factions())
      } else if (section === 'specs') {
        setSpecData(await api.players.specs())
      }
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setSideLoading(false)
    }
  }

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return players.filter(p =>
      p.name.toLowerCase().includes(q) || p.class.toLowerCase().includes(q) ||
      p.map.toLowerCase().includes(q) || String(p.id).includes(q)
    )
  }, [players, search])

  const controllerToName = useMemo(() => {
    const m = new Map<number, string>()
    for (const p of players) m.set(p.controller_id, p.name)
    return m
  }, [players])

  const filteredCurrency = useMemo(() => {
    if (!sideSearch) return currencyData
    const q = sideSearch.toLowerCase()
    return currencyData.filter(r => {
      const name = controllerToName.get(r.player_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.player_id).includes(q)
    })
  }, [currencyData, sideSearch, controllerToName])

  const filteredFactions = useMemo(() => {
    if (!sideSearch) return factionData
    const q = sideSearch.toLowerCase()
    return factionData.filter(r => {
      const name = controllerToName.get(r.actor_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.actor_id).includes(q)
    })
  }, [factionData, sideSearch, controllerToName])

  const filteredSpecs = useMemo(() => {
    if (!sideSearch) return specData
    const q = sideSearch.toLowerCase()
    return specData.filter(r => {
      const name = controllerToName.get(r.player_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.player_id).includes(q)
    })
  }, [specData, sideSearch, controllerToName])

  const filteredOnline = useMemo(() => {
    if (!sideSearch) return onlineData
    const q = sideSearch.toLowerCase()
    return onlineData.filter(r =>
      r.name.toLowerCase().includes(q) || String(r.player_id).includes(q)
    )
  }, [onlineData, sideSearch])

  const sidebarItems: { key: Sidebar; label: string }[] = [
    { key: 'players', label: 'Players' },
    { key: 'online', label: 'Online State' },
    { key: 'currency', label: 'Currency' },
    { key: 'factions', label: 'Factions' },
    { key: 'specs', label: 'Specs / XP' },
  ]

  const tableHeader = (cols: string[]) => (
    <thead>
      <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
        {cols.map(h => (
          <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
        ))}
      </tr>
    </thead>
  )

  return (
    <div className="flex gap-4 h-full">
      {/* Sidebar */}
      <div className="w-40 shrink-0 flex flex-col gap-1 rounded-lg p-2" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
        {sidebarItems.map(item => (
          <button
            key={item.key}
            onClick={() => loadSideData(item.key)}
            className="text-left px-3 py-2 rounded text-sm transition-colors"
            style={{ background: active === item.key ? 'var(--color-primary)' : 'transparent', color: active === item.key ? '#fff' : 'var(--color-text)' }}
          >
            {item.label}
          </button>
        ))}
      </div>

      {/* Main content */}
      <div className="flex-1 overflow-auto flex flex-col gap-4">
        {active === 'players' && (
          <>
            <div className="flex items-center gap-3">
              <input
                className="rounded px-3 py-1.5 text-sm border w-72"
                style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                placeholder="Search players..."
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
              <Button variant="outline" size="sm" onPress={loadPlayers} isDisabled={loading}>
                {loading ? <Spinner size="sm" color="current" /> : null}
                Refresh
              </Button>
            </div>

            {loading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  <thead>
                    <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                      {['ID', 'Name', 'Class', 'Map', 'Faction', 'Actions'].map(h => (
                        <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((player, i) => (
                      <tr key={player.id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-2 font-mono" style={{ color: 'var(--color-text-dim)' }}>{player.id}</td>
                        <td className="px-3 py-2 font-semibold" style={{ color: 'var(--color-text)' }}>
                          <div className="flex items-center">
                            <StatusDot status={player.online_status} />
                            {player.name}
                          </div>
                        </td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{player.class}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{player.map}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{player.faction_id || '—'}</td>
                        <td className="px-3 py-2">
                          <div className="flex gap-1 flex-wrap">
                            <Button size="sm" variant="ghost" onPress={() => { setSelectedPlayer(player); setShowInventory(true) }}>Inventory</Button>
                            <Button size="sm" variant="ghost" onPress={() => { setSelectedPlayer(player); setShowGiveItem(true) }}>Give Item</Button>
                            <Button size="sm" variant="ghost" onPress={() => { setSelectedPlayer(player); setShowActions(true) }}>Actions</Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}

        {active === 'currency' && (
          <div className="flex flex-col gap-3 h-full min-h-0">
            <div className="flex items-center gap-2 shrink-0">
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Currency</h3>
              <input
                className="flex-1 rounded px-3 py-1 text-sm border"
                style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                placeholder="Search player..."
                value={sideSearch}
                onChange={e => setSideSearch(e.target.value)}
              />
            </div>
            {sideLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <div className="overflow-auto rounded-lg flex-1 min-h-0" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  {tableHeader(['Player', 'Currency', 'Balance'])}
                  <tbody>
                    {filteredCurrency.map((row, i) => (
                      <tr key={`${row.player_id}-${row.currency_id}`} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-2">
                          {controllerToName.get(row.player_id) && <div className="font-semibold" style={{ color: 'var(--color-text)' }}>{controllerToName.get(row.player_id)}</div>}
                          <div className="font-mono" style={{ color: 'var(--color-text-dim)' }}>#{row.player_id}</div>
                        </td>
                        <td className="px-3 py-2 font-mono" style={{ color: 'var(--color-text-dim)' }}>{row.currency_id}</td>
                        <td className="px-3 py-2 font-semibold" style={{ color: 'var(--color-text)' }}>{row.balance.toLocaleString()}</td>
                      </tr>
                    ))}
                    {filteredCurrency.length === 0 && (
                      <tr><td colSpan={3} className="px-3 py-8 text-center" style={{ color: 'var(--color-text-dim)' }}>{sideSearch ? 'No matches' : 'No data'}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {active === 'factions' && (
          <div className="flex flex-col gap-3 h-full min-h-0">
            <div className="flex items-center gap-2 shrink-0">
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Factions</h3>
              <input
                className="flex-1 rounded px-3 py-1 text-sm border"
                style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                placeholder="Search player..."
                value={sideSearch}
                onChange={e => setSideSearch(e.target.value)}
              />
            </div>
            {sideLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <div className="overflow-auto rounded-lg flex-1 min-h-0" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  {tableHeader(['Player', 'Faction', 'Reputation', 'Scrips'])}
                  <tbody>
                    {filteredFactions.map((row, i) => (
                      <tr key={`${row.actor_id}-${row.faction_id}`} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-2">
                          {controllerToName.get(row.actor_id) && <div className="font-semibold" style={{ color: 'var(--color-text)' }}>{controllerToName.get(row.actor_id)}</div>}
                          <div className="font-mono" style={{ color: 'var(--color-text-dim)' }}>#{row.actor_id}</div>
                        </td>
                        <td className="px-3 py-2 font-semibold" style={{ color: 'var(--color-text)' }}>{row.faction_name}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{row.reputation.toLocaleString()}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{row.scrips.toLocaleString()}</td>
                      </tr>
                    ))}
                    {filteredFactions.length === 0 && (
                      <tr><td colSpan={4} className="px-3 py-8 text-center" style={{ color: 'var(--color-text-dim)' }}>{sideSearch ? 'No matches' : 'No data'}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {active === 'specs' && (
          <div className="flex flex-col gap-3 h-full min-h-0">
            <div className="flex items-center gap-2 shrink-0">
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Specs / XP</h3>
              <input
                className="flex-1 rounded px-3 py-1 text-sm border"
                style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                placeholder="Search player..."
                value={sideSearch}
                onChange={e => setSideSearch(e.target.value)}
              />
            </div>
            {sideLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <div className="overflow-auto rounded-lg flex-1 min-h-0" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  {tableHeader(['Player', 'Track', 'XP', 'Level'])}
                  <tbody>
                    {filteredSpecs.map((row, i) => (
                      <tr key={`${row.player_id}-${row.track_type}`} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-2">
                          {controllerToName.get(row.player_id) && <div className="font-semibold" style={{ color: 'var(--color-text)' }}>{controllerToName.get(row.player_id)}</div>}
                          <div className="font-mono" style={{ color: 'var(--color-text-dim)' }}>#{row.player_id}</div>
                        </td>
                        <td className="px-3 py-2 font-semibold" style={{ color: 'var(--color-text)' }}>{row.track_type}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{row.xp.toLocaleString()}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{row.level}</td>
                      </tr>
                    ))}
                    {filteredSpecs.length === 0 && (
                      <tr><td colSpan={4} className="px-3 py-8 text-center" style={{ color: 'var(--color-text-dim)' }}>{sideSearch ? 'No matches' : 'No data'}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {active === 'online' && (
          <div className="flex flex-col gap-3 h-full min-h-0">
            <div className="flex items-center gap-2 shrink-0">
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Online State</h3>
              <input
                className="flex-1 rounded px-3 py-1 text-sm border"
                style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                placeholder="Search player..."
                value={sideSearch}
                onChange={e => setSideSearch(e.target.value)}
              />
            </div>
            {sideLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <div className="overflow-auto rounded-lg flex-1 min-h-0" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  {tableHeader(['Player', 'Status', 'Last Seen', 'Map'])}
                  <tbody>
                    {filteredOnline.map((row, i) => (
                      <tr key={row.player_id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-2">
                          <div className="font-semibold" style={{ color: 'var(--color-text)' }}>{row.name}</div>
                          <div className="font-mono" style={{ color: 'var(--color-text-dim)' }}>#{row.player_id}</div>
                        </td>
                        <td className="px-3 py-2"><OnlineBadge status={row.status} /></td>
                        <td className="px-3 py-2 font-mono" style={{ color: 'var(--color-text-dim)' }}>{row.last_seen}</td>
                        <td className="px-3 py-2" style={{ color: 'var(--color-text-dim)' }}>{row.map}</td>
                      </tr>
                    ))}
                    {filteredOnline.length === 0 && (
                      <tr><td colSpan={4} className="px-3 py-8 text-center" style={{ color: 'var(--color-text-dim)' }}>{sideSearch ? 'No matches' : 'No data'}</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>

      {selectedPlayer && (
        <InventoryModal player={selectedPlayer} open={showInventory} onClose={() => setShowInventory(false)} />
      )}
      {selectedPlayer && (
        <GiveItemModal player={selectedPlayer} open={showGiveItem} onClose={() => setShowGiveItem(false)} />
      )}
      {selectedPlayer && (
        <PlayerActionsModal player={selectedPlayer} open={showActions} onClose={() => setShowActions(false)} />
      )}
    </div>
  )
}

// ── Inventory Modal ────────────────────────────────────────────────────────────

function InventoryModal({ player, open, onClose }: { player: Player; open: boolean; onClose: () => void }) {
  const [items, setItems] = useState<InventoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [vehicles, setVehicles] = useState<VehicleRow[]>([])
  const [vehiclesLoading, setVehiclesLoading] = useState(false)

  useEffect(() => {
    if (!open) {
      setVehicles([])
      return
    }
    setLoading(true)
    setVehiclesLoading(true)
    api.players.inventory(player.id)
      .then(setItems)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
    api.players.vehicles(player.account_id)
      .then(setVehicles)
      .catch(() => {})
      .finally(() => setVehiclesLoading(false))
  }, [open, player.id, player.account_id])

  const handleDelete = async (itemId: number) => {
    if (player.online_status === 'Online') {
      const ok = window.confirm('Player is online — deleting items may cause inventory desyncs. Continue?')
      if (!ok) return
    }
    try {
      await api.players.deleteItem(itemId)
      setItems(prev => prev.filter(i => i.id !== itemId))
      toast.success('Item deleted')
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepair = async (item: InventoryItem) => {
    try {
      await api.players.repairItem(item.id)
      setItems(prev => prev.map(i => i.id === item.id ? { ...i, durability: i.max_durability } : i))
      toast.success(`Repaired ${item.name || item.template_id}`)
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="full">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header><Modal.Heading>{player.name} — Inventory</Modal.Heading></Modal.Header>
            <Modal.Body>
              {loading ? (
                <div className="flex justify-center py-8"><Spinner size="lg" /></div>
              ) : items.length === 0 ? (
                <p style={{ color: 'var(--color-text-dim)' }}>No items found.</p>
              ) : (
                <>
                  <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418', maxHeight: '55vh', flex: 1, minHeight: 0 }}>
                    <table className="w-full text-xs">
                      <thead>
                        <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                          {['Template', 'Stack', 'Quality', 'Durability', ''].map(h => (
                            <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {items.map((item, i) => (
                          <tr key={item.id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                            <td className="px-3 py-1.5">
                              <div className="font-semibold" style={{ color: 'var(--color-text)' }}>{item.name || item.template_id}</div>
                              {item.name && (
                                <div className="text-xs font-mono" style={{ color: 'var(--color-text-dim)' }}>{item.template_id}</div>
                              )}
                            </td>
                            <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{item.stack_size}</td>
                            <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{item.quality}</td>
                            <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>
                              {item.durability} / {item.max_durability}
                            </td>
                            <td className="px-3 py-1.5">
                              <div className="flex gap-1">
                                {item.max_durability !== 'N/A' && (
                                  <Button size="sm" variant="ghost" onPress={() => handleRepair(item)}>Repair</Button>
                                )}
                                <Button size="sm" variant="danger-soft" onPress={() => handleDelete(item.id)}>X</Button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>

                  {/* Vehicles section */}
                  <div className="mt-4">
                    <div className="flex items-center gap-2 mb-2">
                      <span className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Vehicles</span>
                      {vehiclesLoading && <Spinner size="sm" color="current" />}
                    </div>
                    <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418', maxHeight: '25vh' }}>
                      <table className="w-full text-xs">
                        <thead>
                          <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                            {['Class', 'Location', 'Chassis', 'Name', 'Type'].map(h => (
                              <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {vehicles.map((v, i) => (
                            <tr key={v.id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                              <td className="px-3 py-1.5 font-semibold" style={{ color: 'var(--color-text)' }}>{v.class}</td>
                              <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{v.map || '—'}</td>
                              <td className="px-3 py-1.5" style={{ color: v.chassis_durability < 0.3 ? '#e88' : 'var(--color-text-dim)' }}>{Math.round(v.chassis_durability * 100)}%</td>
                              <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{v.vehicle_name || '—'}</td>
                              <td className="px-3 py-1.5">
                                {v.is_backup && <span className="px-1.5 py-0.5 rounded text-xs" style={{ background: '#1a1a2a', color: '#8888ff', border: '1px solid #3a3a6a' }}>Backup</span>}
                                {v.is_recovered && <span className="px-1.5 py-0.5 rounded text-xs" style={{ background: '#2a1a0a', color: '#f0a830', border: '1px solid #5a3a10' }}>Recovered</span>}
                              </td>
                            </tr>
                          ))}
                          {!vehiclesLoading && vehicles.length === 0 && (
                            <tr><td colSpan={5} className="px-3 py-6 text-center" style={{ color: 'var(--color-text-dim)' }}>No vehicles found</td></tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button onPress={onClose} variant="tertiary">Close</Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

// ── Give Item Modal ────────────────────────────────────────────────────────────

function GiveItemModal({ player, open, onClose }: { player: Player; open: boolean; onClose: () => void }) {
  const [templates, setTemplates] = useState<{id: string; name: string}[]>([])
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState('')
  const [qty, setQty] = useState(1)
  const [quality, setQuality] = useState(0)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    setLoading(true)
    api.players.templates().then(setTemplates).catch(() => {}).finally(() => setLoading(false))
    setQuery(''); setSelected(''); setQty(1); setQuality(1)
  }, [open])

  const filtered = useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates.filter(t => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q)).slice(0, 100)
  }, [templates, query])

  const pick = (t: {id: string; name: string}) => {
    setSelected(t.id)
    setQuery(t.name ? `${t.id}  —  ${t.name}` : t.id)
  }

  const handleSubmit = async () => {
    if (!selected) { toast.warning('Select a template'); return }
    setSubmitting(true)
    try {
      await api.players.giveItem(player.id, selected, qty, quality)
      toast.success(`Gave ${qty}× ${selected} to ${player.name}`)
      onClose()
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="full">
          <Modal.Dialog style={{ maxHeight: '85vh', display: 'flex', flexDirection: 'column' }}>
            <Modal.CloseTrigger />
            <Modal.Header><Modal.Heading>Give Item — {player.name}</Modal.Heading></Modal.Header>
            <Modal.Body style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '12px 16px' }}>
              {loading ? (
                <div className="flex justify-center py-6"><Spinner size="lg" /></div>
              ) : (
                <div className="flex flex-col gap-3 h-full overflow-hidden">
                  <div className="flex items-center gap-3 shrink-0">
                    <Button variant="tertiary" size="sm" onPress={onClose}>Cancel</Button>
                    <Button size="sm" onPress={handleSubmit} isDisabled={submitting || !selected}>
                      {submitting ? <Spinner size="sm" color="current" /> : null}
                      Give Item
                    </Button>
                    <div className="flex-1" />
                    <div className="flex gap-3">
                      <div className="flex flex-col gap-0.5">
                        <label className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Qty</label>
                        <input type="number" min={1} max={9999} value={qty}
                          onChange={e => setQty(Math.max(1, parseInt(e.target.value) || 1))}
                          className="rounded px-2 py-1 text-sm border w-20"
                          style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#3a3020', outline: 'none' }} />
                      </div>
                      <div className="flex flex-col gap-0.5">
                        <label className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Quality (0–5)</label>
                        <input type="number" min={0} max={5} value={quality}
                          onChange={e => setQuality(Math.max(0, Math.min(5, parseInt(e.target.value) || 0)))}
                          className="rounded px-2 py-1 text-sm border w-20"
                          style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#3a3020', outline: 'none' }} />
                      </div>
                    </div>
                  </div>
                  <div className="shrink-0">
                    {selected && (
                      <div className="text-xs mb-1 font-mono" style={{ color: 'var(--color-success)' }}>✓ {selected}</div>
                    )}
                    <input
                      className="rounded px-3 py-2 text-sm border w-full"
                      style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: selected ? 'var(--color-success)' : '#3a3020', outline: 'none' }}
                      placeholder="Search by template ID or item name..."
                      value={query}
                      onChange={e => { setQuery(e.target.value); setSelected('') }}
                      autoFocus
                    />
                  </div>
                  <div className="flex-1 overflow-y-auto rounded-lg min-h-0" style={{ border: '1px solid #2a2418', background: '#0a0806' }}>
                    {query.length === 0 ? (
                      <div className="flex items-center justify-center h-full py-8 text-xs" style={{ color: 'var(--color-text-dim)' }}>
                        Start typing to search {templates.length.toLocaleString()} templates
                      </div>
                    ) : filtered.length === 0 ? (
                      <div className="flex items-center justify-center h-full py-8 text-xs" style={{ color: 'var(--color-text-dim)' }}>
                        No results for "{query}"
                      </div>
                    ) : (
                      filtered.map(t => (
                        <div key={t.id}
                          className="flex items-baseline gap-3 px-3 py-2 cursor-pointer"
                          style={{ borderBottom: '1px solid #1a1610', background: selected === t.id ? '#241e12' : 'transparent' }}
                          onMouseEnter={e => { if (selected !== t.id) e.currentTarget.style.background = '#161208' }}
                          onMouseLeave={e => { if (selected !== t.id) e.currentTarget.style.background = 'transparent' }}
                          onClick={() => pick(t)}
                        >
                          <span className="font-mono text-xs shrink-0" style={{ color: selected === t.id ? 'var(--color-primary)' : 'var(--color-text)' }}>{t.id}</span>
                          {t.name && <span className="text-xs truncate" style={{ color: 'var(--color-text-dim)' }}>{t.name}</span>}
                        </div>
                      ))
                    )}
                  </div>
                </div>
              )}
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

// ── Player Actions Modal ───────────────────────────────────────────────────────

function PlayerActionsModal({ player, open, onClose }: { player: Player; open: boolean; onClose: () => void }) {
  const [section, setSection] = useState<ActionSection>('resources')
  const [busy, setBusy] = useState(false)

  // Resources
  const [currency, setCurrency] = useState(100)
  const [scrip, setScrip] = useState(100)
  const [intel, setIntel] = useState(100)

  // XP
  const [charXP, setCharXP] = useState(1000)
  const [charXPCurrent, setCharXPCurrent] = useState<{xp: number; level: number} | null>(null)

  // Faction
  const [factionId, setFactionId] = useState(player.faction_id || 0)
  const [repDelta, setRepDelta] = useState(100)

  // Specs
  const [playerSpecs, setPlayerSpecs] = useState<SpecTrack[]>([])
  const [playerKeystones, setPlayerKeystones] = useState<KeystoneRow[]>([])
  const [specsLoaded, setSpecsLoaded] = useState(false)
  const [specsLoading, setSpecsLoading] = useState(false)

  // Journey
  const [nodes, setNodes] = useState<JourneyNode[]>([])
  const [nodesLoaded, setNodesLoaded] = useState(false)
  const [nodesLoading, setNodesLoading] = useState(false)
  const [nodeSearch, setNodeSearch] = useState('')
  const [unlockFaction, setUnlockFaction] = useState('atreides')
  const [unlockPreset, setUnlockPreset] = useState('rank19_eligible')

  // Admin / Teleport
  const [partitions, setPartitions] = useState<TeleportLocation[]>([])
  const [selectedPartition, setSelectedPartition] = useState('')

  // History
  const [events, setEvents] = useState<GameEvent[]>([])
  const [dungeons, setDungeons] = useState<DungeonRecord[]>([])
  const [historyLoaded, setHistoryLoaded] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)

  useEffect(() => {
    if (!open) {
      setSection('resources')
      setNodesLoaded(false)
      setNodes([])
      setPlayerSpecs([])
      setPlayerKeystones([])
      setSpecsLoaded(false)
      setHistoryLoaded(false)
      setEvents([])
      setDungeons([])
      setCharXPCurrent(null)
    } else {
      setFactionId(player.faction_id > 0 ? player.faction_id : 1)
      api.players.partitions().then(setPartitions).catch(() => {})
      api.players.charXPCurrent(player.id).then(setCharXPCurrent).catch(() => {})
    }
  }, [open, player.faction_id])

  useEffect(() => {
    if (section === 'journey' && !nodesLoaded && open) {
      setNodesLoading(true)
      api.players.journey(player.account_id)
        .then(n => { setNodes(n); setNodesLoaded(true) })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setNodesLoading(false))
    }
  }, [section, nodesLoaded, open, player.account_id])

  useEffect(() => {
    if (section === 'specs' && !specsLoaded && open) {
      setSpecsLoading(true)
      Promise.all([
        api.players.specs_for(player.controller_id),
        api.players.keystones(player.controller_id),
      ]).then(([s, k]) => {
        setPlayerSpecs(s)
        setPlayerKeystones(k)
        setSpecsLoaded(true)
      })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setSpecsLoading(false))
    }
  }, [section, specsLoaded, open, player.controller_id])

  useEffect(() => {
    if (section === 'history' && !historyLoaded && open) {
      setHistoryLoading(true)
      Promise.all([
        api.players.events(player.id),
        api.players.dungeons(player.id)
      ]).then(([evts, dngns]) => {
        setEvents(evts)
        setDungeons(dngns)
        setHistoryLoaded(true)
      }).catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setHistoryLoading(false))
    }
  }, [section, historyLoaded, open, player.id])

  const run = async (fn: () => Promise<unknown>, label: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(label)
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const filteredNodes = useMemo(() => {
    if (!nodeSearch) return nodes
    const q = nodeSearch.toLowerCase()
    return nodes.filter(n => n.node_id.toLowerCase().includes(q))
  }, [nodes, nodeSearch])

  const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
    <input
      type="number" min={min} max={max} value={val}
      onChange={e => set(Math.max(min, Math.min(max, parseInt(e.target.value) || min)))}
      className="rounded px-2 py-1.5 text-sm border w-28"
      style={{ background: '#0d0b07', color: 'var(--color-text)', borderColor: '#3a3020', outline: 'none' }}
    />
  )

  const actionRow = (label: string, inputs: ReactNode, btnLabel: string, onAction: () => void, danger = false) => (
    <div className="flex items-end gap-3 py-3" style={{ borderBottom: '1px solid #1a1610' }}>
      <div className="w-36 shrink-0 text-sm" style={{ color: 'var(--color-text-dim)' }}>{label}</div>
      <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
      <Button size="sm" variant={danger ? 'danger-soft' : 'ghost'} onPress={onAction} isDisabled={busy}>{btnLabel}</Button>
    </div>
  )

  const onlineWarning = (
    <div className="text-xs px-3 py-2 rounded mb-3" style={{ background: '#1a1200', border: '1px solid #c9820a', color: '#f0a830' }}>
      ⚠ Player is online — XP changes may not take effect until they reconnect
    </div>
  )

  const formatDuration = (ms: number) => {
    const secs = Math.floor(ms / 1000)
    const m = Math.floor(secs / 60)
    const s = secs % 60
    return `${m}:${String(s).padStart(2, '0')}`
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="full">
          <Modal.Dialog style={{ height: '92vh', display: 'flex', flexDirection: 'column' }}>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>
                {player.name}
                <span className="ml-2 text-xs font-mono font-normal" style={{ color: 'var(--color-text-dim)' }}>
                  actor:{player.id} · ctrl:{player.controller_id} · acct:{player.account_id}
                </span>
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body style={{ display: 'flex', gap: 0, overflow: 'hidden', padding: 0, flex: 1 }}>
              {/* Section nav */}
              <div className="shrink-0 flex flex-col gap-1 p-3" style={{ borderRight: '1px solid #2a2418', background: '#0d0b07', minWidth: 120 }}>
                {ACTION_SECTIONS.map(s => (
                  <button key={s.key} onClick={() => setSection(s.key)}
                    className="text-left px-3 py-2 rounded text-sm transition-colors"
                    style={{ background: section === s.key ? 'var(--color-primary)' : 'transparent', color: section === s.key ? '#fff' : 'var(--color-text)' }}>
                    {s.label}
                  </button>
                ))}
              </div>

              {/* Section content */}
              <div className="flex-1 overflow-hidden flex flex-col p-4">

                {section === 'resources' && (
                  <div className="overflow-y-auto flex-1 flex flex-col">

                    {/* ── Currency & Resources ──────────────────────────────── */}
                    <div className="text-xs font-semibold uppercase tracking-widest px-1 py-2" style={{ color: 'var(--color-primary)', borderBottom: '1px solid #2a2418' }}>
                      Currency &amp; Resources
                    </div>
                    {actionRow('Give Currency', numInput(currency, setCurrency, 1, 9999999), 'Give',
                      () => run(() => api.players.giveCurrency(player.controller_id, currency), `Gave ${currency} Solari to ${player.name}`))}
                    {actionRow('Give Scrip', numInput(scrip, setScrip, 1, 9999999), 'Give',
                      () => run(() => api.players.giveScrip(player.controller_id, scrip), `Gave ${scrip} scrip to ${player.name}`))}
                    {actionRow('Award Intel', numInput(intel, setIntel, 1, 9999999), 'Award',
                      () => run(() => api.players.awardIntel(player.controller_id, intel), `Awarded ${intel} intel to ${player.name}`))}

                    {/* ── Character XP ──────────────────────────────────────── */}
                    <div className="text-xs font-semibold uppercase tracking-widest px-1 py-2 mt-4" style={{ color: 'var(--color-primary)', borderBottom: '1px solid #2a2418' }}>
                      Character XP
                    </div>
                    {player.online_status === 'Online' && onlineWarning}
                    {charXPCurrent && (
                      <div className="px-1 py-2 text-xs" style={{ color: 'var(--color-text-dim)' }}>
                        Current: <span style={{ color: 'var(--color-text)' }}>{charXPCurrent.xp.toLocaleString()} XP</span>
                        {' '}— Level <span style={{ color: 'var(--color-text)' }}>{charXPCurrent.level}</span>
                        <span style={{ color: '#555' }}> / 200</span>
                      </div>
                    )}
                    {actionRow('Award Char XP',
                      <div className="flex flex-col gap-0.5">
                        {numInput(charXP, setCharXP, 0, 344440)}
                        <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Max 344,440 (level 200)</span>
                      </div>,
                      'Award',
                      () => run(() => api.players.awardCharXP(player.id, charXP), `Awarded ${charXP} char XP to ${player.name}`)
                        .then(() => api.players.charXPCurrent(player.id).then(setCharXPCurrent).catch(() => {})))}

                    {/* ── Faction ───────────────────────────────────────────── */}
                    <div className="text-xs font-semibold uppercase tracking-widest px-1 py-2 mt-4" style={{ color: 'var(--color-primary)', borderBottom: '1px solid #2a2418' }}>
                      Faction Reputation
                    </div>
                    <div className="flex items-center gap-2 py-3" style={{ borderBottom: '1px solid #1a1610' }}>
                      <div className="w-36 shrink-0 text-sm" style={{ color: 'var(--color-text-dim)' }}>Faction</div>
                      <Select selectedKey={String(factionId)} onSelectionChange={k => setFactionId(Number(k))} className="w-40">
                        <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                        <Select.Popover>
                          <ListBox>
                            {FACTIONS.map(f => <ListBox.Item key={String(f.id)} id={String(f.id)} textValue={f.name}>{f.name}<ListBox.ItemIndicator /></ListBox.Item>)}
                          </ListBox>
                        </Select.Popover>
                      </Select>
                    </div>
                    {actionRow('Reputation',
                      <div className="flex flex-col gap-0.5">
                        {numInput(repDelta, setRepDelta, 0, 12474)}
                        <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Adds to current, max 12,474</span>
                      </div>,
                      'Give',
                      () => run(() => api.players.giveFactionRep(player.controller_id, factionId, repDelta), `Gave ${repDelta} rep (faction ${factionId}) to ${player.name}`))}

                  </div>
                )}

                {section === 'specs' && (
                  <div className="overflow-y-auto flex-1 flex flex-col gap-3">
                    {player.online_status === 'Online' && onlineWarning}
                    <div className="flex items-center gap-2 shrink-0">
                      <span className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>Specializations</span>
                      <Button size="sm" variant="ghost" isDisabled={specsLoading}
                        onPress={() => { setSpecsLoaded(false) }}>
                        {specsLoading ? <Spinner size="sm" color="current" /> : '↻'}
                      </Button>
                      <Button size="sm" variant="ghost" isDisabled={busy}
                        onPress={() => run(
                          () => api.players.grantAllKeystones(player.controller_id),
                          `Grant all keystones to ${player.name}`
                        ).then(() => setSpecsLoaded(false))}>
                        Grant Max Keystones
                      </Button>
                    </div>
                    {specsLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418', flex: 1, minHeight: 0 }}>
                        <table className="w-full text-xs">
                          <thead>
                            <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                              {['Track', 'XP', 'Level', '', ''].map((h, idx) => (
                                <th key={idx} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                              ))}
                            </tr>
                          </thead>
                          <tbody>
                            {XP_TRACKS.map((track, i) => {
                              const found = playerSpecs.find(s => s.track_type === track)
                              const currentXP = found ? found.xp : 0
                              const currentLevel = found ? found.level : 0
                              const trackKeystones = playerKeystones.filter(k => k.track === track)
                              return (
                                <tr key={track} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                                  <td className="px-3 py-2 font-semibold" style={{ color: 'var(--color-text)', verticalAlign: 'top' }}>
                                    <div>{track}</div>
                                    {trackKeystones.length > 0 && (
                                      <div className="flex flex-col gap-0.5 mt-1">
                                        {trackKeystones.map(k => (
                                          <div key={k.id} className="text-xs font-mono" style={{ color: 'var(--color-text-dim)' }}>
                                            ↳ {k.name.replace(/^DA_\w+Keystone_/, '').replace(/_/g, ' ')}
                                            {k.cost > 0 && <span style={{ color: '#5a4a2a', marginLeft: 4 }}>{k.cost}m</span>}
                                          </div>
                                        ))}
                                      </div>
                                    )}
                                  </td>
                                  <td className="px-3 py-2 font-mono" style={{ color: 'var(--color-text-dim)', verticalAlign: 'top' }}>{currentXP.toLocaleString()}</td>
                                  <td className="px-3 py-2 font-mono" style={{ color: 'var(--color-text-dim)', verticalAlign: 'top' }}>{currentLevel}</td>
                                  <td className="px-3 py-2">
                                    <Button size="sm" variant="ghost" isDisabled={busy}
                                      onPress={() => run(
                                        () => api.players.grantMaxSpec(player.controller_id, track),
                                        `Grant max ${track} spec to ${player.name}`
                                      ).then(() => {
                                        setPlayerSpecs(prev => {
                                          const exists = prev.find(s => s.track_type === track)
                                          if (exists) {
                                            return prev.map(s => s.track_type === track ? { ...s, xp: 44182, level: 100 } : s)
                                          }
                                          return [...prev, { player_id: player.controller_id, track_type: track, xp: 44182, level: 100 }]
                                        })
                                      })}>
                                      Grant Max
                                    </Button>
                                  </td>
                                  <td className="px-3 py-2">
                                    <Button size="sm" variant="danger-soft" isDisabled={busy}
                                      onPress={() => run(
                                        () => api.players.resetSpec(player.controller_id, track),
                                        `Reset ${track} spec for ${player.name}`
                                      ).then(() => {
                                        setPlayerSpecs(prev => prev.filter(s => s.track_type !== track))
                                      })}>
                                      Reset
                                    </Button>
                                  </td>
                                </tr>
                              )
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                )}

                {section === 'journey' && (
                  <div className="flex flex-col gap-3 flex-1 min-h-0">
                    {/* ── Progression Unlock ─────────────────────────────────── */}
                    <div className="rounded-lg p-3 shrink-0 flex flex-col gap-2" style={{ background: '#0f0d09', border: '1px solid #2a2418' }}>
                      <div className="text-xs font-semibold uppercase tracking-widest" style={{ color: 'var(--color-primary)' }}>Progression Unlock</div>
                      <div className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Atomically applies faction journey flags from BP_ProgressionUnlockComponent presets.</div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <Select selectedKey={unlockFaction} onSelectionChange={k => setUnlockFaction(String(k))} className="w-36">
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              <ListBox.Item key="atreides" id="atreides" textValue="Atreides">Atreides<ListBox.ItemIndicator /></ListBox.Item>
                              <ListBox.Item key="harkonnen" id="harkonnen" textValue="Harkonnen">Harkonnen<ListBox.ItemIndicator /></ListBox.Item>
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Select selectedKey={unlockPreset} onSelectionChange={k => setUnlockPreset(String(k))} className="w-48">
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              <ListBox.Item key="ch3_start" id="ch3_start" textValue="Ch3 Start">Ch3 Start<ListBox.ItemIndicator /></ListBox.Item>
                              <ListBox.Item key="rank19_eligible" id="rank19_eligible" textValue="Rank 19 Eligible">Rank 19 Eligible<ListBox.ItemIndicator /></ListBox.Item>
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Button size="sm" variant="secondary" isDisabled={busy}
                          onPress={() => run(
                            () => api.players.progressionUnlock(player.id, unlockFaction, unlockPreset),
                            `Applied ${unlockPreset} (${unlockFaction}) to ${player.name}`
                          ).then(() => { setNodesLoaded(false) })}>
                          Apply Unlock
                        </Button>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <input
                        className="flex-1 rounded px-2 py-1.5 text-xs border"
                        style={{ background: '#0d0b07', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                        placeholder="Filter nodes..."
                        value={nodeSearch}
                        onChange={e => setNodeSearch(e.target.value)}
                      />
                      <Button size="sm" variant="ghost" onPress={() => { setNodesLoaded(false) }} isDisabled={nodesLoading}>
                        {nodesLoading ? <Spinner size="sm" color="current" /> : '↻'}
                      </Button>
                      <Button size="sm" variant="danger-soft" isDisabled={busy}
                        onPress={() => run(() => api.players.journeyWipe(player.account_id), `Wiped all journey nodes for ${player.name}`)
                          .then(() => setNodes([]))}>
                        Wipe All
                      </Button>
                    </div>
                    {nodesLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <div className="overflow-y-auto rounded-lg flex-1 min-h-0" style={{ border: '1px solid #2a2418', background: '#0a0806' }}>
                        <table className="w-full text-xs">
                          <thead>
                            <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418', position: 'sticky', top: 0 }}>
                              {['Node ID', 'Done', 'Revealed', 'Reward', ''].map(h => (
                                <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                              ))}
                            </tr>
                          </thead>
                          <tbody>
                            {filteredNodes.map((n, i) => (
                              <tr key={n.node_id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                                <td className="px-3 py-1.5 font-mono" style={{ color: 'var(--color-text)' }}>{n.node_id}</td>
                                <td className="px-3 py-1.5">{n.is_complete ? '✓' : '—'}</td>
                                <td className="px-3 py-1.5">{n.is_revealed ? '✓' : '—'}</td>
                                <td className="px-3 py-1.5">{n.has_pending_reward ? '✓' : '—'}</td>
                                <td className="px-3 py-1.5">
                                  <div className="flex gap-1">
                                    <Button size="sm" variant="ghost" isDisabled={busy}
                                      onPress={() => run(
                                        () => api.players.journeyComplete(player.account_id, n.node_id),
                                        `Completed ${n.node_id}`
                                      ).then(() => {
                                        setNodes(prev => prev.map(x =>
                                          x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                            ? { ...x, is_complete: true, is_revealed: true }
                                            : x
                                        ))
                                      })}>
                                      {n.is_complete ? '↻ Re-do' : 'Complete'}
                                    </Button>
                                    <Button size="sm" variant="danger-soft" isDisabled={busy}
                                      onPress={() => run(
                                        () => api.players.journeyReset(player.account_id, n.node_id),
                                        `Reset ${n.node_id}`
                                      ).then(() => {
                                        setNodes(prev => prev.map(x =>
                                          x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                            ? { ...x, is_complete: false, has_pending_reward: false }
                                            : x
                                        ))
                                      })}>
                                      Reset
                                    </Button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                        {filteredNodes.length === 0 && !nodesLoading && (
                          <div className="text-center py-8 text-xs" style={{ color: 'var(--color-text-dim)' }}>
                            {nodes.length === 0 ? 'No journey nodes found' : 'No matching nodes'}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )}

                {section === 'admin' && (
                  <div className="overflow-y-auto flex-1">
                    {actionRow('Delete Tutorials', <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Removes all tutorial completion records</span>, 'Delete',
                      () => run(() => api.players.deleteTutorials(player.id), `Deleted tutorials for ${player.name}`), true)}
                    {actionRow('Wipe Codex', <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Clears all codex discoveries</span>, 'Wipe',
                      () => run(() => api.players.wipeCodex(player.account_id), `Wiped codex for ${player.name}`), true)}
                    {actionRow('Kick Player', <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Force-disconnect from server</span>, 'Kick',
                      () => run(() => api.players.kick(player.id), `Kicked ${player.name}`), true)}
                    {/* Teleport */}
                    <div className="flex items-end gap-3 py-3" style={{ borderBottom: '1px solid #1a1610' }}>
                      <div className="w-36 shrink-0 text-sm" style={{ color: 'var(--color-text-dim)' }}>Teleport</div>
                      <div className="flex items-end gap-2 flex-1">
                        <select
                          value={selectedPartition}
                          onChange={e => setSelectedPartition(e.target.value)}
                          className="rounded px-2 py-1.5 text-sm border flex-1"
                          style={{ background: '#0d0b07', color: 'var(--color-text)', borderColor: '#3a3020', outline: 'none' }}
                        >
                          <option value="">Select location...</option>
                          {partitions.map(p => (
                            <option key={p.name} value={p.name}>{p.name}</option>
                          ))}
                        </select>
                        <Button size="sm" variant="ghost" isDisabled={busy || !selectedPartition || player.online_status === 'Online'}
                          onPress={() => run(() => api.players.teleport(player.fls_id, selectedPartition), `Teleported ${player.name} to ${selectedPartition}`)}>
                          Move
                        </Button>
                      </div>
                      {player.online_status === 'Online' && (
                        <span className="text-xs" style={{ color: '#888' }}>Player must be offline</span>
                      )}
                    </div>
                  </div>
                )}

                {section === 'history' && (
                  <div className="flex flex-col gap-4 flex-1 min-h-0 overflow-y-auto">
                    {historyLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <>
                        {/* Game Events */}
                        <div className="flex flex-col gap-2 min-h-0">
                          <h4 className="text-sm font-semibold shrink-0" style={{ color: 'var(--color-primary)' }}>Game Events</h4>
                          <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418', maxHeight: '40vh' }}>
                            <table className="w-full text-xs">
                              <thead>
                                <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418', position: 'sticky', top: 0 }}>
                                  {['Time', 'Map', 'Event Type', 'Location'].map(h => (
                                    <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                                  ))}
                                </tr>
                              </thead>
                              <tbody>
                                {events.map((evt, i) => (
                                  <tr key={`${evt.actor_id}-${evt.universe_time}-${i}`} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                                    <td className="px-3 py-1.5 font-mono" style={{ color: 'var(--color-text-dim)' }}>{evt.universe_time}</td>
                                    <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{evt.map}</td>
                                    <td className="px-3 py-1.5" style={{ color: 'var(--color-text)' }}>{evt.event_type}</td>
                                    <td className="px-3 py-1.5 font-mono" style={{ color: 'var(--color-text-dim)' }}>
                                      {Math.round(evt.x)}, {Math.round(evt.y)}, {Math.round(evt.z)}
                                    </td>
                                  </tr>
                                ))}
                                {events.length === 0 && (
                                  <tr><td colSpan={4} className="px-3 py-6 text-center" style={{ color: 'var(--color-text-dim)' }}>No events</td></tr>
                                )}
                              </tbody>
                            </table>
                          </div>
                        </div>

                        {/* Dungeon Records */}
                        <div className="flex flex-col gap-2 min-h-0">
                          <h4 className="text-sm font-semibold shrink-0" style={{ color: 'var(--color-primary)' }}>Dungeon Records</h4>
                          <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418', maxHeight: '40vh' }}>
                            <table className="w-full text-xs">
                              <thead>
                                <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418', position: 'sticky', top: 0 }}>
                                  {['Dungeon', 'Difficulty', 'Duration', 'Party Size'].map(h => (
                                    <th key={h} className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}>{h}</th>
                                  ))}
                                </tr>
                              </thead>
                              <tbody>
                                {dungeons.map((d, i) => (
                                  <tr key={d.completion_id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                                    <td className="px-3 py-1.5 font-semibold" style={{ color: 'var(--color-text)' }}>{d.dungeon_id}</td>
                                    <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{d.difficulty}</td>
                                    <td className="px-3 py-1.5 font-mono" style={{ color: 'var(--color-text-dim)' }}>{formatDuration(d.duration_ms)}</td>
                                    <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{d.players_num}</td>
                                  </tr>
                                ))}
                                {dungeons.length === 0 && (
                                  <tr><td colSpan={4} className="px-3 py-6 text-center" style={{ color: 'var(--color-text-dim)' }}>No dungeon records</td></tr>
                                )}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
