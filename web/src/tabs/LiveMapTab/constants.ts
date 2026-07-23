import type { LatLngBoundsExpression } from 'leaflet'
import type { MapCfg } from './types'
import { mapUrl } from './utils'

const MAP_BASE = ((import.meta.env.VITE_CDN_BASE_URL as string) ?? 'https://assets.dune.layout.tools').replace(/\/$/, '')

const TILE_CDN = 'https://cdn.th.gl/dune-awakening/map-tiles'

const IMG_W = 4096
const IMG_H = 4096
const IMAGE_BOUNDS: LatLngBoundsExpression = [[0, 0], [IMG_H, IMG_W]]
const POLL_MS = 30000

// Minimum separation, in world units (Unreal cm), between two calibration
// anchors. Points closer than this produce a near-singular least-squares fit,
// so the UI rejects them and warns the operator to move further away.
const CALIB_MIN_WORLD_DIST = 50000

const SPRITE_URL = mapUrl('map-data/map-icons.webp')
const SPRITE_COLS = 11
const SPRITE_ROWS = 12
const SPRITE_CELL = 64

const ICON_POS: Record<string, [number, number]> = {
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
  buggy: [2, 3], ebuggy: [2, 3],
  bike: [10, 2],
  bene_gesserit_trainer: [1, 4],
  mentat: [9, 4],
  planetologist: [8, 2],
  swordmaster: [1, 5],
  trooper: [10, 1],
  blue_id_band: [6, 2],
  green_id_band: [0, 1],
  orange_id_band: [5, 2],
  purple_id_band: [10, 0],
  red_id_band: [5, 1],
  spice_field_small: [10, 7],
  spice_field_medium: [0, 8],
  spice_field_large: [1, 8],
  agave_seeds: [1, 9],
  azurite: [8, 8], azurite_pickup: [8, 8],
  basalt: [5, 8], basalt_pickup: [5, 8],
  bauxite: [7, 8], bauxite_pickup: [7, 8],
  dolomite: [10, 8], dolomite_pickup: [10, 8],
  erythrite: [0, 9], erythrite_pickup: [0, 9],
  fiber_plant: [6, 8], plant_fiber: [6, 8],
  fuel_cells: [5, 9],
  jasmium: [3, 9], jasmium_crystal: [3, 9],
  magnetite: [2, 9], magnetite_pickup: [2, 9],
  primrose_field: [9, 7],
  rhyolite: [9, 8], rhyolite_pickup: [9, 8],
  scrap_electronics: [7, 9],
  scrap_metal: [6, 9],
  stravidium: [4, 8],
  titanium_ore: [3, 8],
  barkeep: [0, 7],
  base_vendor: [6, 6],
  landsraad_vendor: [2, 7],
  scrap_trader: [9, 6],
  spice_merchant: [7, 6],
  vehicle_vendor: [5, 6],
  water_seller: [8, 6],
  weapons_merchant: [1, 7],
  banker: [10, 6],
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
  alexin: [1, 6], argosaz: [0, 2], dyvetz: [7, 4], ecaz: [10, 4],
  hagal: [4, 4], hurata: [9, 3], imota: [5, 5], kenola: [3, 3],
  lindaren: [5, 4], maros: [8, 7], mikarrol: [4, 5], moritani: [6, 4],
  mutelli: [4, 7], novebruns: [8, 3], richese: [2, 4], sor: [2, 5],
  spinette: [2, 6], taligari: [0, 3], thorvald: [0, 5], tseida: [7, 3],
  varota: [3, 7], vernius: [0, 4], wallach: [5, 7], wayku: [7, 7], wydras: [8, 1],
  aluminum_ore: [7, 8],
  copper_ore: [8, 8],
  carbon_fiber: [10, 8],
  iron_ore: [2, 9],
  stone: [9, 8],
  fiber: [6, 8],
  cistanche: [1, 9],
  saguaro_cactus: [1, 9],
  t6_resource_a: [4, 8],
  t6_resource_b: [3, 8],
  sandworm_territory: [4, 0],
  enemycamp: [4, 0], enemyoutpost: [6, 1], enemylaboroutpost: [10, 3],
  wreck: [2, 2], tradingpost: [9, 1], sietch: [0, 2], ecolab: [7, 2],
  small_shipwreck: [7, 0], atreides: [3, 4], harkonnen: [8, 4], poi: [6, 7],
  npc_harkonnen: [8, 5], npc_atreides: [3, 5], npc_bandits: [3, 5],
  npc_unaffiliated: [7, 1], npc_choam: [7, 5], npc_fremen: [0, 6],
  npc_sardaukar: [9, 5], npc_smugglers: [6, 5], npc_spacingguild: [8, 5],
  trainersswordmaster: [1, 5], trainersmentat: [9, 4], trainersbenegesserit: [1, 4],
  trainersplanetologist: [8, 2], trainerstrooper: [10, 1],
  sandbike: [10, 2],
}

const CAT_COLOR: Record<string, string> = {
  player: '#3b9dff', vehicle: '#5fd35a', base: '#e0a13a',
  resources: '#f5a623', locations: '#9b59b6', npcs: '#e74c3c',
  vendors: '#2ecc71', landsraad: '#e91e8c', static: '#7f8c8d',
  hazards: '#ff5020',
}

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
  aluminum_ore: 'Aluminum Ore', copper_ore: 'Copper Ore', carbon_fiber: 'Carbon Fiber',
  iron_ore: 'Iron Ore', stone: 'Stone', fiber: 'Plant Fiber',
  cistanche: 'Cistanche', saguaro_cactus: 'Saguaro Cactus',
  t6_resource_a: 'T6 Resource A', t6_resource_b: 'T6 Resource B',
  sandworm_territory: 'Sandworm Territory', buried_treasure: 'Buried Treasure',
  static: 'Static Object',
  enemycamp: 'Enemy Camp', enemyoutpost: 'Enemy Outpost', enemylaboroutpost: 'Enemy Lab Outpost',
  cave: 'Cave', wreck: 'Wreck', tradingpost: 'Trading Post', sietch: 'Sietch',
  ecolab: 'Eco Lab', secret_door: 'Secret Door', shipwreck: 'Shipwreck',
  small_shipwreck: 'Small Shipwreck', atreides: 'Atreides', harkonnen: 'Harkonnen', poi: 'Point of Interest',
  npc_harkonnen: 'Harkonnen NPC', npc_atreides: 'Atreides NPC', npc_bandits: 'Bandits',
  npc_unaffiliated: 'Unaffiliated', npc_choam: 'CHOAM', npc_fremen: 'Fremen',
  npc_sardaukar: 'Sardaukar', npc_smugglers: 'Smugglers', npc_spacingguild: 'Spacing Guild',
  trainersswordmaster: 'Swordmaster', trainersmentat: 'Mentat', trainersbenegesserit: 'Bene Gesserit',
  trainersplanetologist: 'Planetologist', trainerstrooper: 'Trooper',
  purple_id_band: 'Purple ID Band', green_id_band: 'Green ID Band',
  red_id_band: 'Red ID Band', orange_id_band: 'Orange ID Band', blue_id_band: 'Blue ID Band',
  sandbike: 'Sandbike',
}

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

const MAPS: MapCfg[] = [
  {
    key: 'HaggaBasin', label: 'Hagga Basin', image: 'hagga-basin.webp', spawnFile: 'hagga',
    tileId: 'survival_1-0c70ddebb3e41cf49915b22e103e94ed',
    depthFile: 'hagga-depth.webp',
    hasLiveData: true,
    minX: -437871, maxX: 350539, minY: -462011, maxY: 376267, flipY: true,
  },
  {
    key: 'DeepDesert', label: 'Deep Desert', image: 'deepdesert.webp', spawnFile: 'deepdesert',
    tileId: 'deepdesert_1-40f176fc4cce018dff08f3cd66b52f08',
    depthFile: 'deepdesert-depth.webp',
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

const HEATMAP_BOUNDS: Record<string, { minX: number, maxX: number, minY: number, maxY: number }> = {
  HaggaBasin: { minX: -457200, maxX: 355600, minY: -457200, maxY: 355600 },
  DeepDesert: { minX: -1270000, maxX: 1168400, minY: -1270000, maxY: 1168400 },
}

const HEATMAP_PREFIX: Record<string, string> = {
  HaggaBasin: 'hagga',
  DeepDesert: 'deepdesert',
}

const HEATMAP_TO_FILTER: Record<string, string> = {
  aluminum_ore: 'bauxite',
  copper_ore: 'azurite',
  carbon_fiber: 'dolomite',
  iron_ore: 'magnetite',
  stone: 'rhyolite',
  fiber: 'fiber_plant',
}

const HEATMAP_COLORS: Record<string, string> = {
  aluminum_ore: 'rgb(201,130,10)', copper_ore: 'rgb(184,115,51)',
  carbon_fiber: 'rgb(90,90,90)', iron_ore: 'rgb(130,130,145)',
  stone: 'rgb(160,145,120)', basalt: 'rgb(150,100,50)',
  scrap_metal: 'rgb(100,120,145)', fuel: 'rgb(255,200,50)',
  fiber: 'rgb(120,200,80)', cistanche: 'rgb(60,180,120)',
  saguaro_cactus: 'rgb(40,160,80)', primrose_field: 'rgb(200,200,60)',
  jasmium: 'rgb(180,100,220)', erythrite: 'rgb(220,60,60)',
  t6_resource_a: 'rgb(100,220,220)', t6_resource_b: 'rgb(60,180,220)',
  sandworm_territory: 'rgb(255,80,30)',
  titanium_ore: 'rgb(180,180,190)',
  spice_field_small: 'rgb(230,150,60)', spice_field_medium: 'rgb(220,120,40)',
  spice_field_large: 'rgb(200,90,20)',
}

const DD_ROWS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I']
const DD_COLS = [1, 2, 3, 4, 5, 6, 7, 8, 9]

const HEATMAP_TYPES: Record<string, string[]> = {
  HaggaBasin: [
    'aluminum_ore', 'basalt', 'carbon_fiber', 'cistanche', 'copper_ore',
    'erythrite', 'fiber', 'fuel', 'iron_ore', 'jasmium',
    'primrose_field', 'saguaro_cactus', 'sandworm_territory', 'scrap_metal', 'stone',
  ],
  DeepDesert: [
    'aluminum_ore', 'basalt', 'carbon_fiber', 'copper_ore',
    'fiber', 'fuel', 'iron_ore',
    'sandworm_territory', 'scrap_metal', 'stone', 't6_resource_a', 't6_resource_b',
    'titanium_ore', 'spice_field_small', 'spice_field_medium', 'spice_field_large',
  ],
}

const LIVE_TYPES = ['players', 'vehicles', 'bases'] as const

const CATEGORY_GROUPS: { id: string, labelKey: string }[] = [
  { id: 'locations', labelKey: 'liveMap.filterLocations' },
  { id: 'resources', labelKey: 'liveMap.filterResources' },
  { id: 'npcs', labelKey: 'liveMap.filterNPCs' },
  { id: 'vendors', labelKey: 'liveMap.filterVendors' },
  { id: 'trainers', labelKey: 'liveMap.filterTrainers' },
  { id: 'landsraad', labelKey: 'liveMap.filterLandsraad' },
  { id: 'pentashield_keys', labelKey: 'liveMap.filterKeys' },
  { id: 'vehicles', labelKey: 'liveMap.vehicles' },
  { id: 'static', labelKey: 'liveMap.filterStaticObjects' },
  { id: 'hazards', labelKey: 'liveMap.filterHazards' },
]

export {
  MAP_BASE,
  TILE_CDN,
  IMG_W,
  IMG_H,
  IMAGE_BOUNDS,
  POLL_MS,
  CALIB_MIN_WORLD_DIST,
  SPRITE_URL,
  SPRITE_COLS,
  SPRITE_ROWS,
  SPRITE_CELL,
  ICON_POS,
  CAT_COLOR,
  TYPE_LABELS,
  TYPE_MERGE_KEY,
  MAPS,
  CALIB_LS_KEY,
  HEATMAP_BOUNDS,
  HEATMAP_PREFIX,
  HEATMAP_TO_FILTER,
  HEATMAP_COLORS,
  DD_ROWS,
  DD_COLS,
  HEATMAP_TYPES,
  LIVE_TYPES,
  CATEGORY_GROUPS,
}
