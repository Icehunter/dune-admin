import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Drawer, Select, ListBox, SearchField, Spinner, toast } from '@heroui/react'
import { MapContainer, ImageOverlay, CircleMarker, Marker, Tooltip, useMapEvents, useMap } from 'react-leaflet'
import L, { CRS, type LatLngBoundsExpression } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { api, ApiError } from '../api/client'
import type { MapMarker, Player } from '../api/client'
import { Icon, PageHeader } from '../dune-ui'
import { useAutoRefresh } from '../hooks/useAutoRefresh'

const IMG_W = 1200
const IMG_H = 1200
const IMAGE_BOUNDS: LatLngBoundsExpression = [[0, 0], [IMG_H, IMG_W]]
const POLL_MS = 30000

// Sprite sheet: 11 cols × 12 rows, each icon 64×64px
// Positions extracted from the reference tool's HTML (object-position / 64).
const SPRITE_URL = `${import.meta.env.BASE_URL}map-icons.webp`
const SPRITE_COLS = 11
const SPRITE_ROWS = 12
const SPRITE_CELL = 64

// (col, row) exact positions from the reference tool — do not guess.
const ICON_POS: Record<string, [number, number]> = {
  // Loot / Items
  basic: [3, 0], vbasic: [3, 0], wbasic: [3, 0], ebasic: [3, 0], rbasic: [3, 0], srbasic: [3, 0],
  rare: [1, 0], vrare: [1, 0], wrare: [1, 0], drare: [1, 0],
  ultra_rare: [1, 1],
  small_ultra_rare: [6, 0],
  ammo: [2, 1], vammo: [2, 1], wammo: [2, 1], uammo: [2, 1], dammo: [2, 1],
  medical: [3, 1],
  weapon: [9, 0],
  corpse: [2, 0], vcorpse: [2, 0], fcorpse: [2, 0],
  fuel: [1, 2], vfuel: [1, 2], wfuel: [1, 2], dfuel: [1, 2], ufuel: [1, 2], owfuel: [1, 2],
  contract: [8, 0],
  refinery: [4, 3],
  water_tank: [1, 3],
  buried_treasure: [4, 9],
  treasure_loot_container: [3, 0],
  // Locations
  cave: [0, 0],
  intel_point: [4, 1],
  enemy_camp: [4, 0],
  primitive: [5, 0], kirab_camp: [5, 0],
  shipwreck: [7, 0],
  trading_post: [9, 1],
  taxi: [4, 6],
  bank: [10, 6],
  discoverable: [6, 7],
  exploration: [9, 2],
  // Vehicles
  buggy: [2, 3], ebuggy: [2, 3],
  bike: [10, 2],
  // Trainers
  bene_gesserit_trainer: [1, 4],
  mentat: [9, 4],
  planetologist: [8, 2],
  swordmaster: [1, 5],
  trooper: [10, 1],
  // IDs / Keys
  blue_id_band: [6, 2],
  green_id_band: [0, 1],
  orange_id_band: [5, 2],
  purple_id_band: [10, 0],
  red_id_band: [5, 1],
  // Resources — large
  spice_field_small: [10, 7],
  spice_field_medium: [0, 8],
  spice_field_large: [1, 8],
  agave_seeds: [1, 9],
  azurite: [8, 8], azurite_pickup: [8, 8], // Copper Ore
  basalt: [5, 8], basalt_pickup: [5, 8],
  bauxite: [7, 8], bauxite_pickup: [7, 8], // Aluminium Ore
  dolomite: [10, 8], dolomite_pickup: [10, 8], // Carbon Ore
  erythrite: [0, 9], erythrite_pickup: [0, 9],
  fiber_plant: [6, 8], plant_fiber: [6, 8],
  fuel_cells: [5, 9],
  jasmium: [3, 9], jasmium_crystal: [3, 9],
  magnetite: [2, 9], magnetite_pickup: [2, 9], // Iron Ore
  primrose_field: [9, 7],
  rhyolite: [9, 8], rhyolite_pickup: [9, 8], // Granite Stone
  scrap_electronics: [7, 9],
  scrap_metal: [6, 9],
  stravidium: [4, 8],
  titanium_ore: [3, 8],
  // Vendors
  barkeep: [0, 7],
  base_vendor: [6, 6],
  landsraad_vendor: [2, 7],
  scrap_trader: [9, 6],
  spice_merchant: [7, 6],
  vehicle_vendor: [5, 6],
  water_seller: [8, 6],
  weapons_merchant: [1, 7],
  banker: [10, 6],
  // NPCs (by faction)
  atreides_npc: [3, 4],
  harkonnen_npc: [8, 4],
  fremen_npc: [0, 6],
  bene_gesserit_npc: [10, 5],
  choam_npc: [7, 5],
  bandits_npc: [3, 5],
  sardaukar_npc: [9, 5],
  smugglers_npc: [6, 5],
  spacing_guild_npc: [8, 5],
  unaffiliated_npc: [7, 1],
  // Landsraad houses
  alexin: [1, 6], argosaz: [0, 2], dyvetz: [7, 4], ecaz: [10, 4],
  hagal: [4, 4], hurata: [9, 3], imota: [5, 5], kenola: [3, 3],
  lindaren: [5, 4], maros: [8, 7], mikarrol: [4, 5], moritani: [6, 4],
  mutelli: [4, 7], novebruns: [8, 3], richese: [2, 4], sor: [2, 5],
  spinette: [2, 6], taligari: [0, 3], thorvald: [0, 5], tseida: [7, 3],
  varota: [3, 7], vernius: [0, 4], wallach: [5, 7], wayku: [7, 7], wydras: [8, 1],
}

// Per-category dot colors for non-sprite fallback
const CAT_COLOR: Record<string, string> = {
  player: '#3b9dff', vehicle: '#5fd35a', base: '#e0a13a',
  resources: '#f5a623', locations: '#9b59b6', npcs: '#e74c3c',
  vendors: '#2ecc71', landsraad: '#e91e8c', static: '#7f8c8d',
}

// Map spawn type → category
const TYPE_CATEGORY: Record<string, string> = {
  basic: 'resources', vbasic: 'resources', wbasic: 'resources', ebasic: 'resources', rbasic: 'resources', srbasic: 'resources',
  rare: 'resources', vrare: 'resources', wrare: 'resources', drare: 'resources',
  ultra_rare: 'resources', ammo: 'resources', vammo: 'resources', wammo: 'resources', uammo: 'resources', dammo: 'resources',
  medical: 'resources', weapon: 'resources', corpse: 'resources', vcorpse: 'resources', fcorpse: 'resources',
  fuel: 'resources', vfuel: 'resources', wfuel: 'resources', dfuel: 'resources', ufuel: 'resources', owfuel: 'resources',
  contract: 'resources', refinery: 'resources', water_tank: 'resources', buried_treasure: 'resources',
  treasure_loot_container: 'resources',
  enemy_camp: 'locations', primitive: 'locations', kirab_camp: 'locations', intel_point: 'locations',
  buggy: 'vehicles', ebuggy: 'vehicles',
  spice_field_small: 'resources', spice_field_medium: 'resources', spice_field_large: 'resources',
  basalt: 'resources', basalt_pickup: 'resources',
  fiber_plant: 'resources', plant_fiber: 'resources',
  bauxite: 'resources', bauxite_pickup: 'resources',
  agave_seeds: 'resources', erythrite: 'resources', erythrite_pickup: 'resources',
  jasmium: 'resources', jasmium_crystal: 'resources',
  scrap_electronics: 'resources', scrap_metal: 'resources',
  fuel_cells: 'resources',
  azurite: 'resources', azurite_pickup: 'resources',
  dolomite: 'resources', dolomite_pickup: 'resources',
  magnetite: 'resources', magnetite_pickup: 'resources',
  rhyolite: 'resources', rhyolite_pickup: 'resources',
  primrose_field: 'resources', stravidium: 'resources', titanium_ore: 'resources',
  static: 'static',
}

// Friendly display names for types
const TYPE_LABELS: Record<string, string> = {
  basic: 'Basic', vbasic: 'Basic', wbasic: 'Basic', ebasic: 'Basic', rbasic: 'Basic', srbasic: 'Basic',
  rare: 'Rare', vrare: 'Rare', wrare: 'Rare', drare: 'Rare',
  ultra_rare: 'Ultra Rare', ammo: 'Ammo', vammo: 'Ammo', wammo: 'Ammo', uammo: 'Ammo', dammo: 'Ammo',
  medical: 'Medical', weapon: 'Weapon', corpse: 'Corpse', vcorpse: 'Corpse', fcorpse: 'Corpse',
  fuel: 'Fuel', vfuel: 'Fuel', wfuel: 'Fuel', dfuel: 'Fuel', ufuel: 'Fuel', owfuel: 'Fuel',
  contract: 'Contract', refinery: 'Refinery', water_tank: 'Water Tank',
  treasure_loot_container: 'Loot Container',
  enemy_camp: 'Enemy Camp', primitive: 'Primitive Camp', kirab_camp: 'Kirab Camp',
  intel_point: 'Intel Point', buggy: 'Buggy', ebuggy: 'Buggy',
  spice_field_small: 'Small Spice', spice_field_medium: 'Medium Spice', spice_field_large: 'Large Spice',
  basalt: 'Basalt Stone', basalt_pickup: 'Basalt (Node)',
  fiber_plant: 'Plant Fiber', plant_fiber: 'Plant Fiber',
  bauxite: 'Aluminum Ore', bauxite_pickup: 'Aluminum (Node)',
  agave_seeds: 'Agave Seeds',
  erythrite: 'Erythrite Crystal', erythrite_pickup: 'Erythrite (Node)',
  jasmium: 'Jasmium Crystal', jasmium_crystal: 'Jasmium Crystal',
  scrap_electronics: 'Scrap Electronics', scrap_metal: 'Scrap Metal',
  fuel_cells: 'Fuel Cells',
  azurite: 'Copper Ore', azurite_pickup: 'Copper (Node)',
  dolomite: 'Carbon Ore', dolomite_pickup: 'Carbon (Node)',
  magnetite: 'Iron Ore', magnetite_pickup: 'Iron (Node)',
  rhyolite: 'Granite Stone', rhyolite_pickup: 'Granite (Node)',
  primrose_field: 'Primrose Field', stravidium: 'Stravidium', titanium_ore: 'Titanium',
  static: 'Static Object',
}

// Map types that should be merged (same visual/filter group)
const TYPE_MERGE_KEY: Record<string, string> = {
  vbasic: 'basic', wbasic: 'basic', ebasic: 'basic', rbasic: 'basic', srbasic: 'basic',
  vrare: 'rare', wrare: 'rare', drare: 'rare',
  vammo: 'ammo', wammo: 'ammo', uammo: 'ammo', dammo: 'ammo',
  vcorpse: 'corpse', fcorpse: 'corpse',
  vfuel: 'fuel', wfuel: 'fuel', dfuel: 'fuel', ufuel: 'fuel', owfuel: 'fuel',
  ebuggy: 'buggy',
  basalt_pickup: 'basalt',
  bauxite_pickup: 'bauxite',
  erythrite_pickup: 'erythrite',
  jasmium_crystal: 'jasmium',
  azurite_pickup: 'azurite',
  dolomite_pickup: 'dolomite',
  magnetite_pickup: 'magnetite',
  rhyolite_pickup: 'rhyolite',
  plant_fiber: 'fiber_plant',
}

function filterKey(type: string): string {
  return TYPE_MERGE_KEY[type] ?? type
}

type Bounds = { minX: number, maxX: number, minY: number, maxY: number, flipX?: boolean, flipY?: boolean }
type MapCfg = Bounds & { key: string, label: string, image?: string, spawnFile?: string, hasLiveData?: boolean }

const MAPS: MapCfg[] = [
  {
    key: 'HaggaBasin', label: 'Hagga Basin', image: 'hagga-basin.webp', spawnFile: 'hagga',
    hasLiveData: true,
    minX: -437871, maxX: 350539, minY: -462011, maxY: 376267, flipY: true,
  },
  {
    key: 'DeepDesert', label: 'Deep Desert', image: 'deepdesert.webp', spawnFile: 'deepdesert',
    hasLiveData: true,
    minX: -1300000, maxX: 1200000, minY: -1300000, maxY: 1200000,
  },
  {
    key: 'Arrakeen', label: 'Arrakeen', image: 'arrakeen.webp', spawnFile: 'arrakeen',
    hasLiveData: false,
    minX: -32000, maxX: 17000, minY: -10000, maxY: 9500, flipY: true,
  },
  {
    key: 'HarkoVillage', label: 'Harko Village', image: 'harko.webp', spawnFile: 'harko',
    hasLiveData: false,
    minX: -5000, maxX: 14500, minY: -5500, maxY: 32000,
  },
]

const CALIB_LS_KEY = 'dune_admin_livemap_calib'

type SpawnEntry = { type: string, label?: string, category: string, x: number, y: number, z?: number }
type SpawnFile = { spawns: SpawnEntry[] }

// ── Utility ──────────────────────────────────────────────────────────────────

function clamp01(v: number) {
  if (v < 0) return 0
  if (v > 1) return 1
  return v
}

function worldToLatLng(x: number, y: number, cfg: Bounds): [number, number] {
  const normX = (x - cfg.minX) / (cfg.maxX - cfg.minX)
  const normY = (y - cfg.minY) / (cfg.maxY - cfg.minY)
  const fracX = clamp01(cfg.flipX ? 1 - normX : normX)
  const fracYup = clamp01(cfg.flipY ? 1 - normY : normY)
  return [fracYup * IMG_H, fracX * IMG_W]
}

function latLngToWorld(lat: number, lng: number, cfg: Bounds): { x: number, y: number } {
  const fracX = lng / IMG_W
  const fracYup = lat / IMG_H
  const rawX = cfg.flipX ? 1 - fracX : fracX
  const rawY = cfg.flipY ? 1 - fracYup : fracYup
  return {
    x: rawX * (cfg.maxX - cfg.minX) + cfg.minX,
    y: rawY * (cfg.maxY - cfg.minY) + cfg.minY,
  }
}

type CalibPoint = { wx: number, wy: number, fracX: number, fracYup: number }

function solveBounds(pts: CalibPoint[]): Bounds | null {
  if (pts.length < 2) return null
  const a = pts[0]
  const b = pts[pts.length - 1]
  if (b.wx === a.wx || b.wy === a.wy || b.fracX === a.fracX || b.fracYup === a.fracYup) return null
  const sX = (b.fracX - a.fracX) / (b.wx - a.wx)
  const iX = a.fracX - sX * a.wx
  const sY = (b.fracYup - a.fracYup) / (b.wy - a.wy)
  const iY = a.fracYup - sY * a.wy
  const flipY = sY < 0
  const minX = -iX / sX
  const maxX = (1 - iX) / sX
  const R = flipY ? -1 / sY : 1 / sY
  const minY = flipY ? (iY - 1) * R : -iY * R
  return { minX, maxX, minY, maxY: minY + R, flipY }
}

function loadCalib(): Record<string, Bounds> {
  try {
    return JSON.parse(localStorage.getItem(CALIB_LS_KEY) ?? '{}') as Record<string, Bounds>
  }
  catch {
    return {}
  }
}

function InvalidateOnActive({ active }: { active: boolean }) {
  const map = useMap()
  useEffect(() => {
    if (active) {
      const id = setTimeout(() => {
        map.invalidateSize()
        map.fitBounds(IMAGE_BOUNDS)
      }, 50)
      return () => clearTimeout(id)
    }
  }, [active, map])
  return null
}

function MapClickCapture({ active, onPick }: { active: boolean, onPick: (lat: number, lng: number) => void }) {
  useMapEvents({
    click(e) {
      if (active) onPick(e.latlng.lat, e.latlng.lng)
    },
  })
  return null
}

function spawnRadius(category: string) {
  return category === 'resources' || category === 'static' ? 2 : 4
}

// ── Sprite icon ───────────────────────────────────────────────────────────────

// SpriteIcon renders a single icon from the sprite sheet.
// size: desired display size in CSS px (default 22 = 0.35 × 64, matches reference tool).
function SpriteIcon({ type, size = 22 }: { type: string, size?: number }) {
  const pos = ICON_POS[type]
  if (!pos) return null
  const [col, row] = pos
  const scale = size / SPRITE_CELL
  const bw = SPRITE_COLS * SPRITE_CELL * scale
  const bh = SPRITE_ROWS * SPRITE_CELL * scale
  const bx = -(col * SPRITE_CELL * scale)
  const by = -(row * SPRITE_CELL * scale)
  return (
    <span
      className="inline-block shrink-0"
      style={{
        width: size,
        height: size,
        backgroundImage: `url(${SPRITE_URL})`,
        backgroundPosition: `${bx}px ${by}px`,
        backgroundSize: `${bw}px ${bh}px`,
        backgroundRepeat: 'no-repeat',
        imageRendering: 'pixelated',
      }}
    />
  )
}

// Creates a Leaflet DivIcon that renders a sprite icon centered on the marker point.
// size: icon display size in CSS px. Returns null when no sprite mapping exists.
function makeSpriteDivIcon(type: string, size: number): L.DivIcon | null {
  const pos = ICON_POS[type]
  if (!pos) return null
  const [col, row] = pos
  const scale = size / SPRITE_CELL
  const bw = SPRITE_COLS * SPRITE_CELL * scale
  const bh = SPRITE_ROWS * SPRITE_CELL * scale
  const bx = -(col * SPRITE_CELL * scale)
  const by = -(row * SPRITE_CELL * scale)
  const html = `<span style="display:inline-block;width:${size}px;height:${size}px;background-image:url(${SPRITE_URL});background-position:${bx}px ${by}px;background-size:${bw}px ${bh}px;background-repeat:no-repeat;image-rendering:pixelated"></span>`
  return L.divIcon({ html, iconSize: [size, size], iconAnchor: [size / 2, size / 2], className: '' })
}

// ── Filter Drawer ─────────────────────────────────────────────────────────────

const LIVE_TYPES = ['players', 'vehicles', 'bases'] as const

const CATEGORY_GROUPS: { id: string, labelKey: string }[] = [
  { id: 'resources', labelKey: 'liveMap.filterResources' },
  { id: 'locations', labelKey: 'liveMap.filterLocations' },
  { id: 'npcs', labelKey: 'liveMap.filterNPCs' },
  { id: 'vendors', labelKey: 'liveMap.filterVendors' },
  { id: 'landsraad', labelKey: 'liveMap.filterLandsraad' },
  { id: 'static', labelKey: 'liveMap.filterStaticObjects' },
]

function FilterDrawer({
  open, onClose, filter, onToggle, onToggleCategory, spawns,
}: {
  open: boolean
  onClose: () => void
  filter: Record<string, boolean>
  onToggle: (key: string) => void
  onToggleCategory: (category: string, on: boolean) => void
  spawns: SpawnEntry[]
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  // Build unique type list per category with counts
  const typesByCategory = useMemo(() => {
    const map: Record<string, Map<string, { label: string, count: number }>> = {}
    spawns.forEach((s) => {
      const cat = s.category
      if (!map[cat]) map[cat] = new Map()
      const key = filterKey(s.type)
      const label = TYPE_LABELS[key] ?? s.label ?? s.type.replace(/_/g, ' ')
      const existing = map[cat].get(key)
      map[cat].set(key, { label, count: (existing?.count ?? 0) + 1 })
    })
    return map
  }, [spawns])

  const LIVE_LABELS: Record<string, string> = {
    players: t('liveMap.players'),
    vehicles: t('liveMap.vehicles'),
    bases: t('liveMap.filterBases'),
  }

  type TypeRowProps = { typeKey: string, label: string, count: number, category: string }
  function TypeRow({ typeKey, label, count, category }: TypeRowProps) {
    const isOn = filter[typeKey] ?? filter[category] ?? false
    return (
      <label className="flex items-center gap-2 py-1.5 px-3 cursor-pointer hover:bg-surface-secondary rounded-[var(--radius)] select-none">
        <input
          type="checkbox"
          checked={isOn}
          onChange={() => onToggle(typeKey)}
          className="h-3.5 w-3.5 accent-accent shrink-0"
        />
        <SpriteIcon type={typeKey} size={18} />
        {!ICON_POS[typeKey] && (
          <span style={{ color: CAT_COLOR[category] }} className="shrink-0">●</span>
        )}
        <span className="flex-1 text-xs text-foreground truncate">{label}</span>
        <span className="text-xs text-muted tabular-nums shrink-0">{count.toLocaleString()}</span>
      </label>
    )
  }

  function CategorySection({ group }: { group: (typeof CATEGORY_GROUPS)[number] }) {
    const items = typesByCategory[group.id]
    if (!items?.size) return null
    const isExpanded = expanded[group.id] ?? false
    const allOn = [...items.keys()].every((k) => filter[k] ?? filter[group.id] ?? false)
    const anyOn = [...items.keys()].some((k) => filter[k] ?? filter[group.id] ?? false)
    const q = search.toLowerCase()
    const filteredItems = q
      ? [...items.entries()].filter(([k, v]) => v.label.toLowerCase().includes(q) || k.toLowerCase().includes(q))
      : [...items.entries()]

    if (q && filteredItems.length === 0) return null

    return (
      <div className="mb-1">
        <div className="flex items-center gap-1 px-2 py-1.5">
          <input
            type="checkbox"
            checked={allOn}
            ref={(el) => { if (el) el.indeterminate = !allOn && anyOn }}
            onChange={(e) => onToggleCategory(group.id, e.target.checked)}
            className="h-3.5 w-3.5 accent-accent shrink-0"
          />
          <button
            type="button"
            className="flex-1 flex items-center gap-1.5 text-left"
            onClick={() => setExpanded((e) => ({ ...e, [group.id]: !e[group.id] }))}
          >
            <span style={{ color: CAT_COLOR[group.id] }} className="text-xs shrink-0">●</span>
            <span className="text-xs font-medium text-muted uppercase tracking-wide">{t(group.labelKey as never)}</span>
            <span className="text-xs text-muted/60 ml-1">
              {[...items.values()].reduce((s, v) => s + v.count, 0).toLocaleString()}
            </span>
            <Icon
              name={isExpanded || q ? 'chevron-down' : 'chevron-right'}
              className="size-3 text-muted ml-auto"
            />
          </button>
        </div>
        {(isExpanded || !!q) && (
          <div className="ml-1">
            {filteredItems.map(([key, { label, count }]) => (
              <TypeRow key={key} typeKey={key} label={label} count={count} category={group.id} />
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <Drawer>
      <Drawer.Backdrop
        variant="transparent"
        isOpen={open}
        onOpenChange={(v) => !v && onClose()}
      >
        <Drawer.Content placement="right">
          <Drawer.Dialog className="w-72 flex flex-col bg-background" style={{ zIndex: 1100 }}>
            <Drawer.Header className="border-b border-border px-3 py-2.5 shrink-0">
              <div className="flex items-center justify-between w-full">
                <span className="font-semibold text-foreground text-sm">{t('liveMap.filter')}</span>
                <Drawer.CloseTrigger>
                  <Button size="sm" variant="ghost" isIconOnly aria-label="close">
                    <Icon name="x" className="size-4" />
                  </Button>
                </Drawer.CloseTrigger>
              </div>
            </Drawer.Header>

            {/* Search */}
            <div className="px-2 py-2 border-b border-border shrink-0">
              <SearchField
                aria-label={t('liveMap.filter')}
                value={search}
                onChange={setSearch}
              >
                <SearchField.Group>
                  <SearchField.SearchIcon />
                  <SearchField.Input placeholder="Find filter…" />
                  <SearchField.ClearButton />
                </SearchField.Group>
              </SearchField>
            </div>

            <Drawer.Body className="flex-1 overflow-y-auto py-2 px-1">
              {/* Live section */}
              {!search && (
                <div className="mb-2">
                  <div className="px-3 py-1 text-xs font-medium text-muted uppercase tracking-wide">
                    {t('liveMap.filterLive')}
                  </div>
                  {LIVE_TYPES.map((id) => (
                    <label key={id} className="flex items-center gap-2 py-1.5 px-3 cursor-pointer hover:bg-surface-secondary rounded-[var(--radius)] select-none">
                      <input
                        type="checkbox"
                        checked={filter[id] ?? false}
                        onChange={() => onToggle(id)}
                        className="h-3.5 w-3.5 accent-accent shrink-0"
                      />
                      <span style={{ color: CAT_COLOR[id] }} className="text-xs shrink-0">●</span>
                      <span className="flex-1 text-xs text-foreground">{LIVE_LABELS[id]}</span>
                    </label>
                  ))}
                </div>
              )}

              {/* Static category sections */}
              {CATEGORY_GROUPS.map((group) => (
                <CategorySection key={group.id} group={group} />
              ))}
            </Drawer.Body>
          </Drawer.Dialog>
        </Drawer.Content>
      </Drawer.Backdrop>
    </Drawer>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

const LIVE_FILTER_DEFAULTS: Record<string, boolean> = {
  players: true, vehicles: true, bases: true,
}

export default function LiveMapTab({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation()
  const [mapKey, setMapKey] = useState<string>('HaggaBasin')
  const [markers, setMarkers] = useState<MapMarker[]>([])
  const [loading, setLoading] = useState(false)
  const [unsupported, setUnsupported] = useState(false)
  const [updatedLabel, setUpdatedLabel] = useState<string>('')
  const [calibrating, setCalibrating] = useState(false)
  const [calibPoints, setCalibPoints] = useState<CalibPoint[]>([])
  const [calibOverride, setCalibOverride] = useState<Record<string, Bounds>>(() => loadCalib())

  const [spawns, setSpawns] = useState<SpawnEntry[]>([])
  const loadedSpawnKey = useRef<string>('')

  const [filterOpen, setFilterOpen] = useState(false)
  // filter keyed by merged type key OR live category
  const [filter, setFilter] = useState<Record<string, boolean>>(LIVE_FILTER_DEFAULTS)

  const [teleportMode, setTeleportMode] = useState(false)
  const [teleportDest, setTeleportDest] = useState<{ x: number, y: number } | null>(null)
  const [teleportFlsId, setTeleportFlsId] = useState<string>('')
  const [allPlayers, setAllPlayers] = useState<Player[]>([])
  const [teleporting, setTeleporting] = useState(false)

  const baseCfg = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]
  const effCfg: MapCfg = useMemo(
    () => ({ ...baseCfg, ...(calibOverride[mapKey] ?? {}) }),
    [baseCfg, calibOverride, mapKey],
  )

  const load = useCallback((key: string) => {
    const cfg = MAPS.find((m) => m.key === key)
    if (!cfg?.hasLiveData) {
      setMarkers([])
      setUnsupported(false)
      setUpdatedLabel(new Date().toLocaleTimeString())
      return
    }
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setUnsupported(false)
      })
      .then(() => api.map.markers(key))
      .then((rows) => {
        setMarkers(rows)
        setUpdatedLabel(new Date().toLocaleTimeString())
      })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 404) setUnsupported(true)
        else toast.danger(t('liveMap.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
        setMarkers([])
      })
      .finally(() => setLoading(false))
  }, [t])

  const loadCurrent = useCallback(() => load(mapKey), [load, mapKey])
  useEffect(() => {
    if (isActive) {
      const id = setTimeout(loadCurrent, 0)
      return () => clearTimeout(id)
    }
  }, [isActive, loadCurrent])
  const { countdown, refresh } = useAutoRefresh(loadCurrent, POLL_MS, isActive)

  useEffect(() => {
    const cfg = MAPS.find((m) => m.key === mapKey)
    if (!cfg?.spawnFile || loadedSpawnKey.current === mapKey) return
    loadedSpawnKey.current = mapKey
    fetch(`${import.meta.env.BASE_URL}map-data/${cfg.spawnFile}-spawns.json`)
      .then((r) => r.json() as Promise<SpawnFile>)
      .then((d) => setSpawns(d.spawns))
      .catch(() => setSpawns([]))
  }, [mapKey])

  useEffect(() => {
    if (teleportMode && allPlayers.length === 0) {
      api.players.list().then(setAllPlayers).catch(() => {})
    }
  }, [teleportMode, allPlayers.length])

  const playerCount = markers.filter((m) => m.type === 'player').length
  const vehicleCount = markers.filter((m) => m.type === 'vehicle').length
  const orderedLive = useMemo(
    () => [...markers].sort((a, b) => (a.type === 'player' ? 1 : 0) - (b.type === 'player' ? 1 : 0)),
    [markers],
  )

  // A spawn is visible if filter[type_key] is true, falling back to category default
  const visibleSpawns = useMemo(
    () => spawns.filter((s) => {
      const key = filterKey(s.type)
      return filter[key] ?? false
    }),
    [spawns, filter],
  )

  const handleMapClick = useCallback((lat: number, lng: number) => {
    if (calibrating) {
      const player = markers.find((m) => m.type === 'player')
      if (!player) {
        toast.danger(t('liveMap.calibNoPlayer'))
        return
      }
      setCalibPoints((prev) => {
        const next = [...prev, { wx: player.x, wy: player.y, fracX: lng / IMG_W, fracYup: lat / IMG_H }]
        const solved = solveBounds(next)
        if (solved) {
          setCalibOverride((c) => {
            const merged = { ...c, [mapKey]: solved }
            try {
              localStorage.setItem(CALIB_LS_KEY, JSON.stringify(merged))
            }
            catch { /* quota */ }
            return merged
          })
        }
        return next
      })
      return
    }
    if (teleportMode) {
      const { x, y } = latLngToWorld(lat, lng, effCfg)
      setTeleportDest({ x: Math.round(x), y: Math.round(y) })
    }
  }, [calibrating, teleportMode, markers, mapKey, effCfg, t])

  const clearCalib = useCallback(() => {
    setCalibPoints([])
    setCalibOverride((c) => {
      const merged = { ...c }
      delete merged[mapKey]
      try {
        localStorage.setItem(CALIB_LS_KEY, JSON.stringify(merged))
      }
      catch { /* quota */ }
      return merged
    })
  }, [mapKey])

  const solvedStr = useMemo(() => {
    const b = calibOverride[mapKey]
    return b
      ? `minX: ${Math.round(b.minX)}, maxX: ${Math.round(b.maxX)}, minY: ${Math.round(b.minY)}, maxY: ${Math.round(b.maxY)}, flipY: ${!!b.flipY}`
      : ''
  }, [calibOverride, mapKey])

  const doTeleport = useCallback(async () => {
    if (!teleportDest || !teleportFlsId) return
    setTeleporting(true)
    try {
      await api.players.teleportCoords(teleportFlsId, teleportDest.x, teleportDest.y, 5000)
      toast.success(t('liveMap.teleportSent'))
      setTeleportDest(null)
    }
    catch (e) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setTeleporting(false)
    }
  }, [teleportDest, teleportFlsId, t])

  const toggleFilter = useCallback((key: string) => {
    setFilter((f) => ({ ...f, [key]: !f[key] }))
  }, [])

  const toggleCategory = useCallback((category: string, on: boolean) => {
    setFilter((f) => {
      const next = { ...f }
      // Set all types in this category
      Object.keys(TYPE_CATEGORY).forEach((type) => {
        if (TYPE_CATEGORY[type] === category) {
          next[filterKey(type)] = on
        }
      })
      return next
    })
  }, [])

  const mapCursor = calibrating || teleportMode ? 'crosshair' : 'grab'
  const currentMap = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <FilterDrawer
        open={filterOpen}
        onClose={() => setFilterOpen(false)}
        filter={filter}
        onToggle={toggleFilter}
        onToggleCategory={toggleCategory}
        spawns={spawns}
      />

      <PageHeader title={t('liveMap.title')} subtitle={t('liveMap.subtitle')}>
        <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  {isActive && currentMap.hasLiveData && (
                    <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                      {countdown}
                      s
                    </span>
                  )}
                  <Icon name="refresh-cw" />
                </>
              )}
        </Button>
      </PageHeader>

      <div className="shrink-0 flex items-start gap-2 rounded-[var(--radius)] border border-border bg-surface px-3 py-2 text-xs">
        <Icon name="flask-conical" className="size-4 shrink-0 mt-0.5 text-accent" />
        <div>
          <span className="font-medium text-accent">{t('liveMap.betaTitle')}</span>
          {' '}
          <span className="text-muted">{t('liveMap.betaBody')}</span>
        </div>
      </div>

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 shrink-0">
        {MAPS.map((m) => (
          <Button
            key={m.key}
            size="sm"
            variant={m.key === mapKey ? 'primary' : 'outline'}
            onPress={() => {
              loadedSpawnKey.current = ''
              setMapKey(m.key)
              setSpawns([])
              setTeleportDest(null)
              setCalibrating(false)
            }}
          >
            {m.label}
          </Button>
        ))}
        <div className="h-4 border-l border-border mx-1" />
        <Button size="sm" variant={filterOpen ? 'primary' : 'outline'} onPress={() => setFilterOpen((v) => !v)}>
          <Icon name="layers" />
          {' '}
          {t('liveMap.filter')}
        </Button>
        <Button
          size="sm"
          variant={teleportMode ? 'primary' : 'outline'}
          onPress={() => {
            setTeleportMode((v) => !v)
            setTeleportDest(null)
          }}
        >
          <Icon name="navigation" />
          {' '}
          {t('liveMap.teleportMode')}
        </Button>
        <Button size="sm" variant={calibrating ? 'primary' : 'outline'} onPress={() => setCalibrating((v) => !v)}>
          <Icon name="crosshair" />
          {' '}
          {t('liveMap.calibrate')}
        </Button>
        {calibrating && (
          <Button size="sm" variant="outline" onPress={clearCalib}>{t('liveMap.clear')}</Button>
        )}
      </div>

      {/* Stats bar */}
      <div className="flex flex-wrap gap-4 shrink-0 text-xs text-muted">
        {currentMap.hasLiveData && (
          <>
            <span>
              <span style={{ color: CAT_COLOR.player }}>●</span>
              {' '}
              {t('liveMap.players')}
              {': '}
              {playerCount}
            </span>
            <span>
              <span style={{ color: CAT_COLOR.vehicle }}>●</span>
              {' '}
              {t('liveMap.vehicles')}
              {': '}
              {vehicleCount}
            </span>
            <span>
              {t('liveMap.total')}
              {': '}
              {markers.length}
            </span>
          </>
        )}
        {spawns.length > 0 && <span>{t('liveMap.spawnsLoaded', { count: spawns.length })}</span>}
        {updatedLabel !== '' && <span className="ml-auto">{t('liveMap.updated', { time: updatedLabel })}</span>}
      </div>

      {/* Teleport panel */}
      {teleportMode && (
        <div className="shrink-0 rounded-[var(--radius)] border border-accent/40 bg-surface px-3 py-2 text-xs flex flex-wrap items-center gap-3">
          <div className="text-accent font-medium">
            <Icon name="navigation" className="size-3 inline mr-1" />
            {teleportDest
              ? t('liveMap.spawnTooltipCoords', { x: teleportDest.x, y: teleportDest.y })
              : t('liveMap.teleportModeActive')}
          </div>
          {teleportDest && (
            <>
              <Select
                aria-label={t('liveMap.teleportPlayer')}
                placeholder={t('liveMap.teleportSelectPlayer')}
                selectedKey={teleportFlsId || null}
                onSelectionChange={(k) => setTeleportFlsId(k ? String(k) : '')}
                className="w-56"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {allPlayers.map((p) => (
                      <ListBox.Item key={p.fls_id} id={p.fls_id} textValue={p.name}>
                        {p.name}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Button size="sm" isDisabled={!teleportFlsId || teleporting} onPress={doTeleport}>
                {teleporting ? <Spinner size="sm" color="current" /> : t('liveMap.teleportHere')}
              </Button>
              <Button size="sm" variant="ghost" onPress={() => setTeleportDest(null)}>✕</Button>
            </>
          )}
        </div>
      )}

      {/* Calibration hint */}
      {calibrating && (
        <div className="shrink-0 rounded-[var(--radius)] border border-border bg-surface px-3 py-2 text-xs">
          <div className="text-accent">{t('liveMap.calibActive')}</div>
          <div className="text-muted">{t('liveMap.calibPoints', { n: calibPoints.length })}</div>
          {solvedStr && <div className="mt-1 font-mono text-foreground break-all">{solvedStr}</div>}
        </div>
      )}

      {unsupported
        ? <div className="py-8 text-center text-sm text-muted">{t('liveMap.unsupported')}</div>
        : (
            <div className="relative flex-1 min-h-0 overflow-hidden rounded-[var(--radius)] border border-border">
              <MapContainer
                crs={CRS.Simple}
                bounds={IMAGE_BOUNDS}
                minZoom={-3}
                maxZoom={4}
                zoomSnap={0.25}
                attributionControl={false}
                style={{ height: '100%', width: '100%', background: 'var(--color-surface)', cursor: mapCursor }}
              >
                <InvalidateOnActive active={isActive} />
                <MapClickCapture active={calibrating || teleportMode} onPick={handleMapClick} />
                {effCfg.image && (
                  <ImageOverlay
                    key={mapKey}
                    url={`${import.meta.env.BASE_URL}${effCfg.image}`}
                    bounds={IMAGE_BOUNDS}
                  />
                )}

                {/* Static spawn markers — sprite icon when available, circle otherwise */}
                {visibleSpawns.map((s, i) => {
                  const center = worldToLatLng(s.x, s.y, effCfg)
                  // Dense categories get a smaller icon; sparse get the standard 22px.
                  const isDense = s.category === 'resources' || s.category === 'static'
                  const iconSize = isDense ? 16 : 22
                  const divIcon = makeSpriteDivIcon(filterKey(s.type), iconSize)
                  const tooltip = (
                    <Tooltip>
                      <div className="font-medium">{s.label ?? TYPE_LABELS[s.type] ?? s.type}</div>
                      <div className="text-xs opacity-70">{s.category}</div>
                      <div className="text-xs">
                        {Math.round(s.x)}
                        {', '}
                        {Math.round(s.y)}
                        {s.z != null ? `, ${Math.round(s.z)}` : ''}
                      </div>
                    </Tooltip>
                  )
                  const handlers = teleportMode
                    ? { click: () => setTeleportDest({ x: Math.round(s.x), y: Math.round(s.y) }) }
                    : undefined
                  if (divIcon) {
                    return (
                      <Marker
                        key={`spawn-${i}`}
                        position={center}
                        icon={divIcon}
                        eventHandlers={handlers}
                      >
                        {tooltip}
                      </Marker>
                    )
                  }
                  return (
                    <CircleMarker
                      key={`spawn-${i}`}
                      center={center}
                      radius={spawnRadius(s.category)}
                      pathOptions={{
                        color: 'transparent',
                        weight: 0,
                        fillColor: CAT_COLOR[s.category] ?? '#888',
                        fillOpacity: 0.65,
                      }}
                      eventHandlers={handlers}
                    >
                      {tooltip}
                    </CircleMarker>
                  )
                })}

                {/* Live markers */}
                {(filter.players || filter.vehicles) && orderedLive
                  .filter((m) => m.type === 'player' ? filter.players : filter.vehicles)
                  .map((m) => {
                    const [lat, lng] = worldToLatLng(m.x, m.y, effCfg)
                    const isPlayer = m.type === 'player'
                    return (
                      <CircleMarker
                        key={`${m.type}-${m.id}`}
                        center={[lat, lng]}
                        radius={isPlayer ? 7 : 5}
                        pathOptions={{
                          color: '#0b0b0b',
                          weight: 1.5,
                          fillColor: CAT_COLOR[m.type] ?? CAT_COLOR.base,
                          fillOpacity: 1,
                        }}
                        eventHandlers={teleportMode && isPlayer && m.fls_id
                          ? { click: () => setTeleportFlsId(m.fls_id!) }
                          : undefined}
                      >
                        <Tooltip>
                          <div className="font-medium">{m.name || `${m.type} ${m.id}`}</div>
                          <div>
                            {m.type}
                            {m.online_status ? ` · ${m.online_status}` : ''}
                          </div>
                          <div>
                            {Math.round(m.x)}
                            ,
                            {' '}
                            {Math.round(m.y)}
                            ,
                            {' '}
                            {Math.round(m.z)}
                          </div>
                        </Tooltip>
                      </CircleMarker>
                    )
                  })}

                {/* Teleport destination */}
                {teleportDest && (
                  <CircleMarker
                    center={worldToLatLng(teleportDest.x, teleportDest.y, effCfg)}
                    radius={10}
                    pathOptions={{ color: '#ffffff', weight: 2, fillColor: '#f59e0b', fillOpacity: 0.85 }}
                  >
                    <Tooltip permanent>
                      <span className="text-xs">
                        {teleportDest.x}
                        ,
                        {' '}
                        {teleportDest.y}
                      </span>
                    </Tooltip>
                  </CircleMarker>
                )}

                {calibrating && calibPoints.map((p, i) => (
                  <CircleMarker
                    key={`calib-${i}`}
                    center={[p.fracYup * IMG_H, p.fracX * IMG_W]}
                    radius={5}
                    pathOptions={{ color: '#ffffff', weight: 2, fillColor: '#ff2bd6', fillOpacity: 0.9 }}
                  >
                    <Tooltip>{`calib ${i + 1}`}</Tooltip>
                  </CircleMarker>
                ))}
              </MapContainer>
            </div>
          )}
    </div>
  )
}
