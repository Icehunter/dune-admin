import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Modal, Spinner } from '@heroui/react'
import { MapContainer, ImageOverlay, useMapEvents } from 'react-leaflet'
import { CRS, type LatLngBoundsExpression } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { ClickCapturerProps, MapCoordPickerModalProps } from './types'

// Mirror the same image space and map config from LiveMapTab.tsx.
const IMG_W = 1200
const IMG_H = 1200
const IMAGE_BOUNDS: LatLngBoundsExpression = [[0, 0], [IMG_H, IMG_W]]

// Hagga Basin calibration — copied from LiveMapTab MAPS constant.
const HAGGA_BOUNDS = {
  minX: -437871, maxX: 350539,
  minY: -462011, maxY: 376267,
  flipY: true,
} as const

// Inverse of worldToLatLng: Leaflet lat/lng → world X/Y.
// lat = fracYup * IMG_H, lng = fracX * IMG_W
// rawY = flipY ? 1 - fracYup : fracYup  → worldY = rawY * (maxY - minY) + minY
const latLngToWorld = (lat: number, lng: number): { x: number, y: number } => {
  const cfg = HAGGA_BOUNDS
  const fracX = lng / IMG_W
  const fracYup = lat / IMG_H
  const rawX = fracX
  const rawY = cfg.flipY ? 1 - fracYup : fracYup
  const x = rawX * (cfg.maxX - cfg.minX) + cfg.minX
  const y = rawY * (cfg.maxY - cfg.minY) + cfg.minY
  return { x, y }
}

// Default Z for picked coordinates — safe height above most Hagga Basin terrain.
const DEFAULT_Z = 5000

const ClickCapturer: React.FC<ClickCapturerProps> = ({ onPick }) => {
  useMapEvents({
    click(e) {
      const { x, y } = latLngToWorld(e.latlng.lat, e.latlng.lng)
      onPick(x, y, DEFAULT_Z)
    },
  })
  return null
}

export const MapCoordPickerModal: React.FC<MapCoordPickerModalProps> = ({ onPick, onClose }) => {
  const [picked, setPicked] = React.useState<{ x: number, y: number, z: number } | null>(null)
  const [mapReady, setMapReady] = React.useState(false)

  const handleClick = React.useCallback((x: number, y: number, z: number) => {
    setPicked({ x: Math.round(x), y: Math.round(y), z })
  }, [])

  const { t } = useTranslation()

  const confirm = () => {
    if (picked) onPick(picked.x, picked.y, picked.z)
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen onOpenChange={(v) => { if (!v) onClose() }}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10" style={{ height: '85vh', display: 'flex', flexDirection: 'column' }}>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('players.actions.admin.mapPickerModal.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col flex-1 min-h-0 gap-2">
            <p className="text-xs text-muted shrink-0">
              {t('players.actions.admin.mapPickerModal.hint', { z: DEFAULT_Z })}
            </p>
            <div className="relative flex-1 min-h-0 w-full">
              {!mapReady && (
                <div className="absolute inset-0 flex items-center justify-center bg-surface z-10">
                  <Spinner />
                </div>
              )}
              <MapContainer
                crs={CRS.Simple}
                bounds={IMAGE_BOUNDS}
                style={{ width: '100%', height: '100%', background: 'var(--color-surface)', cursor: 'crosshair' } as React.CSSProperties}
                whenReady={() => setMapReady(true)}
              >
                <ImageOverlay url="hagga-basin.png" bounds={IMAGE_BOUNDS} />
                <ClickCapturer onPick={handleClick} />
              </MapContainer>
            </div>
            {picked && (
              <div className="shrink-0 text-sm text-foreground font-mono">
                {t('players.actions.admin.mapPickerModal.selected')}
                {' '}
                <span className="text-accent">
                  X=
                  {picked.x}
                  {' '}
                  Y=
                  {picked.y}
                  {' '}
                  Z=
                  {picked.z}
                </span>
              </div>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="ghost" onPress={onClose}>{t('players.actions.admin.mapPickerModal.cancel')}</Button>
            <Button isDisabled={!picked} onPress={confirm}>{t('players.actions.admin.mapPickerModal.confirm')}</Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
