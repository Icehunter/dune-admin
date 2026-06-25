import * as React from 'react'
import { Button, Chip, Spinner, Switch, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { authApi } from '../../api/client'
import type { AuthLocalUser, PermissionsData } from '../../api/client'
import { ConfirmDialog, FieldInput, Icon, PageHeader, SectionLabel } from '../../dune-ui'
import { CapabilityGrid } from './CapabilityGrid'

// PermissionsTab is the editor for the role→capability matrix and local
// dashboard users. Accessible to owners and sessions with auth:manage.
export const PermissionsTab: React.FC = () => {
  const { t } = useTranslation()
  const [data, setData] = React.useState<PermissionsData | null>(null)
  const [matrix, setMatrix] = React.useState<Record<string, string[]>>({})
  const [users, setUsers] = React.useState<AuthLocalUser[]>([])
  const [loading, setLoading] = React.useState(false)
  const [saving, setSaving] = React.useState(false)
  const [dirty, setDirty] = React.useState(false)
  const [section, setSection] = React.useState<'permissions' | 'users'>('permissions')
  // New-user form state.
  const [newUsername, setNewUsername] = React.useState('')
  const [newPassword, setNewPassword] = React.useState('')
  const [creating, setCreating] = React.useState(false)
  const [deleteTarget, setDeleteTarget] = React.useState<string | null>(null)
  // Per-user password reset values, keyed by username.
  const [passwordEdits, setPasswordEdits] = React.useState<Record<string, string>>({})

  const load = async (): Promise<void> => {
    setLoading(true)
    try {
      const [d, u] = await Promise.all([authApi.permissions.get(), authApi.users.list()])
      setData(d)
      setMatrix(d.matrix)
      setUsers(u)
      setDirty(false)
    }
    catch (e) {
      toast.danger(t('permissions.loadFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setLoading(false)
    }
  }

  React.useEffect(() => {
    void Promise.resolve().then(load)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const toggleMatrix = (roleID: string, cap: string, on: boolean) => {
    setMatrix((m) => {
      const current = new Set(m[roleID] ?? [])
      if (on) current.add(cap)
      else current.delete(cap)
      const next = { ...m }
      if (current.size === 0) delete next[roleID]
      else next[roleID] = [...current].sort()
      return next
    })
    setDirty(true)
  }

  const saveMatrix = async () => {
    setSaving(true)
    try {
      await authApi.permissions.save(matrix)
      setDirty(false)
      toast.success(t('permissions.saved'))
    }
    catch (e) {
      toast.danger(t('permissions.saveFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const saveUser = async (user: AuthLocalUser, overrides?: Partial<{ capabilities: string[], enabled: boolean }>) => {
    const password = passwordEdits[user.username] || undefined
    try {
      await authApi.users.save(user.username, {
        password,
        capabilities: overrides?.capabilities ?? user.capabilities,
        enabled: overrides?.enabled ?? user.enabled,
      })
      setPasswordEdits((p) => ({ ...p, [user.username]: '' }))
      toast.success(t('permissions.userSaved', { username: user.username }))
      setUsers(await authApi.users.list())
    }
    catch (e) {
      toast.danger(t('permissions.userSaveFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
  }

  const createUser = async () => {
    if (!newUsername || !newPassword) return
    setCreating(true)
    try {
      await authApi.users.save(newUsername.trim(), { password: newPassword, capabilities: [], enabled: true })
      setNewUsername('')
      setNewPassword('')
      toast.success(t('permissions.userSaved', { username: newUsername.trim() }))
      setUsers(await authApi.users.list())
    }
    catch (e) {
      toast.danger(t('permissions.userSaveFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setCreating(false)
    }
  }

  const deleteUser = async (username: string) => {
    try {
      await authApi.users.remove(username)
      toast.success(t('permissions.userDeleted', { username }))
      setUsers(await authApi.users.list())
    }
    catch (e) {
      toast.danger(t('permissions.userSaveFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
  }

  if (loading && !data) {
    return <div className="flex-1 flex items-center justify-center"><Spinner /></div>
  }

  const capabilities = data?.capabilities ?? []
  const guildRoles = data?.guild_roles ?? []
  const knownIDs = new Set([...guildRoles.map((r) => r.id), 'guest', 'default'])
  const orphanIDs = Object.keys(matrix).filter((id) => !knownIDs.has(id))
  const roleRows = [
    ...guildRoles.map((r) => ({ id: r.id, name: r.name, hint: r.id })),
    ...orphanIDs.map((id) => ({ id, name: id, hint: t('permissions.unknownRole') })),
  ]
  // The Default row is the cascade root; every other principal inherits its
  // capabilities (locked-on, editable only here).
  const defaultCaps = matrix.default ?? []
  const pseudoRows = [
    { id: 'default', name: t('permissions.defaultRow'), hint: t('permissions.defaultRowDesc') },
    { id: 'guest', name: t('permissions.guestRow'), hint: t('permissions.guestRowDesc') },
  ]

  const resetDefaults = () => {
    setMatrix((m) => ({ ...m, default: [...(data?.seed_defaults ?? [])].sort() }))
    setDirty(true)
  }

  const roleCard = (row: { id: string, name: string, hint: string }) => {
    const isDefault = row.id === 'default'
    return (
      <div key={row.id} className="dune-lift shrink-0 bg-surface border border-border rounded-[var(--radius)] p-4 flex flex-col gap-3">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-semibold text-foreground">{row.name}</span>
          <span className="text-xs text-muted font-mono">{row.hint}</span>
          {(matrix[row.id]?.length ?? 0) > 0 && (
            <Chip size="sm" color="accent" variant="soft">{matrix[row.id].length}</Chip>
          )}
          {isDefault && (
            <Button size="sm" variant="ghost" className="ml-auto" onPress={resetDefaults}>
              <Icon name="rotate-ccw" />
              {' '}
              {t('permissions.resetDefaults')}
            </Button>
          )}
        </div>
        <CapabilityGrid
          capabilities={capabilities}
          selected={matrix[row.id] ?? []}
          inherited={isDefault ? [] : defaultCaps}
          onToggle={(cap, on) => toggleMatrix(row.id, cap, on)}
        />
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      {/* Segment switcher — each section renders its own header below. */}
      <div className="shrink-0 flex items-center justify-end">
        <Segment
          selectedKey={section}
          onSelectionChange={(k) => setSection(k as 'permissions' | 'users')}
          size="sm"
          aria-label={t('permissions.title')}
        >
          <Segment.Item id="permissions">
            <Segment.Separator />
            {t('permissions.title')}
          </Segment.Item>
          <Segment.Item id="users">
            <Segment.Separator />
            {t('permissions.localUsers')}
          </Segment.Item>
        </Segment>
      </div>

      {section === 'permissions' && (
        <PageHeader
          title={t('permissions.title')}
          subtitle={t('permissions.subtitle')}
          onRefresh={load}
          loading={loading}
        >
          <Button size="sm" onPress={saveMatrix} isDisabled={saving || !dirty}>
            {saving
              ? <Spinner size="sm" color="current" />
              : (
                  <React.Fragment>
                    <Icon name="save" />
                    {' '}
                    {t('common.save')}
                  </React.Fragment>
                )}
          </Button>
        </PageHeader>
      )}

      {section === 'permissions' && (
        <p className="shrink-0 text-xs text-muted">{t('permissions.cascadeHint')}</p>
      )}

      {section === 'users' && (
        <PageHeader
          title={t('permissions.localUsers')}
          subtitle={t('permissions.localUsersHint')}
          onRefresh={load}
          loading={loading}
        />
      )}

      {section === 'permissions' && (
        <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pr-1 pb-4">
          {/* Built-in pseudo roles */}
          <SectionLabel>{t('permissions.builtinSection')}</SectionLabel>
          {pseudoRows.map(roleCard)}

          {/* Discord guild roles */}
          <SectionLabel>{t('permissions.rolesSection')}</SectionLabel>
          {roleRows.length === 0 && <p className="text-sm text-muted">{t('permissions.noRoles')}</p>}
          {roleRows.map(roleCard)}
        </div>
      )}

      {section === 'users' && (
        <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pr-1 pb-4">
          {/* Add user */}
          <div className="shrink-0 bg-surface border border-border border-dashed rounded-[var(--radius)] p-4 flex items-center gap-2 flex-wrap">
            <FieldInput
              value={newUsername}
              onChange={setNewUsername}
              placeholder={t('permissions.username')}
              ariaLabel={t('permissions.username')}
              className="w-44"
            />
            <FieldInput
              type="password"
              value={newPassword}
              onChange={setNewPassword}
              placeholder={t('auth.password')}
              ariaLabel={t('auth.password')}
              className="w-44"
            />
            <Button size="sm" onPress={() => void createUser()} isDisabled={creating || !newUsername || !newPassword}>
              {creating
                ? <Spinner size="sm" color="current" />
                : (
                    <React.Fragment>
                      <Icon name="user-plus" />
                      {' '}
                      {t('permissions.addUser')}
                    </React.Fragment>
                  )}
            </Button>
          </div>

          {users.map((user) => (
            <div key={user.username} className="dune-lift shrink-0 bg-surface border border-border rounded-[var(--radius)] p-4 flex flex-col gap-3">
              <div className="flex items-center gap-3 flex-wrap">
                <span className="font-semibold text-foreground">{user.username}</span>
                <Switch
                  size="sm"
                  isSelected={user.enabled}
                  onChange={(on: boolean) => void saveUser(user, { enabled: on })}
                >
                  <Switch.Content className="text-xs">
                    <Switch.Control><Switch.Thumb /></Switch.Control>
                    {t('permissions.userEnabled')}
                  </Switch.Content>
                </Switch>
                <span className="flex-1" />
                <FieldInput
                  type="password"
                  value={passwordEdits[user.username] ?? ''}
                  onChange={(v) => setPasswordEdits((p) => ({ ...p, [user.username]: v }))}
                  placeholder={t('permissions.newPasswordOptional')}
                  ariaLabel={t('permissions.newPasswordOptional')}
                  className="w-48"
                />
                <Button size="sm" variant="outline" onPress={() => void saveUser(user)}>
                  {t('common.save')}
                </Button>
                <Button size="sm" variant="danger-soft" onPress={() => setDeleteTarget(user.username)}>
                  <Icon name="trash-2" />
                </Button>
              </div>
              <CapabilityGrid
                capabilities={capabilities}
                selected={user.capabilities}
                inherited={defaultCaps}
                onToggle={(cap, on) => {
                  const next = on
                    ? [...user.capabilities, cap].sort()
                    : user.capabilities.filter((c) => c !== cap)
                  void saveUser(user, { capabilities: next })
                }}
              />
            </div>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('permissions.deleteUserTitle')}
        description={t('permissions.deleteUserBody', { username: deleteTarget ?? '' })}
        confirmLabel={t('common.delete')}
        onConfirm={() => {
          if (deleteTarget) void deleteUser(deleteTarget)
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
