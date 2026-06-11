import * as React from 'react'
import { KPI, KPIGroup } from '@heroui-pro/react'
import type { CardProps, ItemProps } from './types'

/**
 * Bordered, slightly-elevated label/value row card — the "Phase Reconciling
 * | Database Ready" health row pattern from BattlegroupTab.
 *
 * Backed by KPIGroup + KPI internally; the InfoCard / InfoCard.Item API is
 * preserved so existing call sites need no changes.
 */
export const InfoCard: React.FC<CardProps> & { Item: React.FC<ItemProps> } = ({ children, className = '' }) => {
  return (
    <KPIGroup className={`flex-wrap ${className}`} orientation="horizontal">
      {children}
    </KPIGroup>
  )
}

export const InfoCardItem: React.FC<ItemProps> = ({ label, value, valueColor }) => {
  return (
    <>
      <KPI>
        <KPI.Header>
          <KPI.Title>{label}</KPI.Title>
        </KPI.Header>
        <KPI.Content>
          <span
            className="text-2xl font-semibold"
            style={valueColor ? { color: valueColor } : undefined}
          >
            {value}
          </span>
        </KPI.Content>
      </KPI>
      <KPIGroup.Separator />
    </>
  )
}

// Namespace alias kept for callers using <InfoCard.Item>
InfoCard.Item = InfoCardItem
