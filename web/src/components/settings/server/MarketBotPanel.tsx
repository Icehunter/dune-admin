import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Panel, SectionLabel } from '../../../dune-ui'
import { CheckboxField } from '../fields/CheckboxField'
import type { MarketBotPanelProps } from './interfaces'

export const MarketBotPanel: React.FC<MarketBotPanelProps> = ({ cfg, setBool }) => {
  const { t } = useTranslation()
  return (
    <Panel>
      <SectionLabel>{t('settings.sections.marketBot', 'Market Bot')}</SectionLabel>
      <CheckboxField
        label={t('settings.marketBot.enabled', 'Enable market bot for this server')}
        hint={t('settings.marketBot.enabledHint', 'Runs the embedded market bot against this server. Tuning is shared across servers and lives in the Market tab.')}
        checked={cfg.market_bot_enabled}
        onChange={setBool('market_bot_enabled')}
      />
    </Panel>
  )
}
