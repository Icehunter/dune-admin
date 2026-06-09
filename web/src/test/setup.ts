// Vitest global setup: jest-dom matchers (toBeInTheDocument, etc.) and
// vitest-axe matchers (toHaveNoViolations) for accessibility assertions.
import '@testing-library/jest-dom/vitest'
import * as axeMatchers from 'vitest-axe/matchers'
import { expect } from 'vitest'

expect.extend(axeMatchers)

// jsdom lacks ResizeObserver, which cmdk (command palette) and some Radix
// primitives instantiate on mount. Provide a no-op so components render in tests.
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = globalThis.ResizeObserver ?? (ResizeObserverMock as unknown as typeof ResizeObserver)

// jsdom doesn't implement scrollIntoView, which cmdk calls when the active item
// changes. No-op it so the command palette renders/selects in tests.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}
