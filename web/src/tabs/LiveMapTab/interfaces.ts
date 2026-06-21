import type { MapCfg, SpawnEntry } from './types'

export interface MapClickCaptureProps {
  active: boolean
  onPick: (lat: number, lng: number) => void
}

export interface SpriteIconProps {
  type: string
  size?: number
}

export interface SpawnCanvasLayerProps {
  spawns: SpawnEntry[]
  effCfg: MapCfg
  filter: Record<string, boolean>
  heatmapMode: boolean
}

export interface HeatmapCanvasLayerProps {
  mapKey: string
  effCfg: MapCfg
  filter: Record<string, boolean>
}

export interface MapTileLayerProps {
  tileId: string
}

export interface ZoneGridLayerProps {
  effCfg: MapCfg
}

export interface FitBoundsControllerProps {
  onRegisterFit: (fn: () => void) => void
}

export interface FilterPanelProps {
  filter: Record<string, boolean>
  onToggle: (key: string, currentVisual: boolean) => void
  onClear: () => void
  spawns: SpawnEntry[]
  mapKey: string
  heatmapMode: boolean
  onHeatmapToggle: () => void
}
