const CATEGORY_ORDER = [
  'Survival', 'Progression', 'Harvesting', 'Building', 'Inventory',
  'Guilds & Economy', 'Storm Cycle', 'PvP & Security', 'Spice', 'Taxation', 'Sandworm',
]

const CATEGORY_ICONS: Record<string, string> = {
  'Survival': 'heart-pulse',
  'Progression': 'trending-up',
  'Harvesting': 'pickaxe',
  'Building': 'home',
  'Inventory': 'package',
  'Guilds & Economy': 'coins',
  'Storm Cycle': 'wind',
  'PvP & Security': 'shield',
  'Spice': 'sparkles',
  'Taxation': 'receipt',
  'Sandworm': 'worm',
}

const COMMON_KEYS = new Set([
  '/Script/DuneSandbox.DuneGameMode|m_GlobalXPMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalHealthMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalDamageToNpcsMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalDamageToPlayersMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_GlobalHarvestAmountMultiplier',
  '/Script/DuneSandbox.DuneGameMode|m_WaterConsumptionRate',
  '/Script/DuneSandbox.PvpPveSettings|bPvPEnabled',
  '/Script/DuneSandbox.PvpPveSettings|bServerPVE',
  '/Script/DuneSandbox.SandStormConfig|m_StormCycleDuration',
  '/Script/DuneSandbox.InventorySystemSettings|PlayerInventoryStartingSize',
  '/Script/DuneSandbox.BuildingSettings|m_MaxNumLandclaimSegments',
  '/Script/DuneSandbox.GuildSettings|m_MaxGuildMembersAllowed',
  '/DeteriorationSystem.ItemDeteriorationConstants|m_ItemDurabilityLossMultiplier',
  '/Script/DuneSandbox.SpiceHarvestingSystem|m_bSpawningActive',
])

const SOURCE_FILE: Record<string, string> = {
  defaultGame: 'DefaultGame.ini',
  defaultEngine: 'DefaultEngine.ini',
  userGame: 'UserGame.ini',
  userEngine: 'UserEngine.ini',
  userGameOverrides: 'UserOverrides.ini',
}

const LAYER_STYLE: Record<string, { cls: string }> = {
  defaultGame: { cls: 'text-muted/60' },
  defaultEngine: { cls: 'text-muted/60' },
  userEngine: { cls: 'text-foreground/70' },
  userGame: { cls: 'text-foreground/70' },
  userGameOverrides: { cls: 'text-warning' },
}

const SOURCE_PRIORITY = ['defaultGame', 'defaultEngine', 'userEngine', 'userGame', 'userGameOverrides'] as const

const USER_SOURCES = new Set(['userGame', 'userEngine', 'userGameOverrides'])

export { CATEGORY_ORDER, CATEGORY_ICONS, COMMON_KEYS, SOURCE_FILE, LAYER_STYLE, SOURCE_PRIORITY, USER_SOURCES }
