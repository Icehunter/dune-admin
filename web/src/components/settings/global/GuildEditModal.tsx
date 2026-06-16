import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { DiscordGuild, DiscordGuildOption } from '../../../api/client'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import type { DiscordRole } from '../../types'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { SearchableSelect } from '../fields/SearchableSelect'
import { RolePicker } from '../fields/RolePicker'

export interface GuildEditModalProps {
  open: boolean
  /** When editing, the existing guild; null when adding a new one. */
  existing: DiscordGuild | null
  /** Guild ids already configured — hidden from the add dropdown and rejected on
   *  save so the same guild can't be configured twice. */
  takenGuildIds?: string[]
  onClose: () => void
  /** Called after a successful upsert so the parent can reload its list. */
  onSaved: () => void
}

const emptyGuild = (): DiscordGuild => ({
  guild_id: '',
  roles_viewer: '',
  roles_economy: '',
  roles_admin: '',
})

// GuildEditModal is the add/edit surface for one Discord guild. A guild now
// carries ONLY its three guild-level capability role pickers (driven by the
// guild's own roles, loaded via api.discord.roles). Servers link to a guild
// from their own per-server Discord tab — not here.
export const GuildEditModal: React.FC<GuildEditModalProps> = ({
  open, existing, takenGuildIds = [], onClose, onSaved,
}) => {
  const { t } = useTranslation()
  const [g, setG] = React.useState<DiscordGuild>(() => existing ?? emptyGuild())
  const [saving, setSaving] = React.useState(false)
  const [roles, setRoles] = React.useState<DiscordRole[]>([])
  const [rolesLoading, setRolesLoading] = React.useState(false)
  const [availGuilds, setAvailGuilds] = React.useState<DiscordGuildOption[]>([])

  const isEdit = !!existing

  // The guilds the bot belongs to, so the operator can pick by name instead of
  // pasting a snowflake. Best-effort: if the bot is offline / token unset, the
  // list stays empty and the form falls back to a manual ID input.
  React.useEffect(() => {
    if (!open) return
    api.discord.availableGuilds().then(setAvailGuilds).catch(() => setAvailGuilds([]))
  }, [open])

  // Reset the form whenever the modal (re)opens for a different guild.
  React.useEffect(() => {
    if (open) Promise.resolve().then(() => setG(existing ?? emptyGuild()))
  }, [open, existing])

  const loadRoles = React.useCallback((guildId: string) => {
    const id = guildId.trim()
    if (!id) {
      Promise.resolve().then(() => setRoles([]))
      return
    }
    Promise.resolve()
      .then(() => setRolesLoading(true))
      .then(() => api.discord.roles(id))
      .then(setRoles)
      .catch(() => setRoles([]))
      .finally(() => setRolesLoading(false))
  }, [])

  React.useEffect(() => {
    if (open) loadRoles(existing?.guild_id ?? '')
  }, [open, existing, loadRoles])

  const setRoleCsv = (key: 'roles_viewer' | 'roles_economy' | 'roles_admin') => (v: string) =>
    setG((prev) => ({ ...prev, [key]: v }))

  // The add dropdown only offers guilds that aren't already configured (the one
  // being edited stays listed so it can be re-selected).
  const guildOptions = React.useMemo(() => {
    const taken = new Set(takenGuildIds.filter((id) => id !== existing?.guild_id))
    return availGuilds
      .filter((x) => !taken.has(x.id))
      .map((x) => ({ id: x.id, label: `${x.name} (${x.id})` }))
  }, [availGuilds, takenGuildIds, existing])

  const save = () => {
    const guildId = g.guild_id.trim()
    if (!guildId) {
      toast.danger(t('discordGuilds.guildIdRequired'))
      return
    }
    if (!isEdit && takenGuildIds.includes(guildId)) {
      toast.danger(t('discordGuilds.guildExists'))
      return
    }
    setSaving(true)
    api.discord.guilds.upsert({ ...g, guild_id: guildId })
      .then(() => {
        toast.success(t('discordGuilds.saved'))
        onSaved()
        onClose()
      })
      .catch((e: unknown) =>
        toast.danger(t('discordGuilds.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="lg" scroll="inside" className="w-full min-w-[50vw]">
        <Modal.Dialog className="p-8 dialog-surface-alt max-h-[90vh]">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {isEdit ? t('discordGuilds.editTitle') : t('discordGuilds.addTitle')}
            </Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4 overflow-y-auto min-h-0 max-h-[70vh] pr-1">
            <Panel>
              <SectionLabel>{t('discordGuilds.sections.guild')}</SectionLabel>
              <FieldRow label={t('discordGuilds.guildId')} hint={t('discordGuilds.guildIdHint')}>
                <div className="flex gap-1.5">
                  {guildOptions.length > 0
                    ? (
                        <SearchableSelect
                          value={g.guild_id}
                          onChange={(id) => {
                            setG((prev) => ({ ...prev, guild_id: id }))
                            loadRoles(id)
                          }}
                          options={guildOptions}
                          placeholder={t('discordGuilds.selectGuild')}
                          ariaLabel={t('discordGuilds.guildId')}
                        />
                      )
                    : (
                        <TextInput
                          value={g.guild_id}
                          onChange={(v) => setG((prev) => ({ ...prev, guild_id: v }))}
                          placeholder="123456789012345678"
                        />
                      )}
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => loadRoles(g.guild_id)}
                    isDisabled={rolesLoading || !g.guild_id.trim()}
                  >
                    {rolesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                    {' '}
                    {t('discordGuilds.loadRoles')}
                  </Button>
                </div>
              </FieldRow>
            </Panel>

            <Panel>
              <SectionLabel>{t('discordGuilds.sections.roles')}</SectionLabel>
              <p className="text-xs text-muted -mt-1">{t('discordGuilds.rolesHint')}</p>
              {/* Container query: stacks the pickers when the modal itself is
                  narrow (not just the viewport), so a cramped dialog reflows to
                  one column instead of squeezing two. */}
              <div className="@container mt-1">
                <div className="grid grid-cols-1 @xl:grid-cols-2 gap-3">
                  <RolePicker
                    label={t('discordGuilds.rolesViewer')}
                    hint={t('discordGuilds.rolesViewerHint')}
                    value={g.roles_viewer}
                    onChange={setRoleCsv('roles_viewer')}
                    roles={roles}
                  />
                  <RolePicker
                    label={t('discordGuilds.rolesEconomy')}
                    hint={t('discordGuilds.rolesEconomyHint')}
                    value={g.roles_economy}
                    onChange={setRoleCsv('roles_economy')}
                    roles={roles}
                  />
                  <RolePicker
                    label={t('discordGuilds.rolesAdmin')}
                    hint={t('discordGuilds.rolesAdminHint')}
                    value={g.roles_admin}
                    onChange={setRoleCsv('roles_admin')}
                    roles={roles}
                  />
                </div>
              </div>
            </Panel>
          </Modal.Body>
          <Modal.Footer className="flex items-center gap-2">
            <span className="flex-1" />
            <Button size="sm" onPress={save} isDisabled={saving}>
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
            <Button size="sm" variant="tertiary" slot="close" onPress={onClose}>
              {t('common.cancel')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
