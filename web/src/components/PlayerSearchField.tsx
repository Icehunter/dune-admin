import * as React from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { SearchField, toast } from '@heroui/react'
import { api } from '../api/client'
import type { Player } from '../api/client'
import { useDebounce } from '../hooks/useDebounce'
import type { PlayerSearchFieldProps } from './interfaces'

/**
 * Debounced player search with a capped suggestion dropdown. The canonical
 * "select a player" control — renders at most `resultLimit` rows so the full
 * roster never hits the DOM.
 *
 * When mounted inside a modal ([role="dialog"] ancestor), the dropdown is
 * portaled into the dialog element so it stays within the modal's DOM tree
 * (preventing outside-click dismiss). Position:fixed coords are made
 * dialog-relative to cancel any CSS transform the modal animation applies.
 * The host modal must have overflow:visible so the dropdown can extend past
 * the dialog boundary.
 */
export const PlayerSearchField: React.FC<PlayerSearchFieldProps> = ({
  onSelect,
  ariaLabel,
  placeholder,
  className,
  players,
  resultLimit = 3,
  filter,
  clearOnSelect = false,
  onClear,
}) => {
  const { t } = useTranslation()
  const [query, setQuery] = React.useState('')
  const [open, setOpen] = React.useState(false)
  const [loaded, setLoaded] = React.useState<Player[] | null>(null)
  const [loading, setLoading] = React.useState(false)
  const debouncedQuery = useDebounce(query)
  const wrapRef = React.useRef<HTMLDivElement>(null)
  // The nearest [role="dialog"] ancestor (if any) and the resolved portal
  // target. Set once on mount — the modal is already open when
  // PlayerSearchField mounts inside it.
  const [portalTarget, setPortalTarget] = React.useState<HTMLElement | null>(null)
  const [dialogEl, setDialogEl] = React.useState<HTMLElement | null>(null)
  const [dropdownStyle, setDropdownStyle] = React.useState<React.CSSProperties | null>(null)

  React.useEffect(() => {
    const dialog = wrapRef.current?.closest<HTMLElement>('[role="dialog"]') ?? null
    setDialogEl(dialog)
    setPortalTarget(dialog ?? document.body)
  }, [])

  const roster = players ?? loaded ?? []

  const ensureLoaded = () => {
    if (players || loaded || loading) return
    setLoading(true)
    api.players
      .list()
      .then(setLoaded)
      .catch((e: unknown) => {
        toast.danger(t('playerSearch.loadFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setLoading(false))
  }

  const base = filter ? roster.filter(filter) : roster
  const q = debouncedQuery.trim().toLowerCase()
  const hits = q
    ? base.filter((p) => p.name.toLowerCase().includes(q) || String(p.account_id).includes(q))
    : base
  const matches = hits.slice(0, resultLimit)

  const pick = (p: Player) => {
    setQuery(clearOnSelect ? '' : p.name)
    setOpen(false)
    onSelect(p)
  }

  const showDropdown = open && matches.length > 0

  // Compute where to render the portaled dropdown. When inside a dialog the
  // dropdown is portaled into that dialog element — position:fixed coords are
  // made relative to the dialog rect so they survive any CSS transform the
  // modal entrance animation may have applied. Without a dialog ancestor
  // (standalone use) we fall back to body-portal with plain viewport coords.
  React.useEffect(() => {
    if (!showDropdown) return
    const DROPDOWN_MAX_H = 288 // max-h-72

    const update = () => {
      if (!wrapRef.current) return
      const wr = wrapRef.current.getBoundingClientRect()
      const dr = dialogEl?.getBoundingClientRect()
      setDropdownStyle({
        position: 'fixed' as const,
        top: wr.bottom - (dr?.top ?? 0) + 4,
        left: wr.left - (dr?.left ?? 0),
        width: wr.width,
        maxHeight: DROPDOWN_MAX_H,
        zIndex: 9999,
      })
    }
    update()
    window.addEventListener('scroll', update, true)
    window.addEventListener('resize', update)
    return () => {
      window.removeEventListener('scroll', update, true)
      window.removeEventListener('resize', update)
    }
  }, [showDropdown, dialogEl])

  return (
    <div
      ref={wrapRef}
      className={`relative ${className ?? ''}`}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false)
      }}
    >
      <SearchField
        value={query}
        onChange={(v) => {
          setQuery(v)
          setOpen(true)
          if (v === '') onClear?.()
        }}
        onFocus={() => {
          ensureLoaded()
          setOpen(true)
        }}
        className="w-full"
        aria-label={ariaLabel}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input
            placeholder={loading ? t('playerSearch.loading') : (placeholder ?? t('playerSearch.placeholder'))}
            aria-label={ariaLabel}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setOpen(false)
            }}
          />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {showDropdown && dropdownStyle && portalTarget && createPortal(
        <div
          style={dropdownStyle}
          className="rounded-[var(--radius)] border border-border bg-surface overflow-y-auto shadow-lg"
        >
          {matches.map((p) => (
            <button
              key={p.account_id}
              type="button"
              className="w-full text-left px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover flex items-center justify-between gap-2"
              onMouseDown={(e) => {
                e.preventDefault()
                pick(p)
              }}
            >
              <span className="font-medium">{p.name}</span>
              <span className="text-muted font-mono">
                #
                {p.account_id}
                {' · '}
                {p.online_status}
              </span>
            </button>
          ))}
          <div className="px-3 py-1 text-[10px] text-muted border-t border-border select-none">
            {t('playerSearch.hint', { limit: resultLimit })}
          </div>
        </div>,
        portalTarget,
      )}
    </div>
  )
}
