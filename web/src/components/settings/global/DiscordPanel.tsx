import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { MASKED } from '../../../api/client'
import type { AppConfig } from '../../../api/client'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

export interface DiscordPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
}

// DiscordPanel (global) now only edits the bot-wide credentials: enable + token.
// The per-guild config (roles + the servers each guild watches and their
// channels) lives in the guild-centric Discord Guilds tab, since one bot token
// serves every guild.
export const DiscordPanel: React.FC<DiscordPanelProps> = ({ cfg, set, setBool }) => {
  const { t } = useTranslation()
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <Panel>
        <SectionLabel>{t('settings.sections.discordBot')}</SectionLabel>
        <div className="flex flex-col gap-1 -mt-1">
          <p className="text-sm text-muted">{t('settings.discord.hint')}</p>
          <p className="text-sm text-muted">{t('settings.discord.setupStep1')}</p>
          <p className="text-sm text-muted">{t('settings.discord.setupStep2')}</p>
          <p className="text-sm text-muted">{t('settings.discord.setupStep3')}</p>
        </div>
        <TwoColumnGrid>
          <div className="sm:col-span-2">
            <CheckboxField
              label={t('settings.discord.enabled')}
              checked={cfg.discord_bot_enabled}
              onChange={setBool('discord_bot_enabled')}
            />
          </div>
          <FieldRow label={t('settings.discord.token')} hint={t('settings.discord.tokenHint')}>
            <TextInput value={cfg.discord_bot_token} onChange={set('discord_bot_token')} type="password" placeholder={MASKED} />
          </FieldRow>
        </TwoColumnGrid>
      </Panel>

      <Panel>
        <SectionLabel>{t('settings.sections.discordGuilds')}</SectionLabel>
        <div className="flex items-start gap-2 -mt-1 text-sm text-muted">
          <Icon name="info" className="mt-0.5 shrink-0" />
          <p>{t('settings.discord.perGuildMoved')}</p>
        </div>
      </Panel>
    </div>
  )
}
