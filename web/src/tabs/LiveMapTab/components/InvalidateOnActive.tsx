import { useEffect } from 'react'
import { useMap } from 'react-leaflet'
import { IMAGE_BOUNDS } from '../constants'
import type { InvalidateOnActiveProps } from '../types'

export function InvalidateOnActive({ active }: InvalidateOnActiveProps) {
  const map = useMap()
  useEffect(() => {
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
