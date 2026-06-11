import * as React from 'react'
import { Dropdown, Button, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../dune-ui'
import { copyText } from '../utils/clipboard'
import type { Status } from '../api/client'

const REPO = 'https://github.com/Icehunter/dune-admin'

const buildDiagnostics = (status?: Status | null): string => {
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

export const HelpMenu: React.FC<{ status?: Status | null }> = ({ status }) => {
  const { t } = useTranslation()

  const reportIssue = () => {
    const body = `## Describe the issue\n\n<!-- What happened? What did you expect? Steps to reproduce. -->\n\n## Environment (auto-filled by dune-admin)\n${buildDiagnostics(status)}\n`
    window.open(`${REPO}/issues/new?body=${encodeURIComponent(body)}`, '_blank', 'noopener,noreferrer')
  }
  const copyDiagnostics = () => {
    copyText(buildDiagnostics(status)).then((ok) => {
      if (ok) toast.success(t('help.copied'))
      else toast.danger(t('help.copyFailed'))
    })
  }
  const openRepo = () => {
    window.open(REPO, '_blank', 'noopener,noreferrer')
  }

  const items = [
    { id: 'report', icon: 'bug', label: t('help.reportIssue'), action: reportIssue },
    { id: 'copy', icon: 'clipboard', label: t('help.copyDiagnostics'), action: copyDiagnostics },
    { id: 'repo', icon: 'github', label: t('help.viewOnGitHub'), action: openRepo },
  ]

  return (
    <Dropdown>
      <Button
        isIconOnly
        variant="ghost"
        size="sm"
        aria-label={t('help.menu')}
        className="w-8 h-8 min-w-0 text-muted data-[hover=true]:text-foreground data-[hover=true]:bg-surface-secondary"
      >
        <Icon name="circle-help" />
      </Button>
      <Dropdown.Popover>
        <Dropdown.Menu
          aria-label={t('help.menu')}
          onAction={(key) => items.find((i) => i.id === String(key))?.action()}
        >
          {items.map((it) => (
            <Dropdown.Item key={it.id} id={it.id} textValue={it.label}>
              <span className="flex items-center gap-2">
                <Icon name={it.icon} className="w-4 h-4 text-muted" />
                <span>{it.label}</span>
              </span>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  )
}
