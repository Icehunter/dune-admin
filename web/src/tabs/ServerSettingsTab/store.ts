import { atomWithStorage } from 'jotai/utils'

export const showAllAtom = atomWithStorage<boolean>('serverSettings.showAll', false)
export const expandedCategoryAtom = atomWithStorage<string | null>('serverSettings.expandedCategory', null)
