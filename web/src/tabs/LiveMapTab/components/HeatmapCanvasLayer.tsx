import * as React from 'react'
import { useMap } from 'react-leaflet'
import { HEATMAP_BOUNDS, HEATMAP_PREFIX, HEATMAP_TYPES } from '../constants'
import { worldToLatLng, heatmapFilterKey, mapUrl } from '../utils'
import type { HeatmapCanvasLayerProps } from '../interfaces'

export const HeatmapCanvasLayer: React.FC<HeatmapCanvasLayerProps> = ({
  mapKey, effCfg, filter,
}) => {
  const map = useMap()
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null)
  const imageCache = React.useRef(new Map<string, HTMLImageElement | null>())
  const pendingRef = React.useRef(new Set<string>())

  const bounds = HEATMAP_BOUNDS[mapKey]
  const prefix = HEATMAP_PREFIX[mapKey]
  const types = React.useMemo(() => HEATMAP_TYPES[mapKey] ?? [], [mapKey])

  const draw = React.useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas || !bounds) return
    const mapSize = map.getSize()
    canvas.width = mapSize.x
    canvas.height = mapSize.y
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, mapSize.x, mapSize.y)

    const [tlLat, tlLng] = worldToLatLng(bounds.minX, bounds.maxY, effCfg)
    const [brLat, brLng] = worldToLatLng(bounds.maxX, bounds.minY, effCfg)
    const tl = map.latLngToContainerPoint([tlLat, tlLng])
    const br = map.latLngToContainerPoint([brLat, brLng])
    const dw = br.x - tl.x
    const dh = br.y - tl.y

    ctx.globalAlpha = 0.65
    for (const type of types) {
      if (!(filter[heatmapFilterKey(type)] ?? false)) continue
      const img = imageCache.current.get(type)
      if (img) ctx.drawImage(img, tl.x, tl.y, dw, dh)
    }
    ctx.globalAlpha = 1
  }, [map, bounds, effCfg, filter, types])

  React.useEffect(() => {
    if (!prefix) return
    for (const type of types) {
      if (!(filter[heatmapFilterKey(type)] ?? false)) continue
      if (imageCache.current.has(type) || pendingRef.current.has(type)) continue
      pendingRef.current.add(type)
      const img = new Image()
      img.onload = () => {
        imageCache.current.set(type, img)
        pendingRef.current.delete(type)
        draw()
      }
      img.onerror = () => {
        imageCache.current.set(type, null)
        pendingRef.current.delete(type)
      }
      img.src = mapUrl(`map-data/${prefix}-heatmap-${type}.png`)
    }
  }, [filter, types, prefix, draw])

  React.useEffect(() => {
    const container = map.getContainer()
    const canvas = document.createElement('canvas')
    canvas.style.cssText = 'position:absolute;left:0;top:0;pointer-events:none;z-index:498'
    container.appendChild(canvas)
    canvasRef.current = canvas
    return () => {
      canvas.remove()
      canvasRef.current = null
    }
  }, [map])

  React.useEffect(() => {
    map.on('move zoom moveend zoomend viewreset resize', draw)
    draw()
    return () => {
      map.off('move zoom moveend zoomend viewreset resize', draw)
    }
  }, [map, draw])

  return null
}
