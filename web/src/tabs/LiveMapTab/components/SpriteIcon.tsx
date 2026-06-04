import { ICON_POS, SPRITE_URL, SPRITE_COLS, SPRITE_ROWS, SPRITE_CELL } from '../constants'
import type { SpriteIconProps } from '../types'

export function SpriteIcon({ type, size = 22 }: SpriteIconProps) {
  const pos = ICON_POS[type]
  if (!pos) return null
  const [col, row] = pos
  const scale = size / SPRITE_CELL
  const bw = SPRITE_COLS * SPRITE_CELL * scale
  const bh = SPRITE_ROWS * SPRITE_CELL * scale
  const bx = -(col * SPRITE_CELL * scale)
  const by = -(row * SPRITE_CELL * scale)
  return (
    <span
      className="inline-block shrink-0"
      style={{
        width: size,
        height: size,
        backgroundImage: `url(${SPRITE_URL})`,
        backgroundPosition: `${bx}px ${by}px`,
        backgroundSize: `${bw}px ${bh}px`,
        backgroundRepeat: 'no-repeat',
        imageRendering: 'pixelated',
      }}
    />
  )
}
