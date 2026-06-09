// Type augmentation so vitest's `expect` knows the vitest-axe matchers
// (e.g. `toHaveNoViolations`). jest-dom matchers are augmented by the
// `@testing-library/jest-dom/vitest` import in src/test/setup.ts.
//
// The empty-interface-extends and unused generic param below are required to
// match vitest's `Assertion<T>` signature, so the relevant rules are disabled.
/* eslint-disable @typescript-eslint/no-empty-object-type */
/* eslint-disable @typescript-eslint/no-unused-vars */
/* eslint-disable @typescript-eslint/no-explicit-any */
import 'vitest'
import type { AxeMatchers } from 'vitest-axe/matchers'

declare module 'vitest' {
  interface Assertion<T = any> extends AxeMatchers {}
  interface AsymmetricMatchersContaining extends AxeMatchers {}
}
