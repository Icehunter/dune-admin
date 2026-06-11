import * as React from 'react'
import { useMap } from 'react-leaflet'
import { IMAGE_BOUNDS } from '../constants'
import type { FitBoundsControllerProps } from '../types'

export const FitBoundsController: React.FC<FitBoundsControllerProps> = ({ fitRef }) => {
  const map = useMap()
  React.useEffect(() => {
    fitRef.current = () => map.fitBounds(IMAGE_BOUNDS, { animate: true })
  }, [map, fitRef])
  return null
}
