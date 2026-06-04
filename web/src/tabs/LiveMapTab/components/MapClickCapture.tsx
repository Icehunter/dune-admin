import { useMapEvents } from 'react-leaflet'
import type { MapClickCaptureProps } from '../types'

export function MapClickCapture({ active, onPick }: MapClickCaptureProps) {
  useMapEvents({
    click(e) {
      if (active) onPick(e.latlng.lat, e.latlng.lng)
    },
  })
  return null
}
