import type React from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { Player, PlayerStats, SessionRecord, StatSnapshot } from '../../../api/client'
import { LoadingState, Panel, SectionLabel } from '../../../dune-ui'
import { SolarisChart } from './SolarisChart'
import { SessionChart } from './SessionChart'
import { XPChart } from './XPChart'

interface PlayerDetailPanelProps {
  player: Player
}

function fmtSolaris(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtDuration(s: number): string {
  if (s <= 0) return '—'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps) {
  return (
    <div className="flex items-center justify-between py-1 border-b border-border/30 last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  )
}

export const PlayerDetailPanel: React.FC<PlayerDetailPanelProps> = ({ player }) => {
  const { t } = useTranslation()
  const [stats, setStats] = useState<PlayerStats | null>(null)
  const [sessions, setSessions] = useState<SessionRecord[]>([])
  const [snapshots, setSnapshots] = useState<StatSnapshot[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([
        api.players.stats(player.account_id),
        api.players.sessionHistory(player.account_id),
        api.players.statSnapshots(player.account_id),
      ]))
      .then(([s, sess, snaps]) => {
        setStats(s)
        setSessions(sess)
        setSnapshots(snaps)
      })
      .catch((e: unknown) => {
        // stats fetch failed — panel shows "Failed to load stats." fallback
        console.error('PlayerDetailPanel load error:', e)
      })
      .finally(() => setLoading(false))
  }, [player.account_id])

  if (loading) {
    return <LoadingState />
  }

  if (!stats) {
    return <p className="text-muted text-sm py-4 text-center">{t('players.detail.failedToLoad')}</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-3 gap-3">
        <Panel>
          <SectionLabel>{t('players.detail.economy')}</SectionLabel>
          <div className="mt-2">
            <StatRow label={t('players.detail.solaris')} value={fmtSolaris(stats.solaris_balance)} />
            <StatRow label={t('players.detail.scrip')} value={fmtSolaris(stats.scrip_balance)} />
            <StatRow label={t('players.detail.earned')} value={stats.solaris_earned > 0 ? fmtSolaris(stats.solaris_earned) : '—'} />
            <StatRow label={t('players.detail.spent')} value={stats.solaris_spent > 0 ? fmtSolaris(stats.solaris_spent) : '—'} />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>{t('players.detail.progression')}</SectionLabel>
          <div className="mt-2">
            <StatRow label={t('players.detail.charXP')} value={stats.char_xp > 0 ? stats.char_xp.toLocaleString() : '—'} />
            <StatRow label={t('players.detail.skillPts')} value={stats.skill_points > 0 ? stats.skill_points : '—'} />
            <StatRow label={t('players.detail.pois')} value={stats.pois_discovered > 0 ? stats.pois_discovered : '—'} />
            <StatRow label={t('players.detail.milestones')} value={stats.story_milestones > 0 ? stats.story_milestones : '—'} />
            <StatRow
              label={t('players.detail.factionAlignment')}
              value={stats.faction || t('players.detail.unaligned')}
            />
            <StatRow
              label={t('players.detail.factionTier')}
              value={stats.max_faction_tier > 0 ? t('players.detail.tier', { tier: stats.max_faction_tier }) : '—'}
            />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>{t('players.detail.sessions')}</SectionLabel>
          <div className="mt-2">
            <StatRow label={t('players.detail.playtime')} value={fmtDuration(stats.total_playtime_secs)} />
            <StatRow label={t('players.detail.sessionCount')} value={stats.session_count > 0 ? stats.session_count : '—'} />
            <StatRow label={t('players.detail.avgSession')} value={fmtDuration(stats.avg_session_secs)} />
            <StatRow
              label={t('players.detail.lastSeen')}
              value={stats.last_seen ? new Date(stats.last_seen as string).toLocaleDateString() : '—'}
            />
          </div>
        </Panel>
      </div>

      <Panel>
        <SolarisChart data={snapshots} />
      </Panel>

      <Panel>
        <SessionChart data={sessions} />
      </Panel>

      <Panel>
        <XPChart data={snapshots} />
      </Panel>
    </div>
  )
}
