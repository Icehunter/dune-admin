import { useState, useEffect, useCallback, useRef } from 'react'

/**
 * Polls `fn` every `intervalMs` while `active` is true.
 * Returns `countdown` (seconds until next auto-refresh) and a `refresh`
 * function for manual triggers — calling it fires `fn` and resets the timer.
 */
export function useAutoRefresh(
  fn: () => void,
  intervalMs: number,
  active: boolean,
): { countdown: number, refresh: () => void } {
  const fnRef = useRef(fn)
  useEffect(() => {
    fnRef.current = fn
  })

  const secsTotal = Math.round(intervalMs / 1000)
  const [countdown, setCountdown] = useState(secsTotal)

  useEffect(() => {
    if (!active) {
      Promise.resolve().then(() => setCountdown(secsTotal))
      return
    }

    Promise.resolve().then(() => setCountdown(secsTotal))

    const poll = setInterval(() => {
      fnRef.current()
      setCountdown(secsTotal)
    }, intervalMs)

    const tick = setInterval(() => {
      setCountdown((s) => Math.max(0, s - 1))
    }, 1000)

    return () => {
      clearInterval(poll)
      clearInterval(tick)
    }
  }, [active, intervalMs, secsTotal])

  const refresh = useCallback(() => {
    fnRef.current()
    setCountdown(secsTotal)
  }, [secsTotal])

  return { countdown, refresh }
}
