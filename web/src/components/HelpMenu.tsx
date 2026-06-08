import type React from 'react'
import { useState, useEffect, useRef } from 'react'
import { ListBox, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'
import { copyText } from '../utils/clipboard'
import type { Status } from '../api/client'

const REPO = 'https://github.com/Icehunter/dune-admin'

// buildDiagnostics renders the auto-filled environment block included in a bug
// report / copied to the clipboard. No secrets — just the values already shown
// in the header plus the browser UA.
function buildDiagnostics(status?: Status | null): string {
  return [
    `- dune-admin version: ${status?.version ?? 'unknown'}`,
    `- commit: ${status?.commit ?? 'unknown'}`,
    `- build: ${status?.build_time ?? 'unknown'}`,
    `- control plane: ${status?.control ?? 'unknown'}`,
    `- executor: ${status?.executor ?? 'unknown'}`,
    `- db connected: ${status?.db_connected ?? false}`,
    `- ssh connected: ${status?.ssh_connected ?? false}`,
    `- browser: ${typeof navigator !== 'undefined' ? navigator.userAgent : 'unknown'}`,
  ].join('\n')
}

// HelpMenu (#143): a header dropdown that makes it easy to file a GitHub issue
// pre-filled with diagnostic context, copy that context for Discord/etc., and
// reach the repository. Supersedes the standalone GitHub link (#138/#151).
export const HelpMenu: React.FC<{ status?: Status | null }> = ({ status }) => {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const reportIssue = () => {
    const body = `## Describe the issue\n\n<!-- What happened? What did you expect? Steps to reproduce. -->\n\n## Environment (auto-filled by dune-admin)\n${buildDiagnostics(status)}\n`
    window.open(`${REPO}/issues/new?body=${encodeURIComponent(body)}`, '_blank', 'noopener,noreferrer')
    setOpen(false)
  }
  const copyDiagnostics = () => {
    copyText(buildDiagnostics(status)).then((ok) => {
      if (ok) toast.success(t('help.copied'))
      else toast.danger(t('help.copyFailed'))
    })
    setOpen(false)
  }
  const openRepo = () => {
    window.open(REPO, '_blank', 'noopener,noreferrer')
    setOpen(false)
  }

  const items = [
    { id: 'report', icon: 'bug', label: t('help.reportIssue'), action: reportIssue },
    { id: 'copy', icon: 'clipboard', label: t('help.copyDiagnostics'), action: copyDiagnostics },
    { id: 'repo', icon: 'github', label: t('help.viewOnGitHub'), action: openRepo },
  ]

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        className="flex items-center justify-center w-8 h-8 rounded text-muted hover:text-foreground hover:bg-surface-secondary transition-colors"
        aria-label={t('help.menu')}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="circle-help" />
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[210px] rounded-[var(--radius)] border border-border bg-surface shadow-lg overflow-hidden">
          <ListBox
            aria-label={t('help.menu')}
            onAction={(key) => items.find((i) => i.id === String(key))?.action()}
          >
            {items.map((it) => (
              <ListBox.Item
                key={it.id}
                id={it.id}
                textValue={it.label}
                className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-surface-hover text-foreground"
              >
                <Icon name={it.icon} className="w-4 h-4 text-muted" />
                <span>{it.label}</span>
              </ListBox.Item>
            ))}
          </ListBox>
        </div>
      )}
    </div>
  )
}
