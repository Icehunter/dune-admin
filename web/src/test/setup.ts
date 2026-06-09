// Vitest global setup: jest-dom matchers (toBeInTheDocument, etc.) and
// vitest-axe matchers (toHaveNoViolations) for accessibility assertions.
import '@testing-library/jest-dom/vitest'
import * as axeMatchers from 'vitest-axe/matchers'
import { expect } from 'vitest'

expect.extend(axeMatchers)
