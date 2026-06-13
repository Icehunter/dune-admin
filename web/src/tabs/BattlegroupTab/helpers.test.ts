import { describe, it, expect } from 'vitest'
import { phaseColor, phaseChipColor } from './helpers'

describe('phaseColor', () => {
  it('treats reconciling as online/healthy (success/green)', () => {
    expect(phaseColor('reconciling')).toBe('var(--success)')
    expect(phaseColor('Reconciling')).toBe('var(--success)')
  })

  it('keeps running as success/green', () => {
    expect(phaseColor('running')).toBe('var(--success)')
  })

  it('keeps starting and initializing as warning', () => {
    expect(phaseColor('starting')).toBe('var(--warning)')
    expect(phaseColor('initializing')).toBe('var(--warning)')
  })
})

describe('phaseChipColor', () => {
  it('treats reconciling as online/healthy (success)', () => {
    expect(phaseChipColor('reconciling')).toBe('success')
    expect(phaseChipColor('Reconciling')).toBe('success')
  })

  it('keeps running as success', () => {
    expect(phaseChipColor('running')).toBe('success')
  })

  it('keeps starting and initializing as warning', () => {
    expect(phaseChipColor('starting')).toBe('warning')
    expect(phaseChipColor('initializing')).toBe('warning')
  })
})
