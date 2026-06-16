import { useTranslation } from 'react-i18next'
import type { TabId } from '../../types'
import type { NavGroup } from './nav'

// Left-sidebar navigation, grouped to mirror the product's structure (operator
// tooling today; a Player Portal group lands here later). Built from i18n so it
// re-derives on language change.
export const useNavGroups = (): NavGroup[] => {
  const { t } = useTranslation()
  return [
    {
      title: t('nav.groups.dashboard', 'Home'),
      items: [
        { key: 'dashboard' as TabId, label: t('nav.dashboard', 'Dashboard') },
      ],
    },
    {
      title: t('nav.groups.operations'),
      items: [
        { key: 'battlegroup' as TabId, label: t('nav.battlegroup') },
        { key: 'logs' as TabId, label: t('nav.logs') },
        { key: 'database' as TabId, label: t('nav.database') },
        { key: 'server' as TabId, label: t('nav.server') },
        { key: 'director' as TabId, label: t('nav.director') },
        { key: 'permissions' as TabId, label: t('nav.permissions') },
      ],
    },
    {
      title: t('nav.groups.playerWorld'),
      items: [
        { key: 'players' as TabId, label: t('nav.players') },
        { key: 'livemap' as TabId, label: t('nav.liveMap') },
        { key: 'storage' as TabId, label: t('nav.storage') },
        { key: 'bases' as TabId, label: t('nav.bases') },
        { key: 'guilds' as TabId, label: t('nav.guilds') },
        { key: 'landsraad' as TabId, label: t('nav.landsraad') },
        { key: 'blueprints' as TabId, label: t('nav.blueprints') },
      ],
    },
    {
      title: t('nav.groups.economy'),
      items: [
        { key: 'market' as TabId, label: t('nav.market') },
        { key: 'welcome' as TabId, label: t('nav.welcome') },
        { key: 'events' as TabId, label: t('nav.events') },
        { key: 'battlepass' as TabId, label: t('nav.battlepass') },
      ],
    },
  ]
}
