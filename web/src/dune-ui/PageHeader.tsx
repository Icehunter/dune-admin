import * as React from 'react'
import { Button, Spinner } from '@heroui/react'
import { Icon } from './Icon'
import type { PageHeaderProps } from './types'

export const PageHeader: React.FC<PageHeaderProps> = ({ title, subtitle, onRefresh, loading, countdown, children }) => {
  return (
    <div className="flex items-start justify-between gap-3 shrink-0 border-b border-border/60 pb-3 mb-1">
      <div className="flex-1 min-w-0">
        <h2 className="text-base font-semibold text-accent truncate">{title}</h2>
        {subtitle && <p className="text-sm text-muted mt-0.5">{subtitle}</p>}
      </div>
      {(onRefresh != null || children) && (
        <div className="flex items-center gap-2 shrink-0">
          {children}
          {onRefresh != null && (
            <Button size="sm" variant="ghost" onPress={onRefresh} isDisabled={loading}>
              {loading
                ? <Spinner size="sm" color="current" />
                : (
                    <>
                      {countdown != null && (
                        <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                          {countdown}
                          s
                        </span>
                      )}
                      <Icon name="refresh-cw" />
                    </>
                  )}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
