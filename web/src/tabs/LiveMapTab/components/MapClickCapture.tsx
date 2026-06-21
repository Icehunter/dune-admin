import * as React from 'react'
import { useMapEvents } from 'react-leaflet'
import type { MapClickCaptureProps } from '../interfaces'

export const MapClickCapture: React.FC<MapClickCaptureProps> = ({ active, onPick }) => {
  useMapEvents({
    click(e) {
      if (active) onPick(e.latlng.lat, e.latlng.lng)
    },
  })
  return null
}
