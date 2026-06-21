import * as React from 'react'
import { useMap } from 'react-leaflet'
import L from 'leaflet'
import { TILE_CDN } from '../constants'
import type { MapTileLayerProps } from '../interfaces'

export const MapTileLayer: React.FC<MapTileLayerProps> = ({ tileId }) => {
  const map = useMap()

  React.useEffect(() => {
    const layer = new L.TileLayer('', {
      tileSize: 512,
      minZoom: -3,
      maxZoom: 4,
      maxNativeZoom: 1,
      noWrap: true,
      attribution: '',
    })

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(layer as any).getTileUrl = (coords: L.Coords): string => {
      const cdnZ = Math.min(4, Math.max(0, coords.z + 3))
      const scale = Math.pow(2, coords.z + 3 - cdnZ)
      const cdnX = Math.floor(coords.x / scale)
      const cdnY = Math.floor(Math.pow(2, cdnZ) + coords.y / scale)
      const maxTile = Math.pow(2, cdnZ)
      if (cdnX < 0 || cdnX >= maxTile || cdnY < 0 || cdnY >= maxTile) return ''
      return `${TILE_CDN}/${tileId}/${cdnZ}/${cdnY}/${cdnX}.webp`
    }

    layer.addTo(map)
    return () => {
      layer.remove()
    }
  }, [map, tileId])

  return null
}
