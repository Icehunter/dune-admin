import * as React from 'react'
import { useAtom, useSetAtom } from 'jotai'
import { toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import {
  settingsOpenAtom,
  updateApplyingAtom,
  updateCheckingAtom,
  updateErrorAtom,
  updateInfoAtom,
  updatePhaseAtom,
  updatePromptOpenAtom,
} from '../../atoms/app'

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

const UPDATE_CACHE_KEY = 'dune_update_cache'
const UPDATE_CACHE_TTL_MS = 60 * 60 * 1000

export interface AppUpdate {
  checkUpdate: () => Promise<void>
  applyUpdate: (force?: boolean) => Promise<void>
}

// Owns the update check/apply/poll-and-reload flow. State lives in atoms so the
// navbar release widget and the Settings/prompt modals all observe it.
export const useAppUpdate = (): AppUpdate => {
  const { t } = useTranslation()
  const [, setUpdateInfo] = useAtom(updateInfoAtom)
  const setChecking = useSetAtom(updateCheckingAtom)
  const setApplying = useSetAtom(updateApplyingAtom)
  const setPhase = useSetAtom(updatePhaseAtom)
  const setError = useSetAtom(updateErrorAtom)
  const setSettingsOpen = useSetAtom(settingsOpenAtom)
  const setPromptOpen = useSetAtom(updatePromptOpenAtom)

  // Check for a newer release via the backend — cached in localStorage for 1 hour
  // to avoid hammering GitHub's unauthenticated API rate limit during dev HMR cycles.
  React.useEffect(() => {
    try {
      const cached = localStorage.getItem(UPDATE_CACHE_KEY)
      if (cached) {
        const { ts, data } = JSON.parse(cached)
        if (Date.now() - ts < UPDATE_CACHE_TTL_MS) {
          Promise.resolve().then(() => setUpdateInfo(data))
          return
        }
      }
    }
    catch { /* ignore corrupt cache */ }
    api.update.check().then((data) => {
      setUpdateInfo(data)
      try {
        localStorage.setItem(UPDATE_CACHE_KEY, JSON.stringify({ ts: Date.now(), data }))
      }
      catch { /* ignore */ }
    }).catch(() => {})
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const checkUpdate = React.useCallback(async () => {
    setChecking(true)
    try {
      const data = await api.update.check()
      setUpdateInfo(data)
      try {
        localStorage.setItem(UPDATE_CACHE_KEY, JSON.stringify({ ts: Date.now(), data }))
      }
      catch { /* ignore */ }
    }
    catch {
      // silently ignore — user can retry
    }
    finally {
      setChecking(false)
    }
  }, [setChecking, setUpdateInfo])

  const applyUpdate = React.useCallback(async (force = false) => {
    setApplying(true)
    setPhase('downloading')
    setError(undefined)
    setPromptOpen(false)
    setSettingsOpen(false)
    try {
      const result = await api.update.apply(force)
      if (!result.updated) {
        toast.info(result.message)
        setApplying(false)
        return
      }
      localStorage.removeItem(UPDATE_CACHE_KEY)
      setUpdateInfo(null)

      // Binary swapped; server is restarting in ~500ms.
      setPhase('verifying')
      await sleep(400)
      setPhase('extracting')
      await sleep(300)
      setPhase('restarting')

      // Poll /status until the server comes back up (max 90s).
      const started = Date.now()
      const TIMEOUT_MS = 90_000
      const LONG_WAIT_MS = 20_000
      await sleep(2000)
      setPhase('waiting')

      let back = false
      while (Date.now() - started < TIMEOUT_MS) {
        if (Date.now() - started > LONG_WAIT_MS) {
          setPhase('waitingLong')
        }
        try {
          await api.status()
          back = true
          break
        }
        catch {
          await sleep(2000)
        }
      }

      if (back) {
        setPhase('ready')
        await sleep(800)
        window.location.reload()
      }
      else {
        // Timed out but keep polling in background; page will reload on next success.
        setPhase('waitingLong')
        const keepPolling = async () => {
          while (true) {
            await sleep(3000)
            try {
              await api.status()
              window.location.reload()
              return
            }
            catch { /* keep trying */ }
          }
        }
        void keepPolling()
      }
    }
    catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(t('app.updateFailed', { message: msg }))
      setPhase('error')
    }
  }, [setApplying, setPhase, setError, setPromptOpen, setSettingsOpen, setUpdateInfo, t])

  return { checkUpdate, applyUpdate }
}
