export type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

export const TRAINERS = ['BeneGesserit', 'Mentat', 'Planetologist', 'Swordmaster', 'Trooper'] as const
export type TrainerKey = typeof TRAINERS[number]

export type FilterTab = 'all' | 'done' | 'revealed' | 'reward'
