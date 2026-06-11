import * as React from 'react'
import { useMap } from 'react-leaflet'
import { IMAGE_BOUNDS } from '../constants'
import type { InvalidateOnActiveProps } from '../types'

export const InvalidateOnActive: React.FC<InvalidateOnActiveProps> = ({ active }) => {
  const map = useMap()
  React.useEffect(() => {
    if (active) {
      const id = setTimeout(() => {
        map.invalidateSize()
        map.fitBounds(IMAGE_BOUNDS)
      }, 50)
      return () => clearTimeout(id)
    }
  }, [active, map])
  return null
}
