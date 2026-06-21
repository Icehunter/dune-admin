import * as React from 'react'
import type { CardArtProps } from './interfaces'

/** Pre-normalized theme card art (1500×2500, edge-to-edge). */
export const CardArt: React.FC<CardArtProps> = ({ folder, file }) => (
  <img
    src={`/theme/${folder}/${file}.svg`}
    alt=""
    draggable={false}
    className="absolute inset-0 w-full h-full select-none object-contain"
  />
)
