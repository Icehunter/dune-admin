// ADVANCED_CATEGORIES are the gameplay categories operators actually tune. They
// show by default; every other category (engine/system internals) is treated as
// Expert and hidden behind a toggle. Names are the exact category strings the
// backend emits (curated category names + discovered INI short-section names).
const ADVANCED_CATEGORIES = new Set([
  'Multipliers', 'World & Combat', 'Persistence & Building', 'Server Identity',
  // Server customization (UserServerCustomSettings.ini, #336).
  'Crafting & Harvesting', 'Combat & Progression', 'Survival', 'Building & Landsraad',
  'SandwormSettings', 'SandStormConfig', 'CoriolisSubsystem', 'TaxationSettings',
  'CraftingSettings', 'SpiceHarvestingSystem', 'DewHarvestSettings',
  'SecurityZonesSubsystem', 'GuildSettings', 'DuneVehicleSettings',
  'RespawnSettings', 'ShelterSettings',
  'BuildingSettings', 'InventorySystemSettings', 'DuneGameMode', 'DuneExchangeSettings',
  'SpiceAddictionSubsystem', 'HydrationSubsystem', 'HazardsSettings', 'URL', 'Global',
])

// Display order for categories; any not listed (Expert) sort after, alphabetically.
const CATEGORY_ORDER = [
  'Multipliers', 'World & Combat', 'Persistence & Building', 'Server Identity',
  // Server customization (UserServerCustomSettings.ini, #336).
  'Crafting & Harvesting', 'Combat & Progression', 'Survival', 'Building & Landsraad',
  'SandwormSettings', 'SandStormConfig', 'CoriolisSubsystem', 'TaxationSettings',
  'CraftingSettings', 'SpiceHarvestingSystem', 'DewHarvestSettings',
  'SecurityZonesSubsystem', 'GuildSettings', 'DuneVehicleSettings',
  'RespawnSettings', 'ShelterSettings',
  'BuildingSettings', 'InventorySystemSettings', 'DuneGameMode', 'DuneExchangeSettings',
  'SpiceAddictionSubsystem', 'HydrationSubsystem', 'HazardsSettings', 'URL', 'Global',
]

// Friendlier display names for the raw INI short-section category strings.
const CATEGORY_LABELS: Record<string, string> = {
  SandwormSettings: 'Sandworm',
  SandStormConfig: 'Sandstorm',
  CoriolisSubsystem: 'Coriolis Storm',
  TaxationSettings: 'Taxation',
  CraftingSettings: 'Crafting',
  SpiceHarvestingSystem: 'Spice Harvesting',
  DewHarvestSettings: 'Dew Harvesting',
  SecurityZonesSubsystem: 'Security Zones',
  GuildSettings: 'Guilds',
  DuneVehicleSettings: 'Vehicles',
  RespawnSettings: 'Respawn',
  ShelterSettings: 'Shelter',
  BuildingSettings: 'Building',
  InventorySystemSettings: 'Inventory',
  DuneGameMode: 'Game Mode',
  DuneExchangeSettings: 'Exchange / Market',
  SpiceAddictionSubsystem: 'Spice Addiction',
  HydrationSubsystem: 'Hydration',
  HazardsSettings: 'Hazards',
}

const CATEGORY_ICONS: Record<string, string> = {
  'Multipliers': 'sliders',
  'World & Combat': 'swords',
  'Persistence & Building': 'hammer',
  'Server Identity': 'tag',
  'Crafting & Harvesting': 'pickaxe',
  'Combat & Progression': 'trending-up',
  'Survival': 'heart-pulse',
  'Building & Landsraad': 'landmark',
  'SandwormSettings': 'worm',
  'SandStormConfig': 'wind',
  'CoriolisSubsystem': 'tornado',
  'TaxationSettings': 'receipt',
  'CraftingSettings': 'anvil',
  'SpiceHarvestingSystem': 'sparkles',
  'DewHarvestSettings': 'droplet',
  'SecurityZonesSubsystem': 'shield',
  'GuildSettings': 'users',
  'DuneVehicleSettings': 'car',
  'RespawnSettings': 'rotate-ccw',
  'ShelterSettings': 'tent',
  'BuildingSettings': 'blocks',
  'InventorySystemSettings': 'package',
  'DuneGameMode': 'gamepad-2',
  'DuneExchangeSettings': 'store',
  'SpiceAddictionSubsystem': 'pill',
  'HydrationSubsystem': 'glass-water',
  'HazardsSettings': 'triangle-alert',
  'URL': 'link',
  'Global': 'globe',
}

// Frequently-tuned settings surfaced in the "Common" panel above the categories.
// Keys are the validated CVar / UPROPERTY names from the reworked schema.
const COMMON_KEYS = new Set([
  'ConsoleVariables|Dune.GlobalMiningOutputMultiplier',
  'ConsoleVariables|Dune.GlobalVehicleMiningOutputMultiplier',
  'ConsoleVariables|SecurityZones.PvpResourceMultiplier',
  '/Script/DuneSandbox.SecurityZonesSubsystem|m_bAreSecurityZonesEnabled',
  '/Script/DuneSandbox.PvpPveSettings|m_bShouldForceEnablePvpOnAllPartitions',
  'ConsoleVariables|Sandstorm.Enabled',
  'ConsoleVariables|sandworm.dune.Enabled',
  '/DeteriorationSystem.ItemDeteriorationConstants|UpdateRateInSeconds',
  '/Script/DuneSandbox.BuildingSettings|m_MaxNumLandclaimSegments',
  'ConsoleVariables|Bgd.ServerDisplayName',
  'ConsoleVariables|Bgd.ServerLoginPassword',
  '/Script/DuneSandbox.SandStormConfig|m_bCoriolisAutoSpawnEnabled',
])

const SOURCE_FILE: Record<string, string> = {
  defaultGame: 'DefaultGame.ini',
  defaultEngine: 'DefaultEngine.ini',
  userGame: 'UserGame.ini',
  userEngine: 'UserEngine.ini',
  userCustom: 'UserServerCustomSettings.ini',
  userGameOverrides: 'UserOverrides.ini',
  // AMP-managed curated settings: read back from AMP's live config (#173).
  amp: 'AMP config',
}

const LAYER_STYLE: Record<string, { cls: string }> = {
  defaultGame: { cls: 'text-muted/60' },
  defaultEngine: { cls: 'text-muted/60' },
  userEngine: { cls: 'text-foreground/70' },
  userGame: { cls: 'text-foreground/70' },
  userCustom: { cls: 'text-foreground/70' },
  userGameOverrides: { cls: 'text-warning' },
  amp: { cls: 'text-warning' },
}

const SOURCE_PRIORITY = ['defaultGame', 'defaultEngine', 'userEngine', 'userGame', 'userCustom', 'userGameOverrides', 'amp'] as const

// userSourcesFor returns the set of layer sources that represent a genuine
// operator override for the active control plane (#262 sub-bug B).
//
// On AMP, UserGame.ini/UserEngine.ini are regenerated by AMP itself on every
// start — only "userGameOverrides" (the separate file dune-admin actually
// writes) and "amp" (curated settings written through the AMP API) are real
// operator overrides there (#212). On every other control plane (kubectl,
// local, docker, or unknown), dune-admin writes settings — including custom,
// non-schema keys — directly to UserGame.ini/UserEngine.ini, so those sources
// ARE the operator overrides and must stay visible by default.
const userSourcesFor = (control: string): Set<string> =>
  control === 'amp'
    ? new Set(['userGameOverrides', 'amp'])
    : new Set(['userGame', 'userEngine', 'userCustom', 'userGameOverrides', 'amp'])

export {
  CATEGORY_ORDER, CATEGORY_ICONS, CATEGORY_LABELS, ADVANCED_CATEGORIES, COMMON_KEYS,
  SOURCE_FILE, LAYER_STYLE, SOURCE_PRIORITY, userSourcesFor,
}
