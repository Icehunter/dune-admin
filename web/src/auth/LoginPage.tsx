import * as React from 'react'
import { Button, Card, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { AuthContext } from './context'
import { authApi, ApiError } from '../api/client'
import { FieldInput, Icon } from '../dune-ui'

// LoginPage is the full-screen gate shown when backend auth is enabled and
// there is no valid session. Offers whichever login methods are configured.
export const LoginPage: React.FC = () => {
  const { methods, login, refresh } = React.useContext(AuthContext)
  const [guestSubmitting, setGuestSubmitting] = React.useState(false)

  const continueAsGuest = async () => {
    setGuestSubmitting(true)
    try {
      await authApi.guest()
      await refresh()
    }
    catch {
      setError(t('auth.loginFailed'))
    }
    finally {
      setGuestSubmitting(false)
    }
  }
  const { t } = useTranslation()
  const [username, setUsername] = React.useState('')
  const [password, setPassword] = React.useState('')
  const [submitting, setSubmitting] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  // Surface the not-a-member error from the Discord callback redirect
  // (/#login-error=not-a-member).
  React.useEffect(() => {
    if (window.location.hash.includes('login-error=not-a-member')) {
      window.location.hash = ''
      void Promise.resolve().then(() => setError(t('auth.notAMember')))
    }
  }, [t])

  const submit = async () => {
    if (!username || !password || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await login(username, password)
    }
    catch (e) {
      if (e instanceof ApiError && e.status === 429) {
        setError(t('auth.rateLimited'))
      }
      else {
        setError(t('auth.loginFailed'))
      }
    }
    finally {
      setSubmitting(false)
    }
  }

  const noMethods = !methods.local && !methods.discord

  return (
    <div className="h-screen flex items-center justify-center bg-background">
      <Card className="w-full max-w-sm p-8 flex flex-col gap-5 dune-lift">
        <div className="flex flex-col items-center gap-2">
          <div className="bg-primary rounded-full h-12 flex items-center justify-center">
            <img src="/dune-admin-logo-primary.svg" alt="Dune Admin" className="size-12" />
            <h1 className="text-lg font-bold uppercase tracking-[0.2em] text-accent">{t('app.title')}</h1>
          </div>
          <p className="text-sm text-muted">{t('auth.signInSubtitle')}</p>
        </div>

        {noMethods && (
          <div className="text-sm text-muted flex flex-col gap-2">
            <p>{t('auth.noMethods')}</p>
            <code className="bg-surface px-2 py-1 rounded text-xs">dune-admin --set-password</code>
            <p>{t('auth.noMethodsHint')}</p>
          </div>
        )}

        {methods.local && (
          <form
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void submit()
            }}
          >
            <FieldInput
              value={username}
              onChange={setUsername}
              placeholder={t('auth.username')}
              ariaLabel={t('auth.username')}
            />
            <FieldInput
              type="password"
              value={password}
              onChange={setPassword}
              placeholder={t('auth.password')}
              ariaLabel={t('auth.password')}
            />
            <Button type="submit" className="w-full mt-1" isDisabled={submitting || !username || !password}>
              {submitting
                ? <Spinner size="sm" color="current" />
                : t('auth.signIn')}
            </Button>
          </form>
        )}

        {methods.local && methods.discord && (
          <div className="flex items-center gap-3 text-xs text-muted">
            <span className="flex-1 border-t border-border" />
            {t('auth.or')}
            <span className="flex-1 border-t border-border" />
          </div>
        )}

        {methods.discord && (
          <Button
            variant="outline"
            className="w-full"
            onPress={() => {
              window.location.href = authApi.discordLoginUrl()
            }}
          >
            <Icon name="message-circle" />
            {' '}
            {t('auth.signInWithDiscord')}
          </Button>
        )}

        {methods.guest && (
          <Button
            variant="ghost"
            className="w-full text-muted"
            isDisabled={guestSubmitting}
            onPress={() => void continueAsGuest()}
          >
            {guestSubmitting ? <Spinner size="sm" color="current" /> : t('auth.continueAsGuest')}
          </Button>
        )}

        {error && <p className="text-sm text-danger text-center">{error}</p>}
      </Card>
    </div>
  )
}
