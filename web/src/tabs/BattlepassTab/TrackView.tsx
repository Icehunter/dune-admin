import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Chip, Tooltip } from '@heroui/react'
import type { BattlepassTier, BattlepassTierCounts } from '../../api/client'
import { Icon } from '../../dune-ui'
import { RewardIcon } from './RewardIcons'

const CATEGORY_ORDER = ['level', 'story', 'side_quest', 'faction', 'exploration', 'achievement']

const CATEGORY_ICONS: Record<string, string> = {
  level: 'chevrons-up',
  story: 'book-open',
  side_quest: 'map',
  faction: 'landmark',
  exploration: 'compass',
  achievement: 'trophy',
}

// Theme id (data-theme) → asset folder under /theme. Falls back to spice.
const THEME_FOLDERS: Record<string, string> = {
  spice: 'spice',
  harkonnen: 'harkonnen',
  fremen: 'fremen',
  atreides: 'atredies',
}

// Category → card asset basename (one SVG per theme folder).
const CARD_FILES: Record<string, string> = {
  level: 'level_card',
  story: 'story_card',
  side_quest: 'sidequest_card',
  faction: 'faction_card',
  exploration: 'exploration_card',
  achievement: 'achievement_card',
}

// The card SVGs are pre-normalized: cropped to the art and centered on a
// uniform 1500×2500 canvas (see the slicing notes in the PR), so they render
// as plain images with a fixed 3:5 aspect — no per-card insets needed.
const CARD_ASPECT = '3 / 5'

// Vertical center of each theme's baked panel box (% of card height) — the
// harkonnen frame puts its panel a touch higher than the other themes.
const PANEL_CENTER: Record<string, string> = {
  harkonnen: '77%',
}
const PANEL_CENTER_DEFAULT = '81%'

// useThemeFolder tracks data-theme on <html> so the cards swap with the theme.
const useThemeFolder = (): string => {
  const read = () => THEME_FOLDERS[document.documentElement.getAttribute('data-theme') ?? ''] ?? 'spice'
  const [folder, setFolder] = React.useState(read)
  React.useEffect(() => {
    const obs = new MutationObserver(() => setFolder(read()))
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    return () => obs.disconnect()
  }, [])
  return folder
}

export interface TrackViewProps {
  tiers: BattlepassTier[]
  counts: Record<string, BattlepassTierCounts>
  playerCount: number
  categoryLabel: (cat: string) => string
}

const tierAchieved = (counts: Record<string, BattlepassTierCounts>, tier: BattlepassTier): number => {
  const c = counts[tier.tier_key]
  return c ? c.baseline + c.earned + c.granted : 0
}

const itemCount = (tier: BattlepassTier): number => {
  if (!tier.reward_items) return 0
  try {
    return (JSON.parse(tier.reward_items) as unknown[]).length
  }
  catch {
    return 0
  }
}

/** Pre-normalized theme card art (1500×2500, edge-to-edge). */
const CardArt: React.FC<{ folder: string, file: string }> = ({ folder, file }) => (
  <img
    src={`/theme/${folder}/${file}.svg`}
    alt=""
    draggable={false}
    className="absolute inset-0 w-full h-full select-none object-contain"
  />
)

/** Battlepass track: themed category cards up top (Level selected by
 *  default); picking one shows that category's rewards as a timeline. */
export const TrackView: React.FC<TrackViewProps> = ({ tiers, counts, playerCount, categoryLabel }) => {
  const { t } = useTranslation()
  const folder = useThemeFolder()
  const [selected, setSelected] = React.useState('level')

  const byCategory = React.useMemo(() => {
    const m = new Map<string, BattlepassTier[]>()
    for (const tier of tiers) {
      if (!tier.enabled) continue
      const lane = m.get(tier.category) ?? []
      lane.push(tier)
      m.set(tier.category, lane)
    }
    for (const lane of m.values()) {
      lane.sort((a, b) => (a.threshold - b.threshold) || (a.id - b.id))
    }
    return m
  }, [tiers])

  const categories = CATEGORY_ORDER.filter((c) => byCategory.has(c))
  const lane = byCategory.get(selected) ?? []

  const catStats = (cat: string) => {
    const ts = byCategory.get(cat) ?? []
    const intel = ts.reduce((s, x) => s + x.intel, 0)
    const items = ts.reduce((s, x) => s + itemCount(x), 0)
    let pct = 0
    if (playerCount > 0 && ts.length > 0) {
      const sum = ts.reduce((s, x) => s + Math.min(tierAchieved(counts, x), playerCount), 0)
      pct = Math.round((sum / (ts.length * playerCount)) * 100)
    }
    return { count: ts.length, intel, items, pct }
  }

  const pctColor = (pct: number): 'success' | 'accent' | 'default' => {
    if (pct >= 75) return 'success'
    if (pct >= 25) return 'accent'
    return 'default'
  }

  if (categories.length === 0) {
    return <div className="text-sm text-muted">{t('battlepass.track.empty')}</div>
  }

  return (
    <div className="flex flex-col gap-6 min-h-0">
      {/* Category cards */}
      <div className="grid gap-4 shrink-0" style={{ gridTemplateColumns: `repeat(${categories.length}, minmax(0, 1fr))` }}>
        {categories.map((cat) => {
          const stats = catStats(cat)
          const file = CARD_FILES[cat]
          const active = selected === cat
          return (
            <button
              key={cat}
              type="button"
              onClick={() => setSelected(cat)}
              aria-pressed={active}
              className={`flex flex-col items-center gap-2 text-center transition-transform duration-150 cursor-pointer
                ${active ? 'scale-[1.02]' : 'opacity-75 hover:opacity-100 hover:scale-[1.01]'}`}
            >
              {/* Category name above the card */}
              <div
                className={`uppercase tracking-widest font-semibold ${active ? 'text-accent' : 'text-foreground'}`}
                style={{ fontSize: 'clamp(0.65rem, 1.05vw, 1rem)' }}
              >
                {categoryLabel(cat)}
              </div>

              {/* Card art with the completion % centered in its panel box */}
              <div
                className="relative w-full rounded-[var(--radius)]"
                style={{
                  aspectRatio: CARD_ASPECT,
                  filter: active
                    ? 'drop-shadow(0 0 14px color-mix(in srgb, var(--accent) 45%, transparent))'
                    : 'none',
                }}
              >
                {file
                  ? <CardArt folder={folder} file={file} />
                  : (
                      <div className="absolute inset-0 rounded-[var(--radius)] border border-border bg-surface-secondary dune-lift flex items-start justify-center">
                        <div className="mt-[22%] size-[34%] rounded-full border-2 border-accent/40 flex items-center justify-center text-accent">
                          <Icon name={CATEGORY_ICONS[cat] ?? 'circle'} className="size-1/2" />
                        </div>
                      </div>
                    )}
                <div
                  className="absolute inset-x-0 text-center font-bold tabular-nums text-accent"
                  style={{
                    top: PANEL_CENTER[folder] ?? PANEL_CENTER_DEFAULT,
                    transform: 'translateY(-50%)',
                    fontSize: 'clamp(0.9rem, 1.6vw, 1.5rem)',
                    textShadow: '0 1px 3px rgba(0,0,0,.95), 0 0 10px rgba(0,0,0,.8)',
                  }}
                >
                  {stats.pct}
                  %
                </div>
              </div>

              {/* Tier / intel / item counts below the card */}
              <div
                className="flex items-center justify-center gap-2 text-muted tabular-nums"
                style={{ fontSize: 'clamp(0.6rem, 0.85vw, 0.8rem)' }}
              >
                <span className="flex items-center gap-1">
                  <Icon name={CATEGORY_ICONS[cat] ?? 'circle'} className="size-3" />
                  {stats.count}
                </span>
                <span className="flex items-center gap-1">
                  <Icon name="lightbulb" className="size-3" />
                  {stats.intel}
                </span>
                <span className="flex items-center gap-1">
                  <Icon name="package" className="size-3" />
                  {stats.items}
                </span>
              </div>
            </button>
          )
        })}
      </div>

      {/* Timeline for the selected category */}
      <div className="min-h-0">
        <div className="overflow-x-auto pb-3">
          <div className="relative flex items-start min-w-max px-2 pt-2">
            {lane.map((tier, i) => {
              const achieved = tierAchieved(counts, tier)
              const pct = playerCount > 0 ? Math.round((Math.min(achieved, playerCount) / playerCount) * 100) : 0
              const itemsN = itemCount(tier)
              return (
                <React.Fragment key={tier.tier_key}>
                  {i > 0 && (
                    <div className={`h-0.5 w-12 shrink-0 mt-7 ${pct > 0 ? 'bg-accent/60' : 'bg-border'}`} />
                  )}
                  <Tooltip delay={300}>
                    <Tooltip.Trigger>
                      <div className="flex flex-col items-center w-32 shrink-0 gap-1.5">
                        <div
                          className={`size-14 rounded-full border-2 flex items-center justify-center ${
                            pct >= 50
                              ? 'border-accent bg-accent/20 text-accent'
                              : pct > 0
                                ? 'border-accent/50 bg-surface text-accent/80'
                                : 'border-border bg-surface text-muted'
                          }`}
                        >
                          <RewardIcon tier={tier} className="w-7 h-7" />
                        </div>
                        <div
                          className="text-xs font-medium text-center leading-tight line-clamp-2 px-1"
                        >
                          {tier.label}
                        </div>
                        <div className="flex items-center gap-1.5 text-xs text-muted font-mono tabular-nums">
                          <span className="flex items-center gap-0.5">
                            <Icon name="lightbulb" className="size-3" />
                            {tier.intel}
                          </span>
                          {itemsN > 0 && (
                            <span className="flex items-center gap-0.5">
                              <Icon name="package" className="size-3" />
                              {itemsN}
                            </span>
                          )}
                        </div>
                        <Chip size="sm" variant="soft" color={pctColor(pct)} className="tabular-nums">
                          {pct}
                          %
                        </Chip>
                      </div>
                    </Tooltip.Trigger>
                    <Tooltip.Content>
                      <div className="text-xs">
                        <div className="font-medium">{tier.label}</div>
                        <div className="text-muted font-mono">{tier.tier_key}</div>
                        <div className="mt-1">
                          {t('battlepass.track.tooltip', { pct, count: achieved, total: playerCount })}
                        </div>
                      </div>
                    </Tooltip.Content>
                  </Tooltip>
                </React.Fragment>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
