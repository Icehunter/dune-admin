import { toast as sonner } from 'sonner'

type ToastOptions = Parameters<typeof sonner.success>[1]

/**
 * HeroUI-compatible toast facade over sonner. The existing call sites use
 * `toast.success/.danger/.info/.warning(message)`, so keeping that exact surface
 * lets a tab migrate by changing only its import path — not its calls. `.danger`
 * maps to sonner's error severity. Requires <Toaster /> mounted once (see App).
 */
export const toast = {
  success: (message: string, options?: ToastOptions) => sonner.success(message, options),
  danger: (message: string, options?: ToastOptions) => sonner.error(message, options),
  info: (message: string, options?: ToastOptions) => sonner.info(message, options),
  warning: (message: string, options?: ToastOptions) => sonner.warning(message, options),
}
