import { useState, useEffect, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { MapContainer, ImageOverlay, CircleMarker, Tooltip, useMapEvents, useMap } from 'react-leaflet'
import { CRS, type LatLngBoundsExpression } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { api, ApiError } from '../api/client'
import type { MapMarker } from '../api/client'
import { Icon, PageHeader } from '../dune-ui'

// The map image is a square; CRS.Simple uses image-pixel space as the coordinate
// system. lat = up-fraction * H, lng = left-fraction * W.
const IMG_W = 1200
const IMG_H = 1200
const IMAGE_BOUNDS: LatLngBoundsExpression = [[0, 0], [IMG_H, IMG_W]]
const POLL_MS = 10000

// Marker fill colors. Leaflet draws markers as SVG, which can't consume the
// Tailwind/CSS semantic tokens, so these data-viz colors are intentionally literal.
const MARKER_COLOR: Record<string, string> = {
  player: '#3b9dff',
  vehicle: '#5fd35a',
  base: '#e0a13a',
}

type Bounds = { minX: number, maxX: number, minY: number, maxY: number, flipX?: boolean, flipY?: boolean }
type MapCfg = Bounds & { key: string, label: string, image?: string }

const MAPS: MapCfg[] = [
  // Calibrated from ~14 in-game click points spanning the whole map (near-isotropic: X~788k, Y~838k span).
  // Refine live via the Calibrate tool (persists to localStorage), then bake the numbers here.
  { key: 'HaggaBasin', label: 'Hagga Basin', image: 'hagga-basin.png', minX: -437871, maxX: 350539, minY: -462011, maxY: 376267, flipY: true },
  { key: 'DeepDesert', label: 'Deep Desert', minX: -1300000, maxX: 1200000, minY: -1300000, maxY: 1200000 },
]

const CALIB_LS_KEY = 'dune_admin_livemap_calib'

function clamp01(v: number): number {
  if (v < 0) return 0
  if (v > 1) return 1
  return v
}

function worldToLatLng(x: number, y: number, cfg: Bounds): [number, number] {
  const rawX = (x - cfg.minX) / (cfg.maxX - cfg.minX)
  const rawY = (y - cfg.minY) / (cfg.maxY - cfg.minY)
  const fracX = clamp01(cfg.flipX ? 1 - rawX : rawX)
  const fracYup = clamp01(cfg.flipY ? 1 - rawY : rawY)
  return [fracYup * IMG_H, fracX * IMG_W]
}

type CalibPoint = { wx: number, wy: number, fracX: number, fracYup: number }

// Solve world->image bounds from the first and last calibration points (most
// separated). fracYup is linear in y; a negative slope means the Y axis is flipped.
function solveBounds(pts: CalibPoint[]): Bounds | null {
  if (pts.length < 2) return null
  const a = pts[0]
  const b = pts[pts.length - 1]
  if (b.wx === a.wx || b.wy === a.wy || b.fracX === a.fracX || b.fracYup === a.fracYup) return null

  const sX = (b.fracX - a.fracX) / (b.wx - a.wx)
  const iX = a.fracX - sX * a.wx
  const minX = -iX / sX
  const maxX = (1 - iX) / sX

  const sY = (b.fracYup - a.fracYup) / (b.wy - a.wy)
  const iY = a.fracYup - sY * a.wy
  const flipY = sY < 0
  let minY: number
  let maxY: number
  if (!flipY) {
    const R = 1 / sY
    minY = -iY * R
    maxY = minY + R
  }
  else {
    const R = -1 / sY
    minY = (iY - 1) * R
    maxY = minY + R
  }
  return { minX, maxX, minY, maxY, flipY }
}

function loadCalib(): Record<string, Bounds> {
  try {
    return JSON.parse(localStorage.getItem(CALIB_LS_KEY) ?? '{}') as Record<string, Bounds>
  }
  catch {
    return {}
  }
}

// Keeps the leaflet map sized correctly when the tab becomes visible again
// (it lives in a display:none TabPane while inactive, which zeroes its size).
function InvalidateOnActive({ active }: { active: boolean }) {
  const map = useMap()
  useEffect(() => {
    if (active) {
      const id = setTimeout(() => map.invalidateSize(), 50)
      return () => clearTimeout(id)
    }
  }, [active, map])
  return null
}

function CalibrationCapture({ active, onPick }: { active: boolean, onPick: (lat: number, lng: number) => void }) {
  useMapEvents({
    click(e) {
      if (active) onPick(e.latlng.lat, e.latlng.lng)
    },
  })
  return null
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

  const baseCfg = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]
  const effCfg: MapCfg = useMemo(
    () => ({ ...baseCfg, ...(calibOverride[mapKey] ?? {}) }),
    [baseCfg, calibOverride, mapKey],
  )

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
        if (e instanceof ApiError && e.status === 404) setUnsupported(true)
        else toast.danger(t('liveMap.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
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

  // Players render last so they sit on top of co-located vehicles.
  const ordered = useMemo(
    () => [...markers].sort((a, b) => (a.type === 'player' ? 1 : 0) - (b.type === 'player' ? 1 : 0)),
    [markers],
  )

  const handleCalibPick = useCallback((lat: number, lng: number) => {
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
          catch { /* ignore quota errors */ }
          return merged
        })
      }
      return next
    })
  }, [markers, mapKey, t])

  const clearCalib = useCallback(() => {
    setCalibPoints([])
    setCalibOverride((c) => {
      const merged = { ...c }
      delete merged[mapKey]
      try {
        localStorage.setItem(CALIB_LS_KEY, JSON.stringify(merged))
      }
      catch { /* ignore */ }
      return merged
    })
  }, [mapKey])

  const solvedStr = useMemo(() => {
    const b = calibOverride[mapKey]
    if (!b) return ''
    return `minX: ${Math.round(b.minX)}, maxX: ${Math.round(b.maxX)}, minY: ${Math.round(b.minY)}, maxY: ${Math.round(b.maxY)}, flipY: ${!!b.flipY}`
  }, [calibOverride, mapKey])

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

      <div className="flex flex-wrap items-center gap-2 shrink-0">
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
        <Button size="sm" variant={calibrating ? 'primary' : 'outline'} onPress={() => setCalibrating((v) => !v)}>
          <Icon name="crosshair" />
          {' '}
          {t('liveMap.calibrate')}
        </Button>
        {calibrating && (
          <Button size="sm" variant="outline" onPress={clearCalib}>
            {t('liveMap.clear')}
          </Button>
        )}
      </div>

      <div className="flex flex-wrap gap-4 shrink-0 text-xs text-muted">
        <span>
          <span style={{ color: MARKER_COLOR.player }}>●</span>
          {' '}
          {t('liveMap.players')}
          {': '}
          {playerCount}
        </span>
        <span>
          <span style={{ color: MARKER_COLOR.vehicle }}>●</span>
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
        {updatedLabel !== '' && <span className="ml-auto">{t('liveMap.updated', { time: updatedLabel })}</span>}
      </div>

      {calibrating && (
        <div className="shrink-0 rounded-[var(--radius)] border border-border bg-surface px-3 py-2 text-xs">
          <div className="text-accent">{t('liveMap.calibActive')}</div>
          <div className="text-muted">
            {t('liveMap.calibPoints', { n: calibPoints.length })}
          </div>
          {solvedStr && <div className="mt-1 font-mono text-foreground break-all">{solvedStr}</div>}
        </div>
      )}

      {unsupported
        ? (
            <div className="py-8 text-center text-sm text-muted">{t('liveMap.unsupported')}</div>
          )
        : (
            <div className="relative flex-1 min-h-0 overflow-hidden rounded-[var(--radius)] border border-border">
              <MapContainer
                crs={CRS.Simple}
                bounds={IMAGE_BOUNDS}
                minZoom={-3}
                maxZoom={4}
                zoomSnap={0.25}
                attributionControl={false}
                style={{ height: '100%', width: '100%', background: 'var(--color-surface)', cursor: calibrating ? 'crosshair' : 'grab' }}
              >
                <InvalidateOnActive active={isActive} />
                <CalibrationCapture active={calibrating} onPick={handleCalibPick} />
                {effCfg.image && (
                  <ImageOverlay key={mapKey} url={`${import.meta.env.BASE_URL}${effCfg.image}`} bounds={IMAGE_BOUNDS} />
                )}
                {ordered.map((m) => {
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
                        fillColor: MARKER_COLOR[m.type] ?? MARKER_COLOR.base,
                        fillOpacity: 1,
                      }}
                    >
                      <Tooltip>
                        <div className="font-medium">{m.name || `${m.type} ${m.id}`}</div>
                        <div>
                          {m.type}
                          {m.online_status ? ` · ${m.online_status}` : ''}
                        </div>
                        <div>
                          {Math.round(m.x)}
                          {', '}
                          {Math.round(m.y)}
                          {', '}
                          {Math.round(m.z)}
                        </div>
                      </Tooltip>
                    </CircleMarker>
                  )
                })}
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
