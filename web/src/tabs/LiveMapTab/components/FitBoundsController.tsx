import * as React from 'react'
import { useMap } from 'react-leaflet'
import { IMAGE_BOUNDS } from '../constants'
import type { FitBoundsControllerProps } from '../interfaces'

export const FitBoundsController: React.FC<FitBoundsControllerProps> = ({ onRegisterFit }) => {
  const map = useMap()
  React.useEffect(() => {
    onRegisterFit(() => map.fitBounds(IMAGE_BOUNDS, { animate: true }))
  }, [onRegisterFit, map])
  return null
}
