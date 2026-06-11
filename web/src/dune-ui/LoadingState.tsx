import * as React from 'react'
import { Spinner } from '@heroui/react'
import type { LoadingStateProps } from './types'

const PAD: Record<NonNullable<LoadingStateProps['size']>, string> = {
  sm: 'py-4',
  md: 'py-8',
  lg: 'py-12',
}

/**
 * Standard centered loading spinner. Use for full-tab / full-section loads so
 * every tab shows the same loading treatment.
 */
export const LoadingState: React.FC<LoadingStateProps> = ({ size = 'lg', fill = false, className = '' }) => (
  <div className={`flex justify-center ${PAD[size]} ${fill ? 'flex-1' : ''} ${className}`}>
    <Spinner size="lg" />
  </div>
)
