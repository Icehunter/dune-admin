import * as React from 'react'
import { Show, SignInButton, UserButton } from '@clerk/react'
import { Button, Chip, ListBox, Select } from '@heroui/react'
import { Navbar, Sidebar } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { useAtomValue, useSetAtom } from 'jotai'
import { Icon } from '../../dune-ui'
import { LanguageSelector } from '../LanguageSelector'
import { ThemeSelector } from '../ThemeSelector'
import { HelpMenu } from '../HelpMenu'
import { UserMenu } from '../UserMenu'
import { useActiveServer } from '../../context/useActiveServer'
import {
  addServerOpenAtom,
  manageServerIdAtom,
  settingsOpenAtom,
  updateInfoAtom,
  updatePromptOpenAtom,
} from '../../atoms/app'
import { ConnectionBadge } from './ConnectionBadge'
import type { Status } from '../../api/client'

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

interface AppNavbarProps {
  status: Status | null
  reconnecting: boolean
  onReconnect: () => void
  can: (cap: string) => boolean
  onOpenSettings: (tab?: string) => void
}

export const AppNavbar: React.FC<AppNavbarProps> = ({ status, reconnecting, onReconnect, can, onOpenSettings }) => {
  const { t } = useTranslation()
  const { servers, activeID, setActive } = useActiveServer()
  const updateInfo = useAtomValue(updateInfoAtom)
  const settingsOpen = useAtomValue(settingsOpenAtom)
  const setSettingsOpen = useSetAtom(settingsOpenAtom)
  const setAddServerOpen = useSetAtom(addServerOpenAtom)
  const setManageServerId = useSetAtom(manageServerIdAtom)
  const setUpdatePromptOpen = useSetAtom(updatePromptOpenAtom)

  return (
    <Navbar position="sticky" maxWidth="full">
      <Navbar.Header>
        <Sidebar.Trigger />
        <div className="flex items-center gap-3">
          {/* Connection info is meaningless with no servers configured (fresh
              install / last server deleted) — hide it then. */}
          {servers.length > 0 && status?.control && status.control !== 'none' && <span className="text-xs text-muted">{status.control}</span>}
          {servers.length > 0 && status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {servers.length > 0 && status?.db_host && status.control !== 'kubectl' && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && (
            <Button
              variant="ghost"
              className="text-xs text-muted hover:text-foreground px-0 h-auto min-w-0"
              onPress={() => onOpenSettings()}
              aria-label={t('app.openSettings')}
            >
              v
              {status.version}
            </Button>
          )}
          {updateInfo?.needs_update && (
            <Button
              variant="ghost"
              onPress={() => setUpdatePromptOpen(true)}
              aria-label={t('app.updateAvailable')}
              className="cursor-pointer p-0 h-auto min-w-0"
            >
              <Chip size="sm" color="warning" variant="soft">
                ↑
                {' '}
                {updateInfo.latest.replace(/^v/, '')}
              </Chip>
            </Button>
          )}
        </div>

        {servers.length > 0 && (
          <div className="flex items-center gap-1">
            {/* Always render the dropdown when there is ≥1 server so the navbar
                layout doesn't jump when a second server is added. */}
            <Select
              aria-label="Active server"
              className="w-40"
              selectedKey={String(activeID || servers[0]?.id || '')}
              onSelectionChange={(id) => {
                const next = Number(id)
                if (next && next !== activeID) void setActive(next)
              }}
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {servers.map((s) => (
                    <ListBox.Item key={s.id} id={String(s.id)} textValue={s.name}>
                      {s.name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
            {can('server:control') && (
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('manage.title', 'Manage server')}
                onPress={() => setManageServerId(activeID || servers[0]?.id || 0)}
              >
                <Icon name="settings" />
              </Button>
            )}
            {can('server:control') && (
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label="Add server"
                onPress={() => setAddServerOpen(true)}
              >
                <Icon name="plus" />
              </Button>
            )}
          </div>
        )}

        <Navbar.Spacer />

        <Navbar.Content>
          {/* Connection badges + reconnect only make sense with a server
              configured — hide them on a fresh/empty install. */}
          {servers.length > 0 && status?.executor === 'ssh' && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          {servers.length > 0 && <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />}
          {servers.length > 0 && can('server:control') && status && !status.db_connected && (
            <Button
              size="sm"
              variant="outline"
              isDisabled={reconnecting}
              onPress={onReconnect}
            >
              {reconnecting ? t('app.reconnecting') : t('app.reconnect')}
            </Button>
          )}
          {status?.pod_ns && (
            <span className="text-xs text-muted">
              ns:
              {status.pod_ns}
            </span>
          )}

          <HelpMenu status={status} />
          <ThemeSelector />
          <LanguageSelector />

          {can('config:read') && (
            <Button
              size="sm"
              variant="outline"
              aria-label={t('app.configureBackend')}
              onPress={() => setSettingsOpen((v) => !v)}
              className={settingsOpen ? 'text-accent border-accent' : ''}
            >
              <Icon name="settings" />
              {' '}
              {t('app.settings')}
            </Button>
          )}

          <UserMenu />

          {hasClerk && (
            <>
              <Show when="signed-out">
                <SignInButton>
                  <Button size="sm" variant="outline">
                    {t('app.signIn')}
                  </Button>
                </SignInButton>
              </Show>
              <Show when="signed-in">
                <UserButton />
              </Show>
            </>
          )}
        </Navbar.Content>
      </Navbar.Header>
    </Navbar>
  )
}
