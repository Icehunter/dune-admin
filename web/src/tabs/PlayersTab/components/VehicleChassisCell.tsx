import * as React from 'react'
import type { VehicleChassisCellProps } from './interfaces'

/**
 * Chassis durability cell.
 *
 * The game only records a maximum durability once a part has decayed, so most
 * vehicles have no denominator and no percentage can be derived. The absolute
 * value is always shown; the percentage is appended only when it is real.
 *
 * The column this replaces read COALESCE(recovered_vehicles.chassis_durability,
 * 1.0), and recovered_vehicles is empty on a normal server — so every vehicle
 * displayed "100%" (#313).
 */
export const VehicleChassisCell: React.FC<VehicleChassisCellProps> = ({ vehicle }): React.ReactElement => {
  // A zero reading is only "no data" when there is no maximum either. With a
  // maximum present, 0 is a real destroyed chassis and must not be hidden.
  if (vehicle.chassis_current <= 0 && !vehicle.has_chassis_pct) {
    return <span className="text-muted">—</span>
  }
  const worn = vehicle.has_chassis_pct && vehicle.chassis_pct < 30
  return (
    <span className={worn ? 'text-danger' : 'text-muted'}>
      {Math.round(vehicle.chassis_current).toLocaleString()}
      {vehicle.has_chassis_pct ? ` (${Math.round(vehicle.chassis_pct)}%)` : ''}
    </span>
  )
}
