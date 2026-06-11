import { useAtom, useSetAtom } from 'jotai'
import { toast } from '@heroui/react'
import { busyAtom, confirmAtom } from '../store'

export const useRun = (playerId: number) => {
  const [, setBusy] = useAtom(busyAtom(playerId))
  return async (fn: () => Promise<unknown>, label: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(label)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setBusy(false)
    }
  }
}

export const useGate = (playerId: number) => {
  const setConfirm = useSetAtom(confirmAtom(playerId))
  return (title: string, description: string, confirmLabel: string, onConfirm: () => void) => {
    setConfirm({ title, description, confirmLabel, onConfirm })
  }
}
