import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { api, ApiError } from '../api/client'
import type { MapMarker } from '../api/client'
import { Icon, PageHeader } from '../dune-ui'

// Placeholder world->fraction bounds per map. Round, self-chosen approximations
// used only until a real (GPL-clean) base map image is calibrated in (Phase 0).
type MapCfg = {
  key: string
  label: string
  image?: string // file in web/public, e.g. 'hagga-basin.png'
  minX: number
  maxX: number
  minY: number
  maxY: number
  flipX?: boolean
  flipY?: boolean
}

// World->image calibration. Hagga bounds are an initial GUESS (from the same
// game's reference Hagga map) pending real calibration against the screenshot —
// flipX/flipY correct mirror orientation once we have reference points.
const MAPS: MapCfg[] = [
  { key: 'HaggaBasin', label: 'Hagga Basin', image: 'hagga-basin.png', minX: -456752, maxX: 354547, minY: -450630, maxY: 353822 },
  { key: 'DeepDesert', label: 'Deep Desert', minX: -1300000, maxX: 1200000, minY: -1300000, maxY: 1200000 },
]

const POLL_MS = 10000

function clamp01(v: number): number {
  if (v < 0) return 0
  if (v > 1) return 1
  return v
}

function markerDot(type: string): string {
  switch (type) {
    case 'player':
      return 'bg-primary'
    case 'vehicle':
      return 'bg-success'
    default:
      return 'bg-danger'
  }
}

export default function LiveMapTab({ isActive = true }: { isActive?: boolean }) {
  const { t } = useTranslation()
  const [mapKey, setMapKey] = useState<string>('HaggaBasin')
  const [markers, setMarkers] = useState<MapMarker[]>([])
  const [loading, setLoading] = useState(false)
  const [unsupported, setUnsupported] = useState(false)
  const [updatedLabel, setUpdatedLabel] = useState<string>('')

  const cfg = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]

  const load = useCallback((key: string) => {
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
        if (e instanceof ApiError && e.status === 404) {
          setUnsupported(true)
        }
        else {
          toast.danger(t('liveMap.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
        }
        setMarkers([])
      })
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    if (!isActive) return
    load(mapKey)
    const id = setInterval(() => load(mapKey), POLL_MS)
    return () => clearInterval(id)
  }, [mapKey, isActive, load])

  const playerCount = markers.filter((m) => m.type === 'player').length
  const vehicleCount = markers.filter((m) => m.type === 'vehicle').length

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('liveMap.title')} subtitle={t('liveMap.subtitle')}>
        <Button size="sm" variant="ghost" onPress={() => load(mapKey)} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      <div className="flex gap-2 shrink-0">
        {MAPS.map((m) => (
          <Button
            key={m.key}
            size="sm"
            variant={m.key === mapKey ? 'primary' : 'outline'}
            onPress={() => setMapKey(m.key)}
          >
            {m.label}
          </Button>
        ))}
      </div>

      <div className="flex gap-4 shrink-0 text-xs text-muted">
        <span>
          <span className="text-primary">●</span>
          {' '}
          {t('liveMap.players')}
          {': '}
          {playerCount}
        </span>
        <span>
          <span className="text-success">●</span>
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
        {updatedLabel !== '' && (
          <span className="ml-auto">
            {t('liveMap.updated', { time: updatedLabel })}
          </span>
        )}
      </div>

      {unsupported
        ? (
            <div className="py-8 text-center text-sm text-muted">{t('liveMap.unsupported')}</div>
          )
        : (
            <div className="flex flex-1 min-h-0 items-center justify-center">
              <div
                className="relative aspect-square h-full max-w-full overflow-hidden rounded-[var(--radius)] border border-border bg-surface"
                style={cfg.image
                  ? { backgroundImage: `url(${import.meta.env.BASE_URL}${cfg.image})`, backgroundSize: '100% 100%' }
                  : undefined}
              >
                <div
                  className="pointer-events-none absolute inset-0 opacity-25"
                  style={{
                    backgroundImage:
                      'linear-gradient(var(--color-surface-alt) 1px, transparent 1px), linear-gradient(90deg, var(--color-surface-alt) 1px, transparent 1px)',
                    backgroundSize: '10% 10%',
                  }}
                />
                <div className="absolute left-2 top-2 rounded bg-surface/80 px-1.5 py-0.5 text-[10px] text-muted">
                  {cfg.image ? t('liveMap.calibrating') : t('liveMap.placeholderNote')}
                </div>

                {markers.map((m) => {
                  const rawX = (m.x - cfg.minX) / (cfg.maxX - cfg.minX)
                  const rawY = (m.y - cfg.minY) / (cfg.maxY - cfg.minY)
                  const leftPct = clamp01(cfg.flipX ? 1 - rawX : rawX) * 100
                  const topPct = clamp01(cfg.flipY ? rawY : 1 - rawY) * 100
                  const isPlayer = m.type === 'player'
                  return (
                    <div
                      key={`${m.type}-${m.id}`}
                      className={`group absolute -translate-x-1/2 -translate-y-1/2 ${isPlayer ? 'z-20' : 'z-10'} hover:z-30`}
                      style={{ left: `${leftPct}%`, top: `${topPct}%` }}
                    >
                      <div
                        className={`rounded-full ${markerDot(m.type)} ${isPlayer ? 'h-4 w-4' : 'h-2.5 w-2.5'}`}
                        style={{
                          boxShadow: isPlayer
                            ? '0 0 0 2px var(--color-surface), 0 0 10px 2px var(--color-primary)'
                            : '0 0 0 1.5px var(--color-surface)',
                        }}
                      />
                      <div className="absolute left-3 top-0 z-10 hidden whitespace-nowrap rounded border border-border bg-surface px-2 py-1 text-[11px] text-foreground group-hover:block">
                        <div className="font-medium">{m.name || `${m.type} ${m.id}`}</div>
                        <div className="text-muted">
                          {m.type}
                          {m.online_status ? ` · ${m.online_status}` : ''}
                        </div>
                        <div className="font-mono text-muted">
                          {Math.round(m.x)}
                          {', '}
                          {Math.round(m.y)}
                          {', '}
                          {Math.round(m.z)}
                        </div>
                      </div>
                    </div>
                  )
                })}

                {loading && markers.length === 0 && (
                  <div className="absolute inset-0 flex items-center justify-center">
                    <Spinner size="lg" />
                  </div>
                )}
              </div>
            </div>
          )}
    </div>
  )
}
