import { useCallback, useEffect, useRef } from 'react'
import { useMap } from 'react-leaflet'
import { DD_COLS, DD_ROWS } from '../constants'
import { worldToLatLng } from '../utils'
import type { ZoneGridLayerProps } from '../types'

export function ZoneGridLayer({ effCfg }: ZoneGridLayerProps) {
  const map = useMap()
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const mapSize = map.getSize()
    canvas.width = mapSize.x
    canvas.height = mapSize.y
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, mapSize.x, mapSize.y)

    const b = { minX: effCfg.minX, maxX: effCfg.maxX, minY: effCfg.minY, maxY: effCfg.maxY }
    const cellW = (b.maxX - b.minX) / 9
    const cellH = (b.maxY - b.minY) / 9

    ctx.strokeStyle = 'rgba(255,255,255,0.25)'
    ctx.lineWidth = 1
    ctx.fillStyle = 'rgba(255,255,255,0.45)'
    ctx.font = '11px sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'

    for (let ci = 0; ci <= 9; ci++) {
      const x = b.minX + ci * cellW
      const [latB, lngB] = worldToLatLng(x, b.minY, effCfg)
      const [latT, lngT] = worldToLatLng(x, b.maxY, effCfg)
      const ptB = map.latLngToContainerPoint([latB, lngB])
      const ptT = map.latLngToContainerPoint([latT, lngT])
      ctx.beginPath()
      ctx.moveTo(ptB.x, ptB.y)
      ctx.lineTo(ptT.x, ptT.y)
      ctx.stroke()
    }
    for (let ri = 0; ri <= 9; ri++) {
      const y = b.minY + ri * cellH
      const [latL, lngL] = worldToLatLng(b.minX, y, effCfg)
      const [latR, lngR] = worldToLatLng(b.maxX, y, effCfg)
      const ptL = map.latLngToContainerPoint([latL, lngL])
      const ptR = map.latLngToContainerPoint([latR, lngR])
      ctx.beginPath()
      ctx.moveTo(ptL.x, ptL.y)
      ctx.lineTo(ptR.x, ptR.y)
      ctx.stroke()
    }

    for (let ci = 0; ci < 9; ci++) {
      for (let ri = 0; ri < 9; ri++) {
        const cx = b.minX + (ci + 0.5) * cellW
        const cy = b.minY + (ri + 0.5) * cellH
        const [lat, lng] = worldToLatLng(cx, cy, effCfg)
        const pt = map.latLngToContainerPoint([lat, lng])
        if (pt.x < -20 || pt.x > mapSize.x + 20 || pt.y < -20 || pt.y > mapSize.y + 20) continue
        const label = `${DD_ROWS[ri]}${DD_COLS[ci]}`
        ctx.fillText(label, pt.x, pt.y)
      }
    }
  }, [map, effCfg])

  useEffect(() => {
    const container = map.getContainer()
    const canvas = document.createElement('canvas')
    canvas.style.cssText = 'position:absolute;left:0;top:0;pointer-events:none;z-index:497'
    container.appendChild(canvas)
    canvasRef.current = canvas
    return () => {
      canvas.remove()
      canvasRef.current = null
    }
  }, [map])

  useEffect(() => {
    map.on('move zoom moveend zoomend viewreset resize', draw)
    draw()
    return () => {
      map.off('move zoom moveend zoomend viewreset resize', draw)
    }
  }, [map, draw])

  return null
}
