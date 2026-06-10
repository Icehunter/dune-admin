import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Spinner } from '@heroui/react'
import { NumberValue } from '@heroui-pro/react'
import { DataTable, Icon, SectionLabel } from '../../../../../dune-ui'
import { KeystonesToggle } from '../components/KeystonesToggle'
import { XP_TRACKS } from '../../../types'
import { api } from '../../../../../api/client'
import type { Player, SpecTrack, KeystoneRow } from '../../../../../api/client'
import { busyAtom } from '../store'
import { useRun, useGate } from '../hooks/useActions'

interface SpecsSectionProps { player: Player }

export function SpecsSection({ player }: SpecsSectionProps) {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const run = useRun(player.id)
  const gate = useGate(player.id)

  const [playerSpecs, setPlayerSpecs] = useState<SpecTrack[]>([])
  const [playerKeystones, setPlayerKeystones] = useState<KeystoneRow[]>([])
  const [specsLoaded, setSpecsLoaded] = useState(false)
  const [specsLoading, setSpecsLoading] = useState(false)

  useEffect(() => {
    Promise.resolve().then(() => {
      setSpecsLoaded(false)
      setPlayerSpecs([])
      setPlayerKeystones([])
    })
  }, [player.id])

  useEffect(() => {
    if (specsLoaded) return
    Promise.resolve()
      .then(() => setSpecsLoading(true))
      .then(() =>
        Promise.all([
          api.players.specs_for(player.controller_id),
          api.players.keystones(player.controller_id),
        ]),
      )
      .then(([s, k]) => {
        setPlayerSpecs(s)
        setPlayerKeystones(k)
        setSpecsLoaded(true)
      })
      .catch(() => {})
      .finally(() => setSpecsLoading(false))
  }, [specsLoaded, player.controller_id])

  const handleGrantMaxKeystones = () => {
    run(() => api.players.grantAllKeystones(player.controller_id),
      `Grant all keystones to ${player.name}`)
      .then(() => setSpecsLoaded(false))
  }

  const handleResetAllKeystones = () => {
    gate(
      t('players.actions.specs.resetKeystonesTitle'),
      t('players.actions.specs.resetKeystonesDesc', { player: player.name }),
      t('players.actions.specs.resetAllKeystones'),
      () => run(() => api.players.resetAllKeystones(player.controller_id),
        `Reset all keystones for ${player.name}`)
        .then(() => setSpecsLoaded(false)),
    )
  }

  const handleGrantMaxSpec = (track: string) => {
    run(() => api.players.grantMaxSpec(player.controller_id, track),
      `Grant max ${track} spec to ${player.name}`)
      .then(() => setPlayerSpecs((prev) => {
        const exists = prev.find((s) => s.track_type === track)
        if (exists) {
          return prev.map((s) =>
            s.track_type === track ? { ...s, xp: 44182, level: 100 } : s)
        }
        return [...prev,
          { player_id: player.controller_id, track_type: track, xp: 44182,
            level: 100 }]
      }))
  }

  const handleResetSpec = (track: string) => {
    gate(
      t('players.actions.specs.resetSpecTitle', { track }),
      t('players.actions.specs.resetSpecDesc', { track }),
      t('players.actions.specs.resetSpec'),
      () => run(() => api.players.resetSpec(player.controller_id, track),
        `Reset ${track} spec for ${player.name}`)
        .then(() => setPlayerSpecs((prev) =>
          prev.filter((s) => s.track_type !== track))),
    )
  }

  return (
    <div className="flex flex-col gap-3 flex-1 min-h-0 overflow-hidden">
      <div className="flex items-center gap-3 min-h-8">
        <div className="flex-1"><SectionLabel>{t('players.actions.specs.specializations')}</SectionLabel></div>
        <Button size="sm" variant="ghost" isDisabled={specsLoading} onPress={() => setSpecsLoaded(false)}>
          {specsLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        </Button>
        <Button
          size="sm"
          variant="outline"
          isDisabled={busy || player.online_status === 'Online'}
          onPress={handleGrantMaxKeystones}
        >
          {t('players.actions.specs.grantMaxKeystones')}
        </Button>
        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy || player.online_status === 'Online'}
          onPress={handleResetAllKeystones}
        >
          {t('players.actions.specs.resetAllKeystones')}
        </Button>
      </div>
      {player.online_status === 'Online' && (
        <div className="text-xs px-3 py-2 rounded mb-3 bg-warning/10 border border-warning text-warning">
          {t('players.actions.specs.onlineWarning')}
        </div>
      )}
      <DataTable<{ id: string }, 'track' | 'xp' | 'level' | 'grant' | 'reset'>
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
        rows={XP_TRACKS.map((t) => ({ id: t }))}
        rowId={(r) => r.id}
        initialSort={{ column: 'track', direction: 'ascending' }}
        sortValue={(r, k) => {
          const found = playerSpecs.find((s) => s.track_type === r.id)
          if (k === 'track') return r.id
          if (k === 'xp') return found?.xp ?? 0
          if (k === 'level') return found?.level ?? 0
          return ''
        }}
        renderCell={(r, key) => {
          const track = r.id
          const found = playerSpecs.find((s) => s.track_type === track)
          const trackKeystones = playerKeystones.filter((k) => k.track === track)
          switch (key) {
            case 'track':
              return (
                <span className="inline-flex flex-col font-semibold align-top">
                  <span>{track}</span>
                  {trackKeystones.length > 0 && <KeystonesToggle keystones={trackKeystones} />}
                </span>
              )
            case 'xp': return <NumberValue value={found?.xp ?? 0} maximumFractionDigits={0} className="font-mono text-muted" />
            case 'level': return <span className="font-mono text-muted">{found?.level ?? 0}</span>
            case 'grant':
              return (
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy || player.online_status === 'Online'}
                  onPress={() => handleGrantMaxSpec(track)}
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
                  onPress={() => handleResetSpec(track)}
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
