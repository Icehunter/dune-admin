import * as React from 'react'
import type { BattlepassTier } from '../../api/client'
import { Icon } from '../../dune-ui'

type P = { className?: string }

// All icons share this SVG wrapper: 24×24 viewBox, currentColor, Lucide-style defaults
const Svg: React.FC<P & { children: React.ReactNode }> = ({ className, children }) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.5"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className ?? 'w-6 h-6'}
    aria-hidden="true"
  >
    {children}
  </svg>
)

// ── Fremen kindjal / crysknife ────────────────────────────────────────────────
// A slim curved blade tapering to a wicked point — the Fremen's signature weapon.
// The fuller groove and wrapped handle grip are hallmarks of the real prop design.
export const BladeIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* blade body — diamond cross-section tapers from ricasso to tip */}
    <path
      d="M12 2 C12.6 5 13.2 9 13 14.5 L12 17.5 L11 14.5 C10.8 9 11.4 5 12 2 Z"
      fill="currentColor"
      fillOpacity="0.22"
      strokeWidth="1.3"
    />
    {/* right cutting edge */}
    <path d="M12 2 C12.6 5 13.2 9 13 14.5 L12 17.5" strokeWidth="1.3" />
    {/* fuller groove — dashed, runs 80% of blade length */}
    <line x1="11.4" y1="4.5" x2="11.4" y2="13.5" strokeWidth="0.65" strokeOpacity="0.45" strokeDasharray="2 1.5" />
    {/* crossguard — wider than the blade, slight angle */}
    <path d="M7.5 18 L16.5 18" strokeWidth="2.6" strokeLinecap="round" />
    {/* grip — wrapped handle with three bands */}
    <rect x="10.5" y="18.8" width="3" height="3.5" rx="0.6" fill="currentColor" fillOpacity="0.14" strokeWidth="1.15" />
    <line x1="10.5" y1="19.8" x2="13.5" y2="19.8" strokeWidth="0.65" strokeOpacity="0.5" />
    <line x1="10.5" y1="21.1" x2="13.5" y2="21.1" strokeWidth="0.65" strokeOpacity="0.5" />
  </Svg>
)

// ── Ranged weapon — plasma pistol / SMG / rifle silhouette ────────────────────
// Geometric sci-fi side profile: extended barrel, boxy receiver, angled grip, sight pin.
export const RangedIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* barrel */}
    <rect x="2" y="10.5" width="13" height="3" rx="1" fill="currentColor" fillOpacity="0.15" strokeWidth="1.3" />
    {/* receiver / body */}
    <path d="M14 8 L21 8 L21 15 L14 15 Z" fill="currentColor" fillOpacity="0.15" strokeWidth="1.4" />
    {/* angled grip */}
    <path
      d="M17 15 L15.5 21 L19.5 21 L21 15"
      fill="currentColor"
      fillOpacity="0.1"
      strokeWidth="1.3"
    />
    {/* sight pin on top of receiver */}
    <line x1="19" y1="8" x2="19" y2="6.5" strokeWidth="1.5" />
    {/* muzzle flash dot */}
    <circle cx="3" cy="12" r="0.9" fill="currentColor" strokeWidth="0" />
    {/* ejection port detail */}
    <line x1="16" y1="10" x2="19" y2="10" strokeWidth="0.75" strokeOpacity="0.45" />
  </Svg>
)

// ── Flamethrower — Shaitan's Tongue ──────────────────────────────────────────
// A compact fuel tank on the hip, short nozzle, and a roiling mushroom of flame
// with a brighter inner core — fire is alive, not just a shape.
export const FlameIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* fuel tank — compact cylinder */}
    <rect x="2" y="14" width="5.5" height="7" rx="2" fill="currentColor" fillOpacity="0.18" strokeWidth="1.3" />
    {/* nozzle / barrel connecting tank to flame */}
    <line x1="7.5" y1="16" x2="14" y2="13.5" strokeWidth="2.2" />
    {/* outer flame billow — irregular organic shape */}
    <path
      d="M14 13.5 C15.5 8.5 20 9.5 19 4.5 C23 7 23.5 14.5 19.5 17 C18 18.5 15.5 18 14 13.5 Z"
      fill="currentColor"
      fillOpacity="0.18"
      strokeWidth="1.3"
    />
    {/* inner hot core — brighter, tighter */}
    <path
      d="M15.5 13.5 C16.5 10.5 19.5 11.5 19 8.5 C21.5 10.5 21.5 15 19 16.5 C17.5 17.5 15.5 16.5 15.5 13.5 Z"
      fill="currentColor"
      fillOpacity="0.42"
      strokeWidth="0.7"
    />
  </Svg>
)

// ── Stillsuit — precious water recirculation survival suit ────────────────────
// Arrakis' most sacred technology. A teardrop silhouette (water = life)
// with internal tube loops etched inside — the stillsuit's capillary network.
export const StillsuitIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* outer droplet — slightly stylised, not perfectly round at the base */}
    <path
      d="M12 3 C8.5 9 6 13.5 6 16 A6 6 0 0 0 18 16 C18 13.5 15.5 9 12 3 Z"
      fill="currentColor"
      fillOpacity="0.18"
      strokeWidth="1.4"
    />
    {/* upper recirculation loop */}
    <path
      d="M9 12.5 C10.5 11.5 12 13 13.5 12"
      strokeWidth="1"
      strokeOpacity="0.58"
      fill="none"
    />
    {/* lower recirculation loop */}
    <path
      d="M9.5 15.5 C11 14.5 12.5 16 14 15"
      strokeWidth="1"
      strokeOpacity="0.58"
      fill="none"
    />
    {/* central micro-filter valve dot */}
    <circle cx="12" cy="17.5" r="1.2" fill="currentColor" fillOpacity="0.48" strokeWidth="0.6" />
  </Svg>
)

// ── Power pack — Old Sparky energy cell ──────────────────────────────────────
// The portable power unit that drives shields and kit. Classic battery silhouette
// with a bold lightning bolt — the kinetic promise inside.
export const PowerPackIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* battery body */}
    <rect x="5.5" y="5.5" width="13" height="16.5" rx="2" fill="currentColor" fillOpacity="0.15" strokeWidth="1.4" />
    {/* positive terminal nub */}
    <rect x="9" y="3.5" width="6" height="2.5" rx="1" fill="currentColor" fillOpacity="0.25" strokeWidth="1.2" />
    {/* lightning bolt — the energy inside */}
    <path
      d="M13.5 8.5 L10 14 L13 14 L10.5 19.5 L16 13 L12.5 13 Z"
      fill="currentColor"
      fillOpacity="0.62"
      strokeWidth="0.75"
    />
  </Svg>
)

// ── Scanner — handheld life / survey scanner ──────────────────────────────────
// A radar display: outer housing ring, inner detection ring, a rotating sweep
// beam frozen mid-arc, and a blip dot where the beam last hit.
export const ScannerIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* outer housing ring */}
    <circle cx="12" cy="12" r="9.5" strokeWidth="1.4" />
    {/* inner detection radius ring */}
    <circle cx="12" cy="12" r="5.5" strokeWidth="0.9" strokeOpacity="0.42" />
    {/* sweep beam */}
    <line x1="12" y1="12" x2="20" y2="6.5" strokeWidth="1.4" />
    {/* trailing arc ghost of the sweep */}
    <path d="M 12 2.5 A 9.5 9.5 0 0 1 20 6.5" strokeWidth="1.1" strokeOpacity="0.48" fill="none" />
    {/* centre pivot dot */}
    <circle cx="12" cy="12" r="1.5" fill="currentColor" strokeWidth="0" />
    {/* contact blip on the inner ring */}
    <circle cx="17" cy="8" r="1.1" fill="currentColor" fillOpacity="0.52" strokeWidth="0" />
  </Svg>
)

// ── Dew Reaper — water-harvesting scythe ─────────────────────────────────────
// The Dew Reaper sweeps moisture from the air at dawn. A long pole, a curved
// harvest blade echoing a crescent moon, and a pendant water drop at the tip.
export const DewReaperIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* handle pole — diagonal, grip at top right */}
    <line x1="18" y1="3" x2="11" y2="21" strokeWidth="1.5" />
    {/* curved harvest blade — sweeps from pole tip around to collection point */}
    <path
      d="M17.5 3.5 C21.5 7.5 21.5 17.5 11 21 C15.5 15 17 9.5 13 7.5 C14.5 4 17.5 3.5 17.5 3.5 Z"
      fill="currentColor"
      fillOpacity="0.16"
      strokeWidth="1.3"
    />
    {/* pendant water drop at collection tip — the whole point */}
    <path
      d="M11 19.5 C9 21 9.5 23 11.5 23 C13.5 23 14 21 12 19.5 C11.5 19 11 19.5 11 19.5 Z"
      fill="currentColor"
      fillOpacity="0.48"
      strokeWidth="0.9"
      strokeLinejoin="round"
    />
  </Svg>
)

// ── Body fluid extractor — survival vial ─────────────────────────────────────
// A sealed glass vial with a stopper, three calibration marks, and a dark
// fluid meniscus — equal parts medical instrument and Arrakis survival kit.
export const ExtractorIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* vial body — rounded at the bottom where fluid collects */}
    <path
      d="M9 4 L9 17.5 Q9 22 12 22 Q15 22 15 17.5 L15 4 Z"
      fill="currentColor"
      fillOpacity="0.15"
      strokeWidth="1.4"
    />
    {/* stopper cap */}
    <rect x="7.5" y="2" width="9" height="3" rx="0.75" fill="currentColor" fillOpacity="0.22" strokeWidth="1.2" />
    {/* fluid meniscus line */}
    <path d="M9.5 15 Q12 13.5 14.5 15" strokeWidth="1" strokeOpacity="0.55" fill="none" />
    {/* calibration marks on left wall */}
    <line x1="9" y1="10.5" x2="11" y2="10.5" strokeWidth="0.8" strokeOpacity="0.45" />
    <line x1="9" y1="13" x2="11" y2="13" strokeWidth="0.8" strokeOpacity="0.45" />
    <line x1="9" y1="17.5" x2="11" y2="17.5" strokeWidth="0.8" strokeOpacity="0.45" />
    {/* fluid content — dark settled at the bottom */}
    <path
      d="M9.5 16.5 Q12 15.5 14.5 16.5 L14.5 18.5 Q12 21.5 9.5 18.5 Z"
      fill="currentColor"
      fillOpacity="0.38"
      strokeWidth="0"
    />
  </Svg>
)

// ── Static compactor — industrial compression machine ─────────────────────────
// Two heavy plates — one fixed, one driven — squeezing a block between them.
// Directional chevron arrows make the force visible.
export const CompactorIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* top driven plate */}
    <rect x="3" y="3.5" width="18" height="4.5" rx="1" fill="currentColor" fillOpacity="0.2" strokeWidth="1.4" />
    {/* bottom fixed plate */}
    <rect x="3" y="16" width="18" height="4.5" rx="1" fill="currentColor" fillOpacity="0.2" strokeWidth="1.4" />
    {/* material being compacted */}
    <rect x="6.5" y="10.5" width="11" height="3" rx="0.5" fill="currentColor" fillOpacity="0.32" strokeWidth="1.1" />
    {/* downward force arrow from upper plate */}
    <path d="M12 8 L12 10.5" strokeWidth="1.4" />
    <path d="M10.2 9.8 L12 11.5 L13.8 9.8" strokeWidth="1.3" fill="none" />
    {/* upward reaction arrow from lower plate */}
    <path d="M12 13.5 L12 16" strokeWidth="1.4" />
    <path d="M10.2 14.2 L12 12.5 L13.8 14.2" strokeWidth="1.3" fill="none" />
  </Svg>
)

// ── Schematic fragment — T6 crafting blueprint shard ─────────────────────────
// A jagged hexagonal shard that reads as "a broken-off piece of something larger."
// Blueprint circuit traces inside hint at the technology locked within.
export const FragmentIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* irregular shard — hexagonal with one fractured edge at top-left */}
    <path
      d="M14.5 2 L21 7.5 L19 15.5 L13 18.5 L7 15 L6 7 Z"
      fill="currentColor"
      fillOpacity="0.15"
      strokeWidth="1.4"
    />
    {/* fracture break — dashed line suggesting it was cleaved from a larger piece */}
    <path d="M6 7 L14.5 2" strokeWidth="0.7" strokeOpacity="0.38" strokeDasharray="2 1.5" />
    {/* circuit trace — horizontal run with a node */}
    <path d="M9 10 L12.5 10 L12.5 13" strokeWidth="1" strokeOpacity="0.55" fill="none" />
    <path d="M12.5 13 L16 13" strokeWidth="1" strokeOpacity="0.55" fill="none" />
    {/* junction node */}
    <circle cx="12.5" cy="10" r="1" fill="currentColor" fillOpacity="0.5" strokeWidth="0" />
  </Svg>
)

// ── Suspensor pack — anti-gravity levitation device ───────────────────────────
// A softly glowing device that punches upward thrust. A spherical core hovers
// above a wide levitation field ellipse; emission lines spike outward like heat shimmer.
export const SuspensorIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* levitation field — wide flat ellipse below the device */}
    <ellipse cx="12" cy="17.5" rx="9.5" ry="3" strokeWidth="1.4" />
    {/* secondary field ring — slightly inside, faded */}
    <ellipse cx="12" cy="16" rx="6" ry="1.8" strokeWidth="0.9" strokeOpacity="0.38" />
    {/* device body — the actual suspensor housing */}
    <circle cx="12" cy="10.5" r="4" fill="currentColor" fillOpacity="0.18" strokeWidth="1.4" />
    {/* glowing energy point at core */}
    <circle cx="12" cy="10.5" r="1.6" fill="currentColor" fillOpacity="0.55" strokeWidth="0" />
    {/* upward emission lines — three rays */}
    <line x1="12" y1="6.5" x2="12" y2="4.5" strokeWidth="1.2" strokeOpacity="0.42" />
    <line x1="15.5" y1="7.5" x2="17" y2="6" strokeWidth="1" strokeOpacity="0.35" />
    <line x1="8.5" y1="7.5" x2="7" y2="6" strokeWidth="1" strokeOpacity="0.35" />
  </Svg>
)

// ── Desert combat helmet ──────────────────────────────────────────────────────
// A hard angular dome, a horizontal visor slit that cuts across the face like
// a wound, cheek flares that deflect sandblast, and a chin strap curve.
export const HelmetIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* dome */}
    <path
      d="M4 14.5 Q4 4 12 4 Q20 4 20 14.5 Z"
      fill="currentColor"
      fillOpacity="0.18"
      strokeWidth="1.4"
    />
    {/* left cheek guard — angled flare */}
    <path
      d="M4 14.5 L4.5 20.5 L8.5 21 L9.5 14.5"
      fill="currentColor"
      fillOpacity="0.12"
      strokeWidth="1.2"
    />
    {/* right cheek guard */}
    <path
      d="M20 14.5 L19.5 20.5 L15.5 21 L14.5 14.5"
      fill="currentColor"
      fillOpacity="0.12"
      strokeWidth="1.2"
    />
    {/* visor slit — the distinctive horizontal gap, heavy and shadowed */}
    <line x1="7.5" y1="13.5" x2="16.5" y2="13.5" strokeWidth="3.2" strokeOpacity="0.48" strokeLinecap="butt" />
    {/* visor brow ridge above the slit */}
    <path d="M6.5 12 L17.5 12" strokeWidth="0.85" strokeOpacity="0.32" />
    {/* chin strap curve at base */}
    <path d="M4.5 20.5 Q12 23.5 19.5 20.5" strokeWidth="1.2" fill="none" />
  </Svg>
)

// ── Full armor set — layered heraldic shields ─────────────────────────────────
// Three shields slightly staggered in depth — communicates "multiple pieces,
// a complete protection system." The foremost shield has a central boss and
// an inlaid horizontal stripe.
export const ArmorSetIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* rearmost shield — offset up-right, lowest opacity */}
    <path
      d="M16 3 L22 6 L22 13.5 Q22 19 16 21.5"
      fill="currentColor"
      fillOpacity="0.08"
      strokeWidth="1.1"
      strokeOpacity="0.4"
    />
    {/* middle shield */}
    <path
      d="M12.5 4 L19 6.5 L19 14 Q19 19.5 12.5 22 Q6 19.5 6 14 L6 6.5 Z"
      fill="currentColor"
      fillOpacity="0.12"
      strokeWidth="1.2"
      strokeOpacity="0.55"
    />
    {/* front shield — main, boldest */}
    <path
      d="M4.5 5 L12 2 L19.5 5 L19.5 13 Q19.5 19.5 12 22.5 Q4.5 19.5 4.5 13 Z"
      fill="currentColor"
      fillOpacity="0.2"
      strokeWidth="1.5"
    />
    {/* horizontal stripe across the front shield (heraldic band) */}
    <path d="M6.5 12 L17.5 12" strokeWidth="0.9" strokeOpacity="0.38" />
    {/* central boss / umbo */}
    <circle cx="12" cy="10.5" r="2.5" fill="currentColor" fillOpacity="0.3" strokeWidth="0.9" />
  </Svg>
)

// ── Single armor piece — pauldron / chest plate ───────────────────────────────
// For individual non-helmet armor pieces (chest, gloves, boots, legs).
// A wider shield profile without the stacked depth — one solid plate.
export const ArmorPieceIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    <path
      d="M4 6 L12 3 L20 6 L20 15 Q20 21 12 23 Q4 21 4 15 Z"
      fill="currentColor"
      fillOpacity="0.2"
      strokeWidth="1.5"
    />
    {/* mid-panel horizontal crease */}
    <path d="M6 13 L18 13" strokeWidth="0.85" strokeOpacity="0.4" />
    {/* central boss */}
    <circle cx="12" cy="9.5" r="2.2" fill="currentColor" fillOpacity="0.28" strokeWidth="0.9" />
  </Svg>
)

// ── Generic schematic scroll — fallback for unclassified rewards ──────────────
// A rolled blueprint — clearly "a crafting schematic" for anything that doesn't
// have a more specific icon.
export const SchematicIcon: React.FC<P> = ({ className }) => (
  <Svg className={className}>
    {/* scroll body */}
    <rect x="5" y="5" width="14" height="15" rx="1" fill="currentColor" fillOpacity="0.15" strokeWidth="1.4" />
    {/* top rolled edge */}
    <path d="M5 6 Q5 3 8 5 L16 5 Q19 3 19 6" fill="currentColor" fillOpacity="0.1" strokeWidth="1.2" />
    {/* bottom rolled edge */}
    <path d="M5 19 Q5 22 8 20 L16 20 Q19 22 19 19" fill="currentColor" fillOpacity="0.1" strokeWidth="1.2" />
    {/* blueprint lines */}
    <line x1="8" y1="9.5" x2="16" y2="9.5" strokeWidth="0.9" strokeOpacity="0.5" />
    <line x1="8" y1="12.5" x2="16" y2="12.5" strokeWidth="0.9" strokeOpacity="0.5" />
    <line x1="8" y1="15.5" x2="13" y2="15.5" strokeWidth="0.9" strokeOpacity="0.5" />
  </Svg>
)

// ── Classification logic ──────────────────────────────────────────────────────

const CATEGORY_ICONS_FALLBACK: Record<string, string> = {
  level: 'chevrons-up',
  story: 'book-open',
  side_quest: 'map',
  faction: 'landmark',
  exploration: 'compass',
  achievement: 'trophy',
}

interface RawItem { Template: string }

const classifyByTemplate = (tpl: string, count: number, className?: string): React.ReactElement => {
  if (count >= 3) return <ArmorSetIcon className={className} />

  const t = tpl.toLowerCase()
  if (t.includes('sword') || t.includes('kindjal') || t.includes('rapier')
    || t.includes('dirk') || t.includes('cutteray')) return <BladeIcon className={className} />
  if (t.includes('flamethrower')) return <FlameIcon className={className} />
  if (t.includes('pistol') || t.includes('smg') || t.includes('longrifle')
    || t.includes('lmg') || t.includes('shotgun') || t.includes('ar_burst')
    || t.includes('uniquear')) return <RangedIcon className={className} />
  if (t.includes('stillsuit')) return <StillsuitIcon className={className} />
  if (t.includes('powerpack')) return <PowerPackIcon className={className} />
  if (t.includes('sandbike') || t.includes('scanner')) return <ScannerIcon className={className} />
  if (t.includes('dewreap')) return <DewReaperIcon className={className} />
  if (t.includes('extractor') || t.includes('bloodsack')) return <ExtractorIcon className={className} />
  if (t.includes('compactor')) return <CompactorIcon className={className} />
  if (t.includes('fragment')) return <FragmentIcon className={className} />
  if (t.includes('suspensor')) return <SuspensorIcon className={className} />
  if (t.includes('helmet') || t.includes('head') || t.includes('mask')
    || t.includes('wrap')) return <HelmetIcon className={className} />
  if (t.includes('top') || t.includes('chest') || t.includes('jacket')
    || t.includes('garb') || t.includes('gloves') || t.includes('gauntlet')
    || t.includes('boots') || t.includes('softstep') || t.includes('feet')
    || t.includes('bottom') || t.includes('legs') || t.includes('pants')
    || t.includes('legging')) return <ArmorPieceIcon className={className} />
  return <SchematicIcon className={className} />
}

/**
 * Renders a hand-drawn SVG icon that reflects the tier's actual reward:
 * weapon shape, stillsuit, scanner, armor set, etc. Falls back to the
 * category icon (Lucide) for tiers with no item rewards (achievements).
 */
export const RewardIcon: React.FC<{ tier: BattlepassTier, className?: string }> = ({ tier, className }) => {
  if (!tier.reward_items) {
    return <Icon name={CATEGORY_ICONS_FALLBACK[tier.category] ?? 'circle'} className={className} />
  }

  let items: RawItem[] = []
  try {
    items = JSON.parse(tier.reward_items) as RawItem[]
  }
  catch { /* fall through */ }

  if (items.length === 0) {
    return <Icon name={CATEGORY_ICONS_FALLBACK[tier.category] ?? 'circle'} className={className} />
  }

  return classifyByTemplate(items[0]?.Template ?? '', items.length, className)
}
