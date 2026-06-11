import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Select, ListBox, Spinner, toast } from '@heroui/react'
import { MapContainer, ImageOverlay, CircleMarker, Marker, Tooltip } from 'react-leaflet'
import L from 'leaflet'
import { CRS } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { api, ApiError } from '../../api/client'
import type { MapMarker, Player } from '../../api/client'
import { ConfirmDialog, Icon, PageHeader } from '../../dune-ui'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { InvalidateOnActive } from './components/InvalidateOnActive'
import { MapClickCapture } from './components/MapClickCapture'
import { SpawnCanvasLayer } from './components/SpawnCanvasLayer'
import { HeatmapCanvasLayer } from './components/HeatmapCanvasLayer'
import { MapTileLayer } from './components/MapTileLayer'
import { ZoneGridLayer } from './components/ZoneGridLayer'
import { FitBoundsController } from './components/FitBoundsController'
import { FilterPanel } from './components/FilterPanel'
import {
  MAPS, CAT_COLOR, IMAGE_BOUNDS, POLL_MS, IMG_H, IMG_W,
} from './constants'
import {
  worldToLatLng, latLngToWorld, solveBounds, loadCalib, loadFilter, saveFilter, mapUrl,
} from './utils'
import type { LiveMapTabProps, SpawnEntry, SpawnFile, CalibPoint, MapCfg, Bounds } from './types'

export const LiveMapTab: React.FC<LiveMapTabProps> = ({ isActive = true }) => {
  const { t } = useTranslation()
  const [mapKey, setMapKey] = React.useState<string>('HaggaBasin')
  const [markers, setMarkers] = React.useState<MapMarker[]>([])
  const [loading, setLoading] = React.useState(false)
  const [unsupported, setUnsupported] = React.useState(false)
  const [updatedLabel, setUpdatedLabel] = React.useState<string>('')
  const [calibrating, setCalibrating] = React.useState(false)
  const [calibPoints, setCalibPoints] = React.useState<CalibPoint[]>([])
  const [calibOverride, setCalibOverride] = React.useState<Record<string, Bounds>>(() => loadCalib())

  const [spawns, setSpawns] = React.useState<SpawnEntry[]>([])
  const loadedSpawnKey = React.useRef<string>('')
  const isDragging = React.useRef(false)

  const [filter, setFilter] = React.useState<Record<string, boolean>>(loadFilter)
  const [selectedFlsId, setSelectedFlsId] = React.useState<string>('')
  const [dragConfirm, setDragConfirm] = React.useState<{
    flsId: string
    name: string
    x: number
    y: number
  } | null>(null)

  const [heatmapMode, setHeatmapMode] = React.useState(false)
  const fitBoundsRef = React.useRef<(() => void) | null>(null)
  const [teleportMode, setTeleportMode] = React.useState(false)
  const [teleportDest, setTeleportDest] = React.useState<{ x: number, y: number } | null>(null)
  const [teleportFlsId, setTeleportFlsId] = React.useState<string>('')
  const [allPlayers, setAllPlayers] = React.useState<Player[]>([])
  const [teleporting, setTeleporting] = React.useState(false)

  const baseCfg = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]
  const effCfg: MapCfg = React.useMemo(
    () => ({ ...baseCfg, ...(calibOverride[mapKey] ?? {}) }),
    [baseCfg, calibOverride, mapKey],
  )

  const load = React.useCallback((key: string) => {
    if (isDragging.current) return
    const cfg = MAPS.find((m) => m.key === key)
    if (!cfg?.hasLiveData) {
      setMarkers([])
      setUnsupported(false)
      setUpdatedLabel(new Date().toLocaleTimeString())
      return
    }
    Promise.resolve()
      .then(() => {
        if (isDragging.current) return
        setLoading(true)
        setUnsupported(false)
      })
      .then(() => api.map.markers(key))
      .then((rows) => {
        if (isDragging.current) return
        setMarkers(rows)
        setUpdatedLabel(new Date().toLocaleTimeString())
      })
      .catch((e: unknown) => {
        if (isDragging.current) return
        if (e instanceof ApiError && e.status === 404) setUnsupported(true)
        else toast.danger(t('liveMap.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
        setMarkers([])
      })
      .finally(() => { if (!isDragging.current) setLoading(false) })
  }, [t])

  const loadCurrent = React.useCallback(() => load(mapKey), [load, mapKey])
  React.useEffect(() => {
    if (isActive) {
      const id = setTimeout(loadCurrent, 0)
      return () => clearTimeout(id)
    }
  }, [isActive, loadCurrent])
  const { countdown, refresh } = useAutoRefresh(loadCurrent, POLL_MS, isActive)

  React.useEffect(() => {
    const cfg = MAPS.find((m) => m.key === mapKey)
    if (!cfg?.spawnFile || loadedSpawnKey.current === mapKey) return
    loadedSpawnKey.current = mapKey
    fetch(mapUrl(`map-data/${cfg.spawnFile}-spawns.json`))
      .then((r) => r.json() as Promise<SpawnFile>)
      .then((d) => setSpawns(d.spawns))
      .catch(() => setSpawns([]))
  }, [mapKey])

  React.useEffect(() => {
    if (teleportMode && allPlayers.length === 0) {
      api.players.list().then(setAllPlayers).catch(() => {})
    }
  }, [teleportMode, allPlayers.length])

  const playerCount = markers.filter((m) => m.type === 'player').length
  const vehicleCount = markers.filter((m) => m.type === 'vehicle').length
  const baseCount = markers.filter((m) => m.type === 'base').length
  const orderedLive = React.useMemo(
    () => [...markers]
      .sort((a, b) => (a.type === 'player' ? 1 : 0) - (b.type === 'player' ? 1 : 0))
      .map((m) => {
        const isPlayer = m.type === 'player'
        const isBase = m.type === 'base'
        const size = isPlayer ? 32 : isBase ? 28 : 24
        const baseColor = CAT_COLOR[m.type] ?? CAT_COLOR.base
        const label = isPlayer ? (m.name?.[0]?.toUpperCase() ?? '?') : isBase ? '🏠' : '🚗'
        const cursor = isPlayer ? 'grab' : 'default'
        const makeHtml = (color: string) =>
          `<div style="width:${size}px;height:${size}px;border-radius:50%;background:${color};border:2.5px solid #0b0b0b;box-shadow:0 0 0 1.5px ${color}40;display:flex;align-items:center;justify-content:center;font-size:9px;font-weight:700;color:#0b0b0b;line-height:1;cursor:${cursor}">${label}</div>`
        const iconOpts = { iconSize: [size, size] as L.PointTuple, iconAnchor: [size / 2, size / 2] as L.PointTuple, className: '' }
        return {
          ...m,
          center: worldToLatLng(m.x, m.y, effCfg) as L.LatLngTuple,
          isPlayer,
          isBase,
          size,
          icon: L.divIcon({ ...iconOpts, html: makeHtml(baseColor) }),
          selectedIcon: L.divIcon({ ...iconOpts, html: makeHtml('#f59e0b') }),
        }
      }),
    [markers, effCfg],
  )

  const handleMapClick = React.useCallback((lat: number, lng: number) => {
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
              localStorage.setItem('dune_admin_livemap_calib', JSON.stringify(merged))
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

  const clearCalib = React.useCallback(() => {
    setCalibPoints([])
    setCalibOverride((c) => {
      const merged = { ...c }
      delete merged[mapKey]
      try {
        localStorage.setItem('dune_admin_livemap_calib', JSON.stringify(merged))
      }
      catch { /* quota */ }
      return merged
    })
  }, [mapKey])

  const solvedStr = React.useMemo(() => {
    const b = calibOverride[mapKey]
    return b
      ? `minX: ${Math.round(b.minX)}, maxX: ${Math.round(b.maxX)}, minY: ${Math.round(b.minY)}, maxY: ${Math.round(b.maxY)}, flipY: ${!!b.flipY}`
      : ''
  }, [calibOverride, mapKey])

  const doTeleport = React.useCallback(async () => {
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

  const toggleFilter = React.useCallback((key: string, currentVisual: boolean) => {
    setFilter((f) => {
      const next = { ...f, [key]: !currentVisual }
      saveFilter(next)
      return next
    })
  }, [])

  const clearFilters = React.useCallback(() => {
    setFilter((f) => {
      const next: Record<string, boolean> = {}
      Object.keys(f).forEach((k) => {
        next[k] = false
      })
      Object.assign(next, { players: true, vehicles: true, bases: true })
      saveFilter(next)
      return next
    })
  }, [])

  const mapCursor = calibrating || teleportMode ? 'crosshair' : 'grab'
  const currentMap = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">

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

      <div className="flex items-center gap-2 shrink-0">
        <Select
          aria-label={t('liveMap.title')}
          selectedKey={mapKey}
          onSelectionChange={(k) => {
            const key = String(k)
            loadedSpawnKey.current = ''
            setMapKey(key)
            setSpawns([])
            setTeleportDest(null)
            setCalibrating(false)
          }}
          className="w-44"
        >
          <Select.Trigger>
            <Icon name="map" className="size-3.5 text-muted shrink-0 mr-1" />
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {MAPS.map((m) => (
                <ListBox.Item key={m.key} id={m.key} textValue={m.label}>
                  {m.label}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>

        <div className="h-4 border-l border-border mx-0.5" />

        <Button size="sm" variant="outline" onPress={() => fitBoundsRef.current?.()}>
          <Icon name="home" />
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
              <span style={{ color: CAT_COLOR.base }}>●</span>
              {' '}
              {t('liveMap.filterBases')}
              {': '}
              {baseCount}
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

      {calibrating && (
        <div className="shrink-0 rounded-[var(--radius)] border border-border bg-surface px-3 py-2 text-xs">
          <div className="text-accent">{t('liveMap.calibActive')}</div>
          <div className="text-muted">{t('liveMap.calibPoints', { n: calibPoints.length })}</div>
          {solvedStr && <div className="mt-1 font-mono text-foreground break-all">{solvedStr}</div>}
        </div>
      )}

      <div className="flex flex-1 min-h-0 gap-2 overflow-hidden">
        <FilterPanel
          filter={filter}
          onToggle={toggleFilter}
          onClear={clearFilters}
          spawns={spawns}
          mapKey={mapKey}
          heatmapMode={heatmapMode}
          onHeatmapToggle={() => setHeatmapMode((v) => !v)}
        />
        {unsupported
          ? <div className="flex-1 py-8 text-center text-sm text-muted">{t('liveMap.unsupported')}</div>
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
                  {effCfg.tileId
                    ? <MapTileLayer key={mapKey} tileId={effCfg.tileId} />
                    : effCfg.image && (
                      <ImageOverlay
                        key={mapKey}
                        url={mapUrl(`map-data/${effCfg.image}`)}
                        bounds={IMAGE_BOUNDS}
                      />
                    )}

                  {effCfg.depthFile && (
                    <ImageOverlay
                      key={`depth-${mapKey}`}
                      url={mapUrl(`map-data/${effCfg.depthFile}`)}
                      bounds={IMAGE_BOUNDS}
                      className="leaflet-depth-overlay"
                    />
                  )}

                  <FitBoundsController fitRef={fitBoundsRef} />

                  {mapKey === 'DeepDesert' && (
                    <ZoneGridLayer effCfg={effCfg} />
                  )}

                  {heatmapMode && (
                    <HeatmapCanvasLayer
                      mapKey={mapKey}
                      effCfg={effCfg}
                      filter={filter}
                    />
                  )}

                  <SpawnCanvasLayer
                    spawns={spawns}
                    effCfg={effCfg}
                    filter={filter}
                    heatmapMode={heatmapMode}
                  />

                  {(filter.players || filter.vehicles) && orderedLive
                    .filter((m) => m.type === 'player' ? filter.players : m.type === 'vehicle' ? filter.vehicles : false)
                    .map((m) => {
                      const { center, isPlayer, size, icon, selectedIcon } = m
                      const isSelected = m.fls_id === selectedFlsId
                      return (
                        <Marker
                          key={`${m.type}-${m.id}`}
                          position={center}
                          icon={isSelected ? selectedIcon : icon}
                          draggable={isPlayer}
                          eventHandlers={{
                            click: () => {
                              if (m.fls_id) {
                                setSelectedFlsId((prev) => prev === m.fls_id ? '' : m.fls_id!)
                                setTeleportFlsId(m.fls_id!)
                              }
                            },
                            dragstart: () => { isDragging.current = true },
                            dragend: (e) => {
                              isDragging.current = false
                              if (!m.fls_id) return
                              const marker = e.target as L.Marker
                              const { lat, lng } = marker.getLatLng()
                              marker.setLatLng(center)
                              const { x, y } = latLngToWorld(lat, lng, effCfg)
                              setDragConfirm({
                                flsId: m.fls_id!,
                                name: m.name || m.fls_id!,
                                x: Math.round(x),
                                y: Math.round(y),
                              })
                            },
                          }}
                        >
                          <Tooltip direction="top" offset={[0, -(size / 2)]}>
                            <div className="font-medium">{m.name || `${m.type} ${m.id}`}</div>
                            <div className="text-xs opacity-70">
                              {m.type}
                              {m.online_status ? ` · ${m.online_status}` : ''}
                            </div>
                            <div className="text-xs font-mono">
                              {Math.round(m.x)}
                              {', '}
                              {Math.round(m.y)}
                            </div>
                            {isPlayer && <div className="text-xs text-accent mt-0.5">Drag to teleport</div>}
                          </Tooltip>
                        </Marker>
                      )
                    })}

                  {filter.bases && orderedLive
                    .filter((m) => m.type === 'base')
                    .map((m) => {
                      const { center, size, icon } = m
                      return (
                        <Marker
                          key={`base-${m.id}`}
                          position={center}
                          icon={icon}
                        >
                          <Tooltip direction="top" offset={[0, -(size / 2)]}>
                            <div className="font-medium">{m.name || `Base ${m.id}`}</div>
                            <div className="text-xs opacity-70">base</div>
                            <div className="text-xs font-mono">
                              {Math.round(m.x)}
                              {', '}
                              {Math.round(m.y)}
                            </div>
                          </Tooltip>
                        </Marker>
                      )
                    })}

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

      <ConfirmDialog
        open={dragConfirm !== null}
        title={t('liveMap.dragTeleportTitle', { name: dragConfirm?.name ?? '' })}
        description={t('liveMap.dragTeleportDesc', { x: dragConfirm?.x ?? 0, y: dragConfirm?.y ?? 0 })}
        confirmLabel={t('liveMap.teleportHere')}
        onConfirm={async () => {
          if (!dragConfirm) return
          try {
            await api.players.teleportCoords(dragConfirm.flsId, dragConfirm.x, dragConfirm.y, 5000)
            toast.success(t('liveMap.teleportSent'))
          }
          catch (err) {
            toast.danger(err instanceof Error ? err.message : String(err))
          }
          setDragConfirm(null)
        }}
        onCancel={() => setDragConfirm(null)}
      />
    </div>
  )
}
