import * as React from 'react'
import { Chip } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { VehicleOwnerCellProps } from './interfaces'

/**
 * Owner cell.
 *
 * Vehicles reachable through an access grant used to render identically to
 * owned ones, because the query filtered on the viewer's player id without
 * reading their rank (#313). Rank 1 is the owner; anything above it is a grant.
 */
export const VehicleOwnerCell: React.FC<VehicleOwnerCellProps> = ({ vehicle }): React.ReactElement => {
  const { t } = useTranslation()
  const name = vehicle.owner_name || t('players.vehicles.unknownOwner')
  // Key off access_label, not !is_owner: an orphaned backup row carries rank 0
  // and no owner, and labelling that "Granted" would be a claim about access
  // that nothing supports.
  if (vehicle.access_label !== 'granted') {
    return <span className="text-muted">{name}</span>
  }
  return (
    <div className="flex items-center gap-1">
      <span className="text-muted">{name}</span>
      <Chip size="sm" color="default" variant="soft">{t('players.vehicles.granted')}</Chip>
    </div>
  )
}
