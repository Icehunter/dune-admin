import { useEffect, useState } from 'react'
import { Spinner } from '@heroui/react'
import { api } from '../../../api/client'
import type { Player, PlayerStats, SolarisPoint, SessionRecord, StatSnapshot } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { SolarisChart } from './SolarisChart'
import { SessionChart } from './SessionChart'
import { XPChart } from './XPChart'

interface Props {
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

function StatRow({ label, value }: { label: string, value: string | number }) {
  return (
    <div className="flex items-center justify-between py-1 border-b border-border/30 last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  )
}

export function PlayerDetailPanel({ player }: Props) {
  const [stats, setStats] = useState<PlayerStats | null>(null)
  const [solaris, setSolaris] = useState<SolarisPoint[]>([])
  const [sessions, setSessions] = useState<SessionRecord[]>([])
  const [snapshots, setSnapshots] = useState<StatSnapshot[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([
        api.players.stats(player.account_id),
        api.players.solarisHistory(player.account_id),
        api.players.sessionHistory(player.account_id),
        api.players.statSnapshots(player.account_id),
      ]))
      .then(([s, sol, sess, snaps]) => {
        setStats(s)
        setSolaris(sol)
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
    return <div className="flex justify-center py-12"><Spinner size="lg" /></div>
  }

  if (!stats) {
    return <p className="text-muted text-sm py-4 text-center">Failed to load stats.</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-3 gap-3">
        <Panel>
          <SectionLabel>Economy</SectionLabel>
          <div className="mt-2">
            <StatRow label="Solaris" value={fmtSolaris(stats.solaris_balance)} />
            <StatRow label="Scrip" value={fmtSolaris(stats.scrip_balance)} />
            <StatRow label="Earned" value={stats.solaris_earned > 0 ? fmtSolaris(stats.solaris_earned) : '—'} />
            <StatRow label="Spent" value={stats.solaris_spent > 0 ? fmtSolaris(stats.solaris_spent) : '—'} />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>Progression</SectionLabel>
          <div className="mt-2">
            <StatRow label="Char XP" value={stats.char_xp > 0 ? stats.char_xp.toLocaleString() : '—'} />
            <StatRow label="Skill pts" value={stats.skill_points > 0 ? stats.skill_points : '—'} />
            <StatRow label="POIs" value={stats.pois_discovered > 0 ? stats.pois_discovered : '—'} />
            <StatRow label="Milestones" value={stats.story_milestones > 0 ? stats.story_milestones : '—'} />
            <StatRow
              label="Faction tier"
              value={stats.max_faction_tier > 0 ? `Tier ${stats.max_faction_tier}` : '—'}
            />
          </div>
        </Panel>

        <Panel>
          <SectionLabel>Sessions</SectionLabel>
          <div className="mt-2">
            <StatRow label="Playtime" value={fmtDuration(stats.total_playtime_secs)} />
            <StatRow label="Count" value={stats.session_count > 0 ? stats.session_count : '—'} />
            <StatRow label="Avg" value={fmtDuration(stats.avg_session_secs)} />
            <StatRow
              label="Last seen"
              value={stats.last_seen ? new Date(stats.last_seen as string).toLocaleDateString() : '—'}
            />
          </div>
        </Panel>
      </div>

      <Panel>
        <SolarisChart data={solaris} />
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
