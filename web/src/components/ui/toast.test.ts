import { describe, it, expect, vi } from 'vitest'

// Hoisted so the vi.mock factory can reference the spies without a TDZ error.
const { success, error, info, warning } = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { success, error, info, warning } }))

const { toast } = await import('./toast')

describe('toast facade', () => {
  it('maps success → sonner.success', () => {
    toast.success('ok')
    expect(success).toHaveBeenCalledWith('ok', undefined)
  })

  it('maps danger → sonner.error (the key severity rename)', () => {
    toast.danger('bad')
    expect(error).toHaveBeenCalledWith('bad', undefined)
  })

  it('maps info → sonner.info', () => {
    toast.info('fyi')
    expect(info).toHaveBeenCalledWith('fyi', undefined)
  })

  it('maps warning → sonner.warning', () => {
    toast.warning('careful')
    expect(warning).toHaveBeenCalledWith('careful', undefined)
  })

  it('forwards options through to sonner', () => {
    toast.success('ok', { duration: 1000 })
    expect(success).toHaveBeenCalledWith('ok', { duration: 1000 })
  })
})
