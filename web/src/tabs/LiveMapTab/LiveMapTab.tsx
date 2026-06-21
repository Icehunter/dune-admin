import * as React from 'react'
import { useAtom } from 'jotai'
import { useTranslation } from 'react-i18next'
import { Button, Select, ListBox, Spinner, toast } from '@heroui/react'
import { MapContainer, ImageOverlay, CircleMarker, Marker, Tooltip } from 'react-leaflet'
import L from 'leaflet'
import { CRS } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { api, ApiError } from '../../api/client'
import type { MapMarker, Player } from '../../api/client'
import { ConfirmDialog, Icon, PageHeader } from '../../dune-ui'
import { PlayerSearchField } from '../../components/PlayerSearchField'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { usePermissions } from '../../hooks/usePermissions'
import { InvalidateOnActive } from './components/InvalidateOnActive'
import { MapClickCapture } from './components/MapClickCapture'
import { SpawnCanvasLayer } from './components/SpawnCanvasLayer'
import { HeatmapCanvasLayer } from './components/HeatmapCanvasLayer'
import { MapTileLayer } from './components/MapTileLayer'
import { ZoneGridLayer } from './components/ZoneGridLayer'
import { FitBoundsController } from './components/FitBoundsController'
import { FilterPanel } from './components/FilterPanel'
import {
  MAPS, CAT_COLOR, IMAGE_BOUNDS, POLL_MS, IMG_H, IMG_W, CALIB_MIN_WORLD_DIST,
} from './constants'
import {
  worldToLatLng, latLngToWorld, solveBounds, liveFilterAtom, mapUrl,
} from './utils'
import type { SpawnEntry, SpawnFile, CalibPoint, MapCfg, Bounds } from './types'

export const LiveMapTab: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canPlayersWrite = can('players:write')
  const [mapKey, setMapKey] = React.useState<string>('HaggaBasin')
  const [markers, setMarkers] = React.useState<MapMarker[]>([])
  const [loading, setLoading] = React.useState(false)
  const [unsupported, setUnsupported] = React.useState(false)
  const [updatedLabel, setUpdatedLabel] = React.useState<string>('')
  const [calibrating, setCalibrating] = React.useState(false)
  const [calibPoints, setCalibPoints] = React.useState<CalibPoint[]>([])
  const [calibOverride, setCalibOverride] = React.useState<Record<string, Bounds>>({})
  // Which player marker is the operator anchoring against. Empty = auto (single player).
  const [calibPlayerId, setCalibPlayerId] = React.useState<string>('')
  const [calibSaving, setCalibSaving] = React.useState(false)
  const [calibDirty, setCalibDirty] = React.useState(false)

  const [spawns, setSpawns] = React.useState<SpawnEntry[]>([])
  const loadedSpawnKey = React.useRef<string>('')
  const isDragging = React.useRef(false)

  const [filter, setFilter] = useAtom(liveFilterAtom)
  const [selectedFlsId, setSelectedFlsId] = React.useState<string>('')
  const [dragConfirm, setDragConfirm] = React.useState<{
    flsId: string
    name: string
    x: number
    y: number
  } | null>(null)

  const [heatmapMode, setHeatmapMode] = React.useState(false)
  const fitBoundsRef = React.useRef<(() => void) | null>(null)
  const registerFitBounds = (fn: () => void): void => {
    fitBoundsRef.current = fn
  }
  const [teleportMode, setTeleportMode] = React.useState(false)
  const [teleportDest, setTeleportDest] = React.useState<{ x: number, y: number } | null>(null)
  const [teleportFlsId, setTeleportFlsId] = React.useState<string>('')
  const [allPlayers, setAllPlayers] = React.useState<Player[]>([])
  const [teleporting, setTeleporting] = React.useState(false)

  const baseCfg = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]
  // Live preview of the solved transform from the current points (unsaved).
  const previewBounds = React.useMemo(() => solveBounds(calibPoints), [calibPoints])
  const effCfg: MapCfg = React.useMemo(
    () => {
      const override = (calibrating && previewBounds) ? previewBounds : calibOverride[mapKey]
      return { ...baseCfg, ...(override ?? {}) }
    },
    [baseCfg, calibrating, previewBounds, calibOverride, mapKey],
  )

  const load = (key: string): void => {
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
  }

  const loadCurrent = (): void => load(mapKey)
  React.useEffect(() => {
    const id = setTimeout(loadCurrent, 0)
    return () => clearTimeout(id)
  }, [mapKey]) // eslint-disable-line react-hooks/exhaustive-deps
  const { countdown, refresh } = useAutoRefresh(loadCurrent, POLL_MS)

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

  // Load per-map calibration from the backend when the map changes.
  React.useEffect(() => {
    api.map.calibration.get(mapKey)
      .then((c) => {
        setCalibOverride((prev) => ({
          ...prev,
          [mapKey]: { minX: c.min_x, maxX: c.max_x, minY: c.min_y, maxY: c.max_y, flipX: c.flip_x, flipY: c.flip_y },
        }))
      })
      .catch(() => { /* 404 or unavailable = no saved calibration, use constants */ })
  }, [mapKey])

  const playerCount = markers.filter((m) => m.type === 'player').length
  const vehicleCount = markers.filter((m) => m.type === 'vehicle').length
  const baseCount = markers.filter((m) => m.type === 'base').length

  const playerMarkers = markers.filter((m) => m.type === 'player')

  // The anchor is the explicitly selected player, or the only online player.
  const calibAnchor = calibPlayerId
    ? (playerMarkers.find((m) => String(m.id) === calibPlayerId) ?? null)
    : (playerMarkers.length === 1 ? playerMarkers[0] : null)

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

  const handleMapClick = (lat: number, lng: number): void => {
    if (calibrating) {
      if (playerMarkers.length === 0) {
        toast.danger(t('liveMap.calibNoPlayer'))
        return
      }
      if (!calibAnchor) {
        toast.danger(t('liveMap.calibPickPlayer'))
        return
      }
      // Snapshot the anchor's world coord at click time.
      const wx = calibAnchor.x
      const wy = calibAnchor.y
      const tooClose = calibPoints.some(
        (p) => Math.hypot(p.wx - wx, p.wy - wy) < CALIB_MIN_WORLD_DIST,
      )
      if (tooClose) {
        toast.danger(t('liveMap.calibTooClose'))
        return
      }
      setCalibPoints((prev) => [...prev, { wx, wy, fracX: lng / IMG_W, fracYup: lat / IMG_H }])
      setCalibDirty(true)
      return
    }
    if (teleportMode) {
      const { x, y } = latLngToWorld(lat, lng, effCfg)
      setTeleportDest({ x: Math.round(x), y: Math.round(y) })
    }
  }

  const removeCalibPoint = (idx: number): void => {
    setCalibPoints((prev) => prev.filter((_, i) => i !== idx))
    setCalibDirty(true)
  }

  const removeLastCalibPoint = (): void => {
    setCalibPoints((prev) => prev.slice(0, -1))
    setCalibDirty(true)
  }

  const clearCalib = (): void => {
    setCalibPoints([])
    setCalibDirty(false)
    setCalibOverride((c) => {
      const merged = { ...c }
      delete merged[mapKey]
      return merged
    })
  }

  const saveCalib = async (): Promise<void> => {
    if (!previewBounds) {
      toast.danger(t('liveMap.calibCannotSolve'))
      return
    }
    setCalibSaving(true)
    try {
      await api.map.calibration.save(mapKey, {
        min_x: previewBounds.minX, max_x: previewBounds.maxX,
        min_y: previewBounds.minY, max_y: previewBounds.maxY,
        flip_x: !!previewBounds.flipX, flip_y: !!previewBounds.flipY,
      })
      setCalibOverride((c) => ({ ...c, [mapKey]: previewBounds }))
      setCalibDirty(false)
      toast.success(t('liveMap.calibSaved'))
    }
    catch (e: unknown) {
      toast.danger(t('liveMap.calibSaveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setCalibSaving(false)
    }
  }

  // Reset removes the saved calibration so the map reverts to its built-in
  // default bounds (from constants), and clears any in-progress points.
  const resetCalib = async (): Promise<void> => {
    setCalibSaving(true)
    try {
      await api.map.calibration.remove(mapKey)
      setCalibPoints([])
      setCalibDirty(false)
      setCalibOverride((c) => {
        const merged = { ...c }
        delete merged[mapKey]
        return merged
      })
      toast.success(t('liveMap.calibReset'))
    }
    catch (e: unknown) {
      toast.danger(t('liveMap.calibResetFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setCalibSaving(false)
    }
  }

  const _solvedB = previewBounds ?? calibOverride[mapKey]
  const solvedStr = _solvedB
    ? `minX: ${Math.round(_solvedB.minX)}, maxX: ${Math.round(_solvedB.maxX)}, minY: ${Math.round(_solvedB.minY)}, maxY: ${Math.round(_solvedB.maxY)}, flipY: ${!!_solvedB.flipY}`
    : ''

  const doTeleport = async (): Promise<void> => {
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
  }

  const toggleFilter = (key: string, currentVisual: boolean): void => {
    setFilter((f) => {
      return { ...f, [key]: !currentVisual }
    })
  }

  const clearFilters = (): void => {
    setFilter((f) => {
      const next: Record<string, boolean> = {}
      Object.keys(f).forEach((k) => {
        next[k] = false
      })
      Object.assign(next, { players: true, vehicles: true, bases: true })
      return next
    })
  }

  const mapCursor = calibrating || teleportMode ? 'crosshair' : 'grab'
  const currentMap = MAPS.find((m) => m.key === mapKey) ?? MAPS[0]

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">

      <PageHeader title={t('liveMap.title')} subtitle={t('liveMap.subtitle')}>
        <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <React.Fragment>
                  {currentMap.hasLiveData && (
                    <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                      {countdown}
                      s
                    </span>
                  )}
                  <Icon name="refresh-cw" />
                </React.Fragment>
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

        {canPlayersWrite && (
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
        )}
        <Button
          size="sm"
          variant={calibrating ? 'primary' : 'outline'}
          onPress={() => {
            setCalibrating((v) => {
              if (v) {
                setCalibPlayerId('')
                setCalibDirty(false)
              }
              return !v
            })
          }}
        >
          <Icon name="crosshair" />
          {' '}
          {t('liveMap.calibrate')}
        </Button>
      </div>

      <div className="flex flex-wrap gap-4 shrink-0 text-xs text-muted">
        {currentMap.hasLiveData && (
          <React.Fragment>
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
          </React.Fragment>
        )}
        {spawns.length > 0 && <span>{t('liveMap.spawnsLoaded', { count: spawns.length })}</span>}
        {updatedLabel !== '' && <span className="ml-auto">{t('liveMap.updated', { time: updatedLabel })}</span>}
      </div>

      {canPlayersWrite && teleportMode && (
        <div className="shrink-0 rounded-[var(--radius)] border border-accent/40 bg-surface px-3 py-2 text-xs flex flex-wrap items-center gap-3">
          <div className="text-accent font-medium">
            <Icon name="navigation" className="size-3 inline mr-1" />
            {teleportDest
              ? t('liveMap.spawnTooltipCoords', { x: teleportDest.x, y: teleportDest.y })
              : t('liveMap.teleportModeActive')}
          </div>
          {teleportDest && (
            <React.Fragment>
              <PlayerSearchField
                ariaLabel={t('liveMap.teleportPlayer')}
                placeholder={t('liveMap.teleportSelectPlayer')}
                players={allPlayers}
                onSelect={(p) => setTeleportFlsId(p.fls_id)}
                onClear={() => setTeleportFlsId('')}
                className="w-56"
              />
              <Button size="sm" isDisabled={!teleportFlsId || teleporting} onPress={doTeleport}>
                {teleporting ? <Spinner size="sm" color="current" /> : t('liveMap.teleportHere')}
              </Button>
              <Button size="sm" variant="ghost" onPress={() => setTeleportDest(null)}>✕</Button>
            </React.Fragment>
          )}
        </div>
      )}

      {calibrating && (
        <div className="shrink-0 rounded-[var(--radius)] border border-border bg-surface px-3 py-2 text-xs flex flex-col gap-2">
          <div className="text-accent">{t('liveMap.calibActive')}</div>

          {playerMarkers.length === 0 && (
            <div className="text-danger">{t('liveMap.calibNoPlayer')}</div>
          )}

          {playerMarkers.length > 1 && (
            <div className="flex items-center gap-2">
              <span className="text-muted shrink-0">{t('liveMap.calibAnchorLabel')}</span>
              <Select
                aria-label={t('liveMap.calibAnchorLabel')}
                selectedKey={calibPlayerId}
                onSelectionChange={(k) => setCalibPlayerId(String(k))}
                className="w-56"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {playerMarkers.map((p) => (
                      <ListBox.Item key={p.id} id={String(p.id)} textValue={p.name}>
                        {p.name}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            </div>
          )}

          {playerMarkers.length === 1 && (
            <div className="text-muted">
              {t('liveMap.calibAnchorSingle', { name: playerMarkers[0].name })}
            </div>
          )}

          <div className="text-muted">{t('liveMap.calibPoints', { n: calibPoints.length })}</div>

          {calibPoints.length > 0 && (
            <div className="flex flex-col gap-1">
              {calibPoints.map((p, i) => (
                <div key={`cp-${i}`} className="flex items-center gap-2 font-mono">
                  <span className="text-accent">{`${i + 1}.`}</span>
                  <span className="text-foreground">
                    {`wx ${Math.round(p.wx)}, wy ${Math.round(p.wy)}`}
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="ml-auto"
                    onPress={() => removeCalibPoint(i)}
                  >
                    ✕
                  </Button>
                </div>
              ))}
            </div>
          )}

          {solvedStr && <div className="font-mono text-foreground break-all">{solvedStr}</div>}
          {calibPoints.length >= 2 && !previewBounds && (
            <div className="text-danger">{t('liveMap.calibCannotSolve')}</div>
          )}

          <div className="flex flex-wrap items-center gap-2 pt-1">
            <Button
              size="sm"
              variant="primary"
              isDisabled={!previewBounds || !calibDirty || calibSaving}
              onPress={saveCalib}
            >
              {calibSaving ? <Spinner size="sm" color="current" /> : t('liveMap.calibSave')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              isDisabled={calibPoints.length === 0 || calibSaving}
              onPress={removeLastCalibPoint}
            >
              {t('liveMap.calibRemoveLast')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              isDisabled={calibPoints.length === 0 || calibSaving}
              onPress={clearCalib}
            >
              {t('liveMap.clear')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="ml-auto text-danger"
              isDisabled={calibSaving}
              onPress={resetCalib}
            >
              {t('liveMap.calibReset')}
            </Button>
          </div>
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
                  <InvalidateOnActive />
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

                  <FitBoundsController onRegisterFit={registerFitBounds} />

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
                          draggable={canPlayersWrite && isPlayer}
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
                            {canPlayersWrite && isPlayer && <div className="text-xs text-accent mt-0.5">Drag to teleport</div>}
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
