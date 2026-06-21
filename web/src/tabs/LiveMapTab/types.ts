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
