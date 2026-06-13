import type { TabId } from './types'

// Tabs that are only meaningful under specific control planes. A tab not listed
// here is unrestricted and always visible (subject to capability checks).
export const TAB_CONTROL_PLANES: Partial<Record<TabId, readonly string[]>> = {
  director: ['amp'],
}

// Whether a tab may be shown for the active control plane.
// - Unrestricted tab → always true.
// - Restricted tab while control is still loading (undefined) → false, to
//   avoid a flash of a tab that will be hidden once status resolves.
// - Restricted tab with a known control → visible only if the control plane
//   is in the tab's allow-list.
export function canSeeTabByControlPlane(key: TabId, control: string | undefined): boolean {
  const restriction = TAB_CONTROL_PLANES[key]
  if (!restriction) return true
  if (control === undefined) return false
  return restriction.includes(control)
}
