import { useState } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BotStatus } from '../../../api/client'
import { Icon, ConfirmDialog } from '../../../dune-ui'

type Props = {
  status: BotStatus
  onRefresh: () => void
}

type BusyOp = 'start' | 'stop' | 'restart' | 'cleanup'

export default function BotActions({ status, onRefresh }: Props) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState<BusyOp | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const run = async (cmd: 'start' | 'stop' | 'restart') => {
    setBusy(cmd)
    try {
      const res = await api.marketBot.lifecycle(cmd)
      const successKey = cmd === 'start'
        ? 'market.bot.actions.resumeSuccess'
        : cmd === 'stop'
          ? 'market.bot.actions.pauseSuccess'
          : 'market.bot.actions.reinitializeSuccess'
      toast.success(t(successKey, { output: res.output || 'ok' }))
      setTimeout(onRefresh, 1500)
    }
    catch (e: unknown) {
      const failKey = cmd === 'start'
        ? 'market.bot.actions.resumeFailed'
        : cmd === 'stop'
          ? 'market.bot.actions.pauseFailed'
          : 'market.bot.actions.reinitializeFailed'
      toast.danger(t(failKey, { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setBusy(null)
    }
  }

  const runCleanup = async () => {
    setConfirmOpen(false)
    setBusy('cleanup')
    try {
      const res = await api.marketBot.cleanup()
      toast.success(t('market.bot.actions.wipedListings', { orders: res.orders_deleted, items: res.items_deleted }))
      setTimeout(onRefresh, 1500)
    }
    catch (e: unknown) {
      toast.danger(t('market.bot.actions.cleanupFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setBusy(null)
    }
  }

  const running = status?.running ?? false
  const dormant = status?.mode === 'none'

  return (
    <>
      <div className="flex items-center gap-2 flex-wrap">
        {dormant
          ? (
              <span className="text-xs text-muted">
                {t('market.bot.actions.dormantHint')}
              </span>
            )
          : (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  isDisabled={running || busy !== null}
                  onPress={() => run('start')}
                >
                  {busy === 'start' ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
                  {t('market.bot.actions.resume')}
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={!running || busy !== null}
                  onPress={() => run('stop')}
                >
                  {busy === 'stop' ? <Spinner size="sm" color="current" /> : <Icon name="square" />}
                  {t('market.bot.actions.pause')}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={busy !== null}
                  onPress={() => run('restart')}
                >
                  {busy === 'restart' ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                  {t('market.bot.actions.reinitialize')}
                </Button>
              </>
            )}

        <Button
          size="sm"
          variant="danger-soft"
          isDisabled={busy !== null}
          onPress={() => setConfirmOpen(true)}
        >
          {busy === 'cleanup' ? <Spinner size="sm" color="current" /> : <Icon name="trash-2" />}
          {t('market.bot.actions.wipeListings')}
        </Button>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        title={t('market.bot.actions.wipeListingsTitle')}
        description={t('market.bot.actions.wipeListingsDesc')}
        confirmLabel={t('market.bot.actions.wipeListingsConfirm')}
        onConfirm={runCleanup}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  )
}
