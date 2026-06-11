import * as React from 'react'
import { Time } from '@internationalized/date'
import { TimeField, ToggleButton, ToggleButtonGroup } from '@heroui/react'
import type { TimeInputProps } from './types'

const parseHHMM = (s: string): Time | null => {
  const parts = s.split(':')
  const h = Number(parts[0])
  const m = Number(parts[1])
  if (Number.isNaN(h) || Number.isNaN(m)) return null
  return new Time(h, m)
}

const toHHMM = (t: Time): string => {
  return `${String(t.hour).padStart(2, '0')}:${String(t.minute).padStart(2, '0')}`
}

export const TimeInput: React.FC<TimeInputProps> = ({ value, onChange, ariaLabel, className, isDisabled }) => {
  const timeValue = parseHHMM(value)
  const isAM = timeValue ? timeValue.hour < 12 : true

  const handleTimeChange = (t: Time | null) => {
    if (t) onChange(toHHMM(t))
  }

  const handlePeriodChange = (keys: Iterable<React.Key> | 'all') => {
    if (!timeValue) return
    const period = keys === 'all' ? null : [...keys][0]
    if (!period) return
    let { hour } = timeValue
    const { minute } = timeValue
    if (period === 'pm' && hour < 12) hour += 12
    else if (period === 'am' && hour >= 12) hour -= 12
    onChange(toHHMM(new Time(hour, minute)))
  }

  return (
    <div className={`flex items-center gap-1 ${className ?? ''}`}>
      <TimeField
        value={timeValue}
        onChange={handleTimeChange}
        hourCycle={12}
        granularity="minute"
        aria-label={ariaLabel}
        isDisabled={isDisabled}
      >
        <TimeField.Group variant="secondary">
          <TimeField.Input>
            {(segment) => (
              <TimeField.Segment
                segment={segment}
                className={segment.type === 'dayPeriod' ? 'hidden' : ''}
              />
            )}
          </TimeField.Input>
        </TimeField.Group>
      </TimeField>
      <ToggleButtonGroup
        selectionMode="single"
        disallowEmptySelection
        selectedKeys={[isAM ? 'am' : 'pm']}
        onSelectionChange={handlePeriodChange}
        size="sm"
        isDisabled={isDisabled}
      >
        <ToggleButton id="am">AM</ToggleButton>
        <ToggleButton id="pm">PM</ToggleButton>
      </ToggleButtonGroup>
    </div>
  )
}
