import { describe, it, expect } from 'vitest'
import { canSeeTabByControlPlane } from './tabNav'

describe('canSeeTabByControlPlane', () => {
  describe('director (AMP-only)', () => {
    it('is shown under the amp control plane', () => {
      expect(canSeeTabByControlPlane('director', 'amp')).toBe(true)
    })

    it('is hidden under kubectl', () => {
      expect(canSeeTabByControlPlane('director', 'kubectl')).toBe(false)
    })

    it('is hidden under docker', () => {
      expect(canSeeTabByControlPlane('director', 'docker')).toBe(false)
    })

    it('is hidden under local', () => {
      expect(canSeeTabByControlPlane('director', 'local')).toBe(false)
    })

    it('is hidden under none', () => {
      expect(canSeeTabByControlPlane('director', 'none')).toBe(false)
    })

    it('is hidden while control is still loading (undefined)', () => {
      expect(canSeeTabByControlPlane('director', undefined)).toBe(false)
    })

    it('is hidden under an unknown control plane', () => {
      expect(canSeeTabByControlPlane('director', 'something-else')).toBe(false)
    })
  })

  describe('unrestricted tabs', () => {
    it('are always visible regardless of control plane', () => {
      expect(canSeeTabByControlPlane('players', 'amp')).toBe(true)
      expect(canSeeTabByControlPlane('players', 'kubectl')).toBe(true)
      expect(canSeeTabByControlPlane('players', 'local')).toBe(true)
      expect(canSeeTabByControlPlane('players', 'none')).toBe(true)
      expect(canSeeTabByControlPlane('players', undefined)).toBe(true)
    })
  })
})
