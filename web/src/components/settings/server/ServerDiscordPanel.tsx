import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type {
  DiscordChannelOption, DiscordGuild, DiscordGuildOption, DiscordServerLink,
} from '../../../api/client'
import { ConfirmDialog, Icon, Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { SearchableSelect } from '../fields/SearchableSelect'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

export interface ServerDiscordPanelProps {
  /** The persisted server (id > 0) whose single Discord link is edited here. */
  serverId: number
}

const emptyLink = (serverId: number): DiscordServerLink => ({
  server_id: serverId,
  guild_id: '',
  announce_channel_id: '',
  status_channel_id: '',
  status_enabled: false,
  status_interval_seconds: 60,
})

// ServerDiscordPanel is THIS server's single Discord link surface. Each game
// server links to exactly one guild and posts its announce/status embeds to its
// own channels. Loads via api.discord.servers.list() filtered to this server,
// saves via api.discord.servers.set, and clears via api.discord.servers.unlink.
// Capability roles for the linked guild are configured in Settings → Guilds.
export const ServerDiscordPanel: React.FC<ServerDiscordPanelProps> = ({ serverId }) => {
  const { t } = useTranslation()
  const [link, setLink] = React.useState<DiscordServerLink>(() => emptyLink(serverId))
  const [hasLink, setHasLink] = React.useState(false)
  const [loading, setLoading] = React.useState(false)
  const [saving, setSaving] = React.useState(false)
  const [unlinkOpen, setUnlinkOpen] = React.useState(false)
  const [guilds, setGuilds] = React.useState<DiscordGuild[]>([])
  const [guildNames, setGuildNames] = React.useState<Record<string, string>>({})
  const [channels, setChannels] = React.useState<DiscordChannelOption[]>([])

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.discord.servers.list())
      .then((links) => {
        const found = links.find((l) => l.server_id === serverId)
        setHasLink(!!found)
        setLink(found ?? emptyLink(serverId))
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t, serverId])

  React.useEffect(() => {
    load()
  }, [load])

  // Configured guilds (added in Settings → Guilds) populate the guild dropdown;
  // the bot's guild membership supplies human names for the labels. Both are
  // best-effort — a failure just yields a manual-ID fallback / raw-id labels.
  React.useEffect(() => {
    Promise.all([api.discord.guilds.list(), api.discord.availableGuilds().catch(() => [] as DiscordGuildOption[])])
      .then(([gs, avail]) => {
        setGuilds(gs)
        setGuildNames(Object.fromEntries(avail.map((a) => [a.id, a.name])))
      })
      .catch(() => undefined)
  }, [])

  // Load the selected guild's postable channels for the announce/status pickers.
  React.useEffect(() => {
    const gid = link.guild_id.trim()
    if (!gid) {
      Promise.resolve().then(() => setChannels([]))
      return
    }
    api.discord.channels(gid).then(setChannels).catch(() => setChannels([]))
  }, [link.guild_id])

  const setField = (patch: Partial<DiscordServerLink>) => setLink((prev) => ({ ...prev, ...patch }))
  const guildLabel = (id: string) => (guildNames[id] ? `${guildNames[id]} (${id})` : id)

  const save = () => {
    const guildId = link.guild_id.trim()
    if (!guildId) {
      toast.danger(t('discordGuilds.guildIdRequired'))
      return
    }
    setSaving(true)
    api.discord.servers.set(serverId, {
      guild_id: guildId,
      announce_channel_id: link.announce_channel_id.trim(),
      status_channel_id: link.status_channel_id.trim(),
      status_enabled: link.status_enabled,
      status_interval_seconds: link.status_interval_seconds || 60,
    })
      .then(() => {
        toast.success(t('discordGuilds.serverDiscord.linked'))
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const confirmUnlink = () => {
    api.discord.servers.unlink(serverId)
      .then(() => {
        toast.success(t('discordGuilds.serverDiscord.unlinked'))
        setHasLink(false)
        setLink(emptyLink(serverId))
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setUnlinkOpen(false))
  }

  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <Panel>
        <div className="flex items-center justify-between">
          <SectionLabel>{t('discordGuilds.serverDiscord.title')}</SectionLabel>
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
            {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            {' '}
            {t('common.refresh')}
          </Button>
        </div>
        <p className="text-xs text-muted -mt-1">{t('discordGuilds.serverDiscord.subtitle')}</p>
        <div className="flex items-start gap-2 text-sm text-muted">
          <Icon name="info" className="mt-0.5 shrink-0" />
          <p>{t('discordGuilds.serverDiscord.rolesHint')}</p>
        </div>

        <FieldRow label={t('discordGuilds.serverDiscord.guildPick')} hint={t('discordGuilds.serverDiscord.guildPickHint')}>
          {guilds.length > 0
            ? (
                <SearchableSelect
                  value={link.guild_id}
                  onChange={(id) => setField({ guild_id: id, announce_channel_id: '', status_channel_id: '' })}
                  options={guilds.map((g) => ({ id: g.guild_id, label: guildLabel(g.guild_id) }))}
                  placeholder={t('discordGuilds.selectGuild')}
                  ariaLabel={t('discordGuilds.serverDiscord.guildPick')}
                />
              )
            : (
                <div className="flex flex-col gap-1">
                  <TextInput
                    value={link.guild_id}
                    onChange={(v) => setField({ guild_id: v })}
                    placeholder="123456789012345678"
                  />
                  <span className="text-xs text-muted">{t('discordGuilds.serverDiscord.noGuilds')}</span>
                </div>
              )}
        </FieldRow>

        <TwoColumnGrid>
          <FieldRow label={t('discordGuilds.announceChannel')} hint={t('discordGuilds.announceChannelHint')}>
            {channels.length > 0
              ? (
                  <SearchableSelect
                    value={link.announce_channel_id}
                    onChange={(id) => setField({ announce_channel_id: id })}
                    options={channels.map((c) => ({ id: c.id, label: `#${c.name}` }))}
                    placeholder={t('discordGuilds.selectChannel')}
                    ariaLabel={t('discordGuilds.announceChannel')}
                  />
                )
              : (
                  <TextInput
                    value={link.announce_channel_id}
                    onChange={(v) => setField({ announce_channel_id: v })}
                    placeholder="444444444444444444"
                  />
                )}
          </FieldRow>
          <FieldRow label={t('discordGuilds.statusChannel')} hint={t('discordGuilds.statusChannelHint')}>
            {channels.length > 0
              ? (
                  <SearchableSelect
                    value={link.status_channel_id}
                    onChange={(id) => setField({ status_channel_id: id })}
                    options={channels.map((c) => ({ id: c.id, label: `#${c.name}` }))}
                    placeholder={t('discordGuilds.selectChannel')}
                    ariaLabel={t('discordGuilds.statusChannel')}
                  />
                )
              : (
                  <TextInput
                    value={link.status_channel_id}
                    onChange={(v) => setField({ status_channel_id: v })}
                    placeholder="555555555555555555"
                  />
                )}
          </FieldRow>
          <div className="sm:col-span-2">
            <CheckboxField
              label={t('discordGuilds.statusEnabled')}
              hint={t('discordGuilds.statusEnabledHint')}
              checked={link.status_enabled}
              onChange={(v) => setField({ status_enabled: v })}
            />
          </div>
          <FieldRow label={t('discordGuilds.statusInterval')} hint={t('discordGuilds.statusIntervalHint')}>
            <TextInput
              value={link.status_interval_seconds ? String(link.status_interval_seconds) : ''}
              onChange={(v) => setField({ status_interval_seconds: Number(v) || 0 })}
              placeholder="60"
              type="number"
            />
          </FieldRow>
        </TwoColumnGrid>

        <div className="flex items-center gap-2 pt-1">
          <Button size="sm" onPress={save} isDisabled={saving || loading}>
            {saving
              ? (
                  <>
                    <Spinner size="sm" color="current" />
                    {' '}
                    {t('common.saving')}
                  </>
                )
              : (
                  <>
                    <Icon name="save" />
                    {' '}
                    {t('common.save')}
                  </>
                )}
          </Button>
          {hasLink && (
            <Button size="sm" variant="danger-soft" onPress={() => setUnlinkOpen(true)} isDisabled={saving || loading}>
              <Icon name="trash-2" />
              {' '}
              {t('discordGuilds.serverDiscord.unlink')}
            </Button>
          )}
        </div>
      </Panel>

      <ConfirmDialog
        open={unlinkOpen}
        title={t('discordGuilds.serverDiscord.unlinkTitle')}
        description={t('discordGuilds.serverDiscord.unlinkConfirm')}
        confirmLabel={t('discordGuilds.serverDiscord.unlink')}
        onConfirm={confirmUnlink}
        onCancel={() => setUnlinkOpen(false)}
      />
    </div>
  )
}
