export type Bounds = {
  minX: number
  maxX: number
  minY: number
  maxY: number
  flipX?: boolean
  flipY?: boolean
}

export type MapCfg = Bounds & {
  key: string
  label: string
  image?: string
  spawnFile?: string
  hasLiveData?: boolean
  tileId?: string
  depthFile?: string
}

export type CalibPoint = {
  wx: number
  wy: number
  fracX: number
  fracYup: number
}

export type SpawnEntry = {
  type: string
  label?: string
  category: string
  x: number
  y: number
  z?: number
  density?: number
}

export type SpawnFile = {
  spawns: SpawnEntry[]
}

export interface InvalidateOnActiveProps {
  active: boolean
}

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
  fitRef: React.MutableRefObject<(() => void) | null>
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

export interface LiveMapTabProps {
  isActive?: boolean
}
