import { atom } from 'jotai'
import type { UpdateCheckResult } from '../api/client'
import type { UpdatePhase } from '../components/UpdateProgressModal'

// Global Settings modal open/close state.
export const settingsOpenAtom = atom(false)
// Which settings tab to open on (e.g. dashboard onboarding deep-links to
// 'discord' or 'auth'); undefined → the form's default tab.
export const settingsTabAtom = atom<string | undefined>(undefined)

// Add-server wizard modal open state.
export const addServerOpenAtom = atom(false)

// Manage server is a modal (keyed by id in state, not a route) so the URL never
// carries a server id that would look stale after a rename. 0 = none.
export const manageServerIdAtom = atom(0)

// Bumped whenever the global Settings modal closes, so the Dashboard re-syncs
// onboarding state (e.g. the Discord/auth card disappears once configured).
export const dashboardRefreshAtom = atom(0)

// Update flow state.
export const updateInfoAtom = atom<UpdateCheckResult | null>(null)
export const updatePromptOpenAtom = atom(false)
export const updateCheckingAtom = atom(false)
export const updateApplyingAtom = atom(false)
export const updatePhaseAtom = atom<UpdatePhase>('downloading')
export const updateErrorAtom = atom<string | undefined>(undefined)
