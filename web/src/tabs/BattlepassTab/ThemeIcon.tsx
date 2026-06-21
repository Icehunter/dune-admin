import * as React from 'react'
import type { ThemeIconProps } from './interfaces'

const THEME_ICON_PREFIX: Record<string, string> = {
  spice: 'spice',
  harkonnen: 'harko',
  fremen: 'fremen',
  atredies: 'atreides',
}

/** Small theme-aware icon from the named SVG set (e.g. intel_token, reward, level). */
export const ThemeIcon: React.FC<ThemeIconProps> = ({ folder, name, className }) => {
  const prefix = THEME_ICON_PREFIX[folder] ?? folder
  return (
    <img
      src={`/theme/${folder}/${prefix}_${name}.svg`}
      alt=""
      draggable={false}
      className={className ?? 'size-3'}
    />
  )
}
