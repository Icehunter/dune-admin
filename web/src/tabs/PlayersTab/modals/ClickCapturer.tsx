import * as React from 'react'
import { useMapEvents } from 'react-leaflet'
import type { ClickCapturerProps } from './interfaces'
import { MAPS } from '../../LiveMapTab/constants'
import { latLngToWorld } from '../../LiveMapTab/utils'

// Hagga Basin is the only map with a teleport coord picker for now.
const HAGGA_CFG = MAPS.find((m) => m.key === 'HaggaBasin')!

// Default Z for picked coordinates — safe height above most Hagga Basin terrain.
const DEFAULT_Z = 5000

export const ClickCapturer: React.FC<ClickCapturerProps> = ({ onPick }) => {
  useMapEvents({
    click(e) {
      const { x, y } = latLngToWorld(e.latlng.lat, e.latlng.lng, HAGGA_CFG)
      onPick(x, y, DEFAULT_Z)
    },
  })
  return null
}
