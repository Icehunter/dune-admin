import { atomWithStorage } from 'jotai/utils'
import type { MarketView } from './types'

export const marketViewAtom = atomWithStorage<MarketView>('market-view', 'table')
