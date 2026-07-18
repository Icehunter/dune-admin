import { atom } from 'jotai'
import { atomFamily } from 'jotai-family'
import type { Player, JourneyNode, TeleportLocation, ContractEntry, IntelInfo } from '../../../../api/client'

type ConfirmState = {
  title: string
  description: string
  confirmLabel: string
  onConfirm: () => void
}

export const playerAtom = atomFamily(() => atom<Player | null>(null))
export const partitionsAtom = atomFamily(() => atom<TeleportLocation[]>([]))
export const allPlayersAtom = atomFamily(() => atom<Player[]>([]))
export const charXPCurrentAtom = atomFamily(() => atom<{ xp: number, level: number } | null>(null))
export const intelCurrentAtom = atomFamily(() => atom<IntelInfo | null>(null))

export const nodesAtom = atomFamily(() => atom<JourneyNode[]>([]))
export const nodesLoadedAtom = atomFamily(() => atom(false))

export const contractCatalogAtom = atomFamily(() => atom<ContractEntry[]>([]))
export const contractCatalogLoadedAtom = atomFamily(() => atom(false))
export const contractCatalogErrorAtom = atomFamily(() => atom(''))

export const busyAtom = atomFamily(() => atom(false))
export const confirmAtom = atomFamily(() => atom<ConfirmState | null>(null))
