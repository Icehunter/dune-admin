import * as React from 'react'
import { Button, Chip } from '@heroui/react'
import { Sidebar } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../dune-ui'
import type { TabId } from '../../types'
import { BETA_TABS, DEFAULT_TAB, TAB_ICONS } from './nav'
import type { NavGroup } from './nav'

interface AppSidebarProps {
  visibleNavGroups: NavGroup[]
  pathname: string
  navigate: (path: string) => void
}

export const AppSidebar: React.FC<AppSidebarProps> = ({ visibleNavGroups, pathname, navigate }) => {
  const { t } = useTranslation()

  // A single top-level menu item. Sub-sections (Database, Welcome Kits,
  // Battle Pass) live inside their tab via an in-header Segment, so every
  // sidebar item is a plain top-level entry.
  const menuItem = (key: TabId) => {
    const label = visibleNavGroups.flatMap((g) => g.items).find((i) => i.key === key)?.label ?? key
    const icon = <Sidebar.MenuIcon><Icon name={TAB_ICONS[key]} /></Sidebar.MenuIcon>

    return (
      <Sidebar.MenuItem key={key} id={key} href={`/${key}`} isCurrent={pathname === `/${key}`} onAction={() => navigate(`/${key}`)}>
        {icon}
        <Sidebar.MenuLabel>
          <Sidebar.MenuItemContent>
            <Sidebar.MenuLabel>
              {label}
              {BETA_TABS.has(key) && (
                <Chip size="sm" color="accent" variant="soft" className="ml-1 text-[9px] h-4 px-1 min-w-0 shrink-0 self-center">{t('common.beta')}</Chip>
              )}
            </Sidebar.MenuLabel>
          </Sidebar.MenuItemContent>
        </Sidebar.MenuLabel>
      </Sidebar.MenuItem>
    )
  }

  return (
    <Sidebar>
      <Sidebar.Header>
        <Button
          variant="ghost"
          className="flex items-center gap-0 px-2 h-14 min-w-0 hover:opacity-80 w-full justify-start"
          onPress={() => navigate(`/${DEFAULT_TAB}`)}
          aria-label={t('app.goHome')}
        >
          <img src="/dune-admin-logo-primary.svg" alt="dune-admin" className="max-h-12 w-auto" />
          <span
            data-sidebar="label"
            className="text-xl font-bold uppercase text-accent overflow-hidden whitespace-nowrap"
          >
            {t('app.title')}
          </span>
        </Button>
      </Sidebar.Header>
      <Sidebar.Content className="pb-2">
        <Sidebar.Menu aria-label={t('nav.menu')}>
          {visibleNavGroups.map((group) => (
            <Sidebar.MenuSection key={group.title}>
              <Sidebar.MenuHeader>{group.title}</Sidebar.MenuHeader>
              {group.items.map((item) => menuItem(item.key))}
            </Sidebar.MenuSection>
          ))}
        </Sidebar.Menu>
      </Sidebar.Content>
    </Sidebar>
  )
}
