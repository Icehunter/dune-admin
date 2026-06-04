import { useEffect } from 'react'
import { useMap } from 'react-leaflet'
import { IMAGE_BOUNDS } from '../constants'
import type { FitBoundsControllerProps } from '../types'

export function FitBoundsController({ fitRef }: FitBoundsControllerProps) {
  const map = useMap()
  useEffect(() => {
    fitRef.current = () => map.fitBounds(IMAGE_BOUNDS, { animate: true })
  }, [map, fitRef])
  return null
}
