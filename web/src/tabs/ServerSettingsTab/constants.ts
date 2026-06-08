const CATEGORY_ORDER = [
  'Multipliers', 'World & Combat', 'Persistence & Building', 'Server Identity',
]

const CATEGORY_ICONS: Record<string, string> = {
  'Multipliers': 'sliders',
  'World & Combat': 'swords',
  'Persistence & Building': 'hammer',
  'Server Identity': 'tag',
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
