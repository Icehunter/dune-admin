import { useTranslation } from 'react-i18next'
import { Button, Spinner } from '@heroui/react'
import { DataTable, Icon, SectionLabel } from '../../../../../dune-ui'
import { KeystonesToggle } from '../components/KeystonesToggle'
import { XP_TRACKS } from '../../../types'
import type { Player, SpecTrack, KeystoneRow } from '../../../../../api/client'

interface SpecsSectionProps {
  player: Player
  playerSpecs: SpecTrack[]
  playerKeystones: KeystoneRow[]
  specsLoading: boolean
  busy: boolean
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
  gate: (title: string, description: string, confirmLabel: string, action: () => void) => void
  onRefresh: () => void
  onSpecsUpdate: (specs: SpecTrack[]) => void
}

export function SpecsSection({
  player,
  playerSpecs,
  playerKeystones,
  specsLoading,
  busy,
  run,
  gate,
  onRefresh,
  onSpecsUpdate,
}: SpecsSectionProps) {
  const { t } = useTranslation()

  const onlineWarning = (
    <div className="text-xs px-3 py-2 rounded mb-3 bg-warning/10 border border-warning text-warning">
      {t('players.actions.specs.onlineWarning')}
    </div>
  )

  return (
    <div className="flex flex-col gap-3 flex-1 min-h-0 overflow-hidden">
      <div className="flex items-center gap-3 min-h-8">
        <div className="flex-1"><SectionLabel>{t('players.actions.specs.specializations')}</SectionLabel></div>
        <Button size="sm" variant="ghost" isDisabled={specsLoading} onPress={onRefresh}>
          {specsLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        </Button>
        <Button
          size="sm"
          variant="outline"
          isDisabled={busy || player.online_status === 'Online'}
          onPress={() =>
            run(
              () => import('../../../../../api/client').then((m) => m.api.players.grantAllKeystones(player.controller_id)),
              `Grant all keystones to ${player.name}`,
            ).then(onRefresh)}
        >
          {t('players.actions.specs.grantMaxKeystones')}
        </Button>
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy || player.online_status === 'Online'}
          onPress={() =>
            gate(
              t('players.actions.specs.resetKeystonesTitle'),
              t('players.actions.specs.resetKeystonesDesc', { player: player.name }),
              t('players.actions.specs.resetAllKeystones'),
              () =>
                run(
                  () => import('../../../../../api/client').then((m) => m.api.players.resetAllKeystones(player.controller_id)),
                  `Reset all keystones for ${player.name}`,
                ).then(onRefresh),
            )}
        >
          {t('players.actions.specs.resetAllKeystones')}
        </Button>
      </div>
      {player.online_status === 'Online' && onlineWarning}
      <DataTable<string, 'track' | 'xp' | 'level' | 'grant' | 'reset'>
        aria-label={t('players.actions.specs.specsLabel')}
        className="min-h-0 max-h-full"
        loading={specsLoading}
        columns={[
          { key: 'track', label: t('players.actions.specs.columns.track'), isRowHeader: true },
          { key: 'xp', label: t('players.actions.specs.columns.xp') },
          { key: 'level', label: t('players.actions.specs.columns.level') },
          { key: 'grant', label: ' ', sortable: false },
          { key: 'reset', label: ' ', sortable: false },
        ]}
        rows={XP_TRACKS}
        rowId={(t) => t}
        initialSort={{ column: 'track', direction: 'ascending' }}
        sortValue={(t, k) => {
          const found = playerSpecs.find((s) => s.track_type === t)
          if (k === 'track') return t
          if (k === 'xp') return found?.xp ?? 0
          if (k === 'level') return found?.level ?? 0
          return ''
        }}
        renderCell={(track: string, key: string) => {
          const found = playerSpecs.find((s) => s.track_type === track)
          const trackKeystones = playerKeystones.filter((k) => k.track === track)
          switch (key) {
            case 'track':
              return (
                <span className="inline-flex flex-col font-semibold align-top">
                  <span>{track}</span>
                  {trackKeystones.length > 0 && (
                    <KeystonesToggle keystones={trackKeystones} />
                  )}
                </span>
              )
            case 'xp':
              return <span className="font-mono text-muted">{(found?.xp ?? 0).toLocaleString()}</span>
            case 'level':
              return <span className="font-mono text-muted">{found?.level ?? 0}</span>
            case 'grant':
              return (
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy || player.online_status === 'Online'}
                  onPress={() =>
                    run(
                      () => import('../../../../../api/client').then((m) => m.api.players.grantMaxSpec(player.controller_id, track)),
                      `Grant max ${track} spec to ${player.name}`,
                    ).then(() => {
                      onSpecsUpdate([
                        ...playerSpecs.filter((s) => s.track_type !== track),
                        { player_id: player.controller_id, track_type: track, xp: 44182, level: 100 },
                      ])
                    })}
                >
                  {t('players.actions.specs.grantMax')}
                </Button>
              )
            case 'reset':
              return (
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={busy}
                  onPress={() =>
                    gate(
                      t('players.actions.specs.resetSpecTitle', { track }),
                      t('players.actions.specs.resetSpecDesc', { track }),
                      t('players.actions.specs.resetSpec'),
                      () =>
                        run(
                          () => import('../../../../../api/client').then((m) => m.api.players.resetSpec(player.controller_id, track)),
                          `Reset ${track} spec for ${player.name}`,
                        ).then(() =>
                          onSpecsUpdate(playerSpecs.filter((s) => s.track_type !== track)),
                        ),
                    )}
                >
                  {t('players.actions.specs.resetSpec')}
                </Button>
              )
          }
        }}
      />
    </div>
  )
}
