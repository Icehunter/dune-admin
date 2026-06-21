import type { Container } from '../types'

export type { Container }

export type AddResult = { given: string[], skipped: { template: string, reason: string }[] } | null
