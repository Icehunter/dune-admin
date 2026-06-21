import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Select, ListBox, Spinner } from '@heroui/react'
import { MASKED } from '../../../api/client'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import { DiscordMemberPicker } from '../../DiscordMemberPicker'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'
import { RolePicker } from '../fields/RolePicker'
import type { AuthPanelProps } from './interfaces'

export const AuthPanel: React.FC<AuthPanelProps> = ({
  cfg, set, setBool, discordRoles, rolesLoading, loadDiscordRoles,
}) => {
  const { t } = useTranslation()
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <Panel>
        <SectionLabel>{t('settings.sections.authDashboard')}</SectionLabel>
        <div className="flex flex-col gap-1 -mt-1">
          <p className="text-sm text-muted">{t('settings.auth.hint')}</p>
        </div>
        <TwoColumnGrid>
          <div className="sm:col-span-2">
            <CheckboxField
              label={t('settings.auth.enabled')}
              checked={cfg.auth_enabled}
              onChange={setBool('auth_enabled')}
            />
          </div>
          <div className="sm:col-span-2">
            <CheckboxField
              label={t('settings.auth.guestEnabled')}
              hint={t('settings.auth.guestEnabledHint')}
              checked={cfg.auth_guest_enabled}
              onChange={setBool('auth_guest_enabled')}
            />
          </div>
          <FieldRow label={t('settings.auth.localUsername')}>
            <TextInput value={cfg.auth_local_username} onChange={set('auth_local_username')} placeholder="admin" />
          </FieldRow>
          <FieldRow label={t('settings.auth.localPassword')} hint={t('settings.auth.localPasswordHint')}>
            <TextInput
              value={cfg.auth_local_password_new ?? ''}
              onChange={set('auth_local_password_new')}
              type="password"
              placeholder={cfg.auth_local_password_hash ? MASKED : ''}
            />
          </FieldRow>
          <FieldRow label={t('settings.auth.sessionTtl')} hint={t('settings.auth.sessionTtlHint')}>
            <TextInput
              value={cfg.auth_session_ttl_hours ? String(cfg.auth_session_ttl_hours) : ''}
              onChange={set('auth_session_ttl_hours')}
              placeholder="24"
              type="number"
            />
          </FieldRow>
          <FieldRow label={t('settings.auth.cookiePolicy')}>
            <Select
              selectedKey={(cfg.auth_cookie_samesite || 'lax').toLowerCase()}
              onSelectionChange={(k) => set('auth_cookie_samesite')(String(k))}
              className="w-full"
              aria-label={t('settings.auth.cookiePolicy')}
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  <ListBox.Item id="lax" textValue={t('settings.auth.cookieLax')}>
                    {t('settings.auth.cookieLax')}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                  <ListBox.Item id="strict" textValue={t('settings.auth.cookieStrict')}>
                    {t('settings.auth.cookieStrict')}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                  <ListBox.Item id="none" textValue={t('settings.auth.cookieNone')}>
                    {t('settings.auth.cookieNone')}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                </ListBox>
              </Select.Popover>
            </Select>
          </FieldRow>
          <div className="sm:col-span-2 flex flex-col gap-1 rounded-[var(--radius)] bg-surface-secondary/40 border border-border p-3 text-xs text-muted">
            <p>
              <strong className="text-foreground">{t('settings.auth.cookieLax')}</strong>
              {' — '}
              {t('settings.auth.cookieLaxDesc')}
            </p>
            <p>
              <strong className="text-foreground">{t('settings.auth.cookieStrict')}</strong>
              {' — '}
              {t('settings.auth.cookieStrictDesc')}
            </p>
            <p>
              <strong className="text-foreground">{t('settings.auth.cookieNone')}</strong>
              {' — '}
              {t('settings.auth.cookieNoneDesc')}
            </p>
          </div>
        </TwoColumnGrid>
      </Panel>

      <Panel>
        <SectionLabel>{t('settings.sections.authDiscord')}</SectionLabel>
        <div className="flex flex-col gap-1 -mt-1">
          <p className="text-sm text-muted">{t('settings.auth.discordHint')}</p>
          <p className="text-sm text-muted">{t('settings.auth.discordStep1')}</p>
          <p className="text-sm text-muted">{t('settings.auth.discordStep2')}</p>
          <p className="text-sm text-muted">{t('settings.auth.discordStep3')}</p>
        </div>
        <TwoColumnGrid>
          <div className="sm:col-span-2">
            <CheckboxField
              label={t('settings.auth.discordEnabled')}
              checked={cfg.auth_discord_enabled}
              onChange={setBool('auth_discord_enabled')}
            />
          </div>
          <FieldRow label={t('settings.auth.clientId')}>
            <TextInput value={cfg.auth_discord_client_id} onChange={set('auth_discord_client_id')} placeholder="123456789012345678" />
          </FieldRow>
          <FieldRow label={t('settings.auth.clientSecret')}>
            <TextInput value={cfg.auth_discord_client_secret} onChange={set('auth_discord_client_secret')} type="password" placeholder={MASKED} />
          </FieldRow>
          <div className="sm:col-span-2">
            <FieldRow label={t('settings.auth.redirectUrl')} hint={t('settings.auth.redirectUrlHint')}>
              <TextInput value={cfg.auth_discord_redirect_url} onChange={set('auth_discord_redirect_url')} placeholder={`${window.location.origin}/api/v1/auth/discord/callback`} />
            </FieldRow>
          </div>
        </TwoColumnGrid>
      </Panel>

      <Panel>
        <div className="flex items-center justify-between">
          <SectionLabel>{t('settings.sections.authOwners')}</SectionLabel>
          <Button size="sm" variant="ghost" onPress={loadDiscordRoles} isDisabled={rolesLoading}>
            {rolesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            {' '}
            {t('settings.auth.refreshRoles')}
          </Button>
        </div>
        <div className="flex flex-col gap-1 -mt-1">
          <p className="text-sm text-muted">{t('settings.auth.ownersHint')}</p>
        </div>
        <TwoColumnGrid>
          <FieldRow label={t('settings.auth.ownerIds')} hint={t('settings.auth.ownerIdsHint')}>
            <DiscordMemberPicker
              value={cfg.auth_owner_discord_ids}
              onChange={set('auth_owner_discord_ids')}
              ariaLabel={t('settings.auth.ownerIds')}
            />
          </FieldRow>
          <RolePicker
            label={t('settings.auth.ownerRoles')}
            hint={t('settings.auth.ownerRolesHint')}
            value={cfg.auth_owner_role_ids}
            onChange={set('auth_owner_role_ids')}
            roles={discordRoles}
          />
        </TwoColumnGrid>
      </Panel>
    </div>
  )
}
