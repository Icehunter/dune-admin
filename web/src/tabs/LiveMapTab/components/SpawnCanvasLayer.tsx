import * as React from 'react'
import { useMap } from 'react-leaflet'
import { ICON_POS, CAT_COLOR, SPRITE_CELL, SPRITE_URL } from '../constants'
import { worldToLatLng, filterKey } from '../utils'
import type { SpawnCanvasLayerProps } from '../types'

export const SpawnCanvasLayer: React.FC<SpawnCanvasLayerProps> = ({
  spawns, effCfg, filter, heatmapMode,
}) => {
  const map = useMap()
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null)
  const spriteRef = React.useRef<HTMLImageElement | null>(null)
  const spriteReady = React.useRef(false)

  const visible = React.useMemo(
    () => spawns.filter((s) => {
      if (!(filter[filterKey(s.type)] ?? false)) return false
      if (heatmapMode && (s.category === 'resources' || s.category === 'hazards')) return false
      return true
    }),
    [spawns, filter, heatmapMode],
  )

  const draw = React.useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const mapSize = map.getSize()
    canvas.width = mapSize.x
    canvas.height = mapSize.y
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, mapSize.x, mapSize.y)

    const sprite = spriteRef.current

    for (const s of visible) {
      const isDense = s.category === 'resources' || s.category === 'static'

      const [lat, lng] = worldToLatLng(s.x, s.y, effCfg)
      const pt = map.latLngToContainerPoint([lat, lng])

      if (pt.x < -32 || pt.x > mapSize.x + 32 || pt.y < -32 || pt.y > mapSize.y + 32) continue

      const typeKey = filterKey(s.type)
      const pos = ICON_POS[typeKey]
      const iconSize = isDense ? 20 : 28

      if (sprite && spriteReady.current && pos) {
        const [col, row] = pos
        ctx.drawImage(
          sprite,
          col * SPRITE_CELL, row * SPRITE_CELL,
          SPRITE_CELL, SPRITE_CELL,
          pt.x - iconSize / 2, pt.y - iconSize / 2,
          iconSize, iconSize,
        )
      }
      else {
        ctx.beginPath()
        ctx.arc(pt.x, pt.y, isDense ? 3 : 5, 0, Math.PI * 2)
        ctx.fillStyle = CAT_COLOR[s.category] ?? '#888'
        ctx.globalAlpha = 0.65
        ctx.fill()
        ctx.globalAlpha = 1
      }
    }
  }, [map, visible, effCfg])

  React.useEffect(() => {
    const container = map.getContainer()
    const canvas = document.createElement('canvas')
    canvas.style.cssText = 'position:absolute;left:0;top:0;pointer-events:none;z-index:499'
    container.appendChild(canvas)
    canvasRef.current = canvas

    const img = new Image()
    img.src = SPRITE_URL
    img.onload = () => {
      spriteRef.current = img
      spriteReady.current = true
      draw()
    }

    return () => {
      canvas.remove()
      canvasRef.current = null
    }
  }, [map]) // eslint-disable-line react-hooks/exhaustive-deps

  React.useEffect(() => {
    map.on('move zoom moveend zoomend viewreset resize', draw)
    draw()
    return () => {
      map.off('move zoom moveend zoomend viewreset resize', draw)
    }
  }, [map, draw])

  return null
}
