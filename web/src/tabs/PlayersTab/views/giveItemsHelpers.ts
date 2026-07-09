const MAX_RESULTS = 100

// Deprecated/dev item templates are prefixed `d_`. They're non-functional
// when given but otherwise appear mixed into search results (#276).
const isDeprecatedTemplate = (id: string): boolean => id.startsWith('d_')

// escapeRegExp neutralizes regex metacharacters in a literal query segment
// before it's spliced into a glob pattern, so e.g. "." matches only itself.
const escapeRegExp = (segment: string): string => segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// globToRegExp turns a simple `*`-wildcard query into a case-insensitive
// RegExp — `*` becomes `.*`, everything else is escaped and matched literally.
const globToRegExp = (query: string): RegExp => {
  const pattern = query.split('*').map(escapeRegExp).join('.*')
  return new RegExp(pattern, 'i')
}

/**
 * filterTemplates narrows the item-template list for the Give Items search.
 *
 * - Always hides `d_`-prefixed (deprecated/dev) templates — non-functional
 *   when given, so there's no legitimate reason to surface them.
 * - `query` containing `*` is treated as a multi-char glob (matched against
 *   both id and name); otherwise falls back to a plain case-insensitive
 *   substring match, matching prior behavior.
 * - Results are capped at 100, same as before.
 */
export const filterTemplates = (
  templates: { id: string, name: string }[],
  query: string,
): { id: string, name: string }[] => {
  if (!query) return []

  const candidates = templates.filter((tpl) => !isDeprecatedTemplate(tpl.id))

  const matches = query.includes('*')
    ? (() => {
        const re = globToRegExp(query)
        return (tpl: { id: string, name: string }): boolean => re.test(tpl.id) || re.test(tpl.name)
      })()
    : (() => {
        const q = query.toLowerCase()
        return (tpl: { id: string, name: string }): boolean =>
          tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q)
      })()

  return candidates.filter(matches).slice(0, MAX_RESULTS)
}

/**
 * retainSkippedStaged computes the staging list to show after a partial give.
 *
 * The backend returns `given` (templates delivered). We remove from `staged`
 * the rows that were successfully given, keeping the skipped ones so the
 * operator can adjust quantities and retry.
 *
 * When the same template appears multiple times in `staged`, we remove one
 * staged row per entry in `given` — i.e. given acts as a consume-count.
 */
export const retainSkippedStaged = <T extends { template: string }>(
  staged: T[],
  given: string[],
): T[] => {
  // Build a mutable removal count keyed by template.
  const removeCount = new Map<string, number>()
  for (const tpl of given) {
    removeCount.set(tpl, (removeCount.get(tpl) ?? 0) + 1)
  }

  const result: T[] = []
  for (const item of staged) {
    const remaining = removeCount.get(item.template) ?? 0
    if (remaining > 0) {
      removeCount.set(item.template, remaining - 1)
    }
    else {
      result.push(item)
    }
  }
  return result
}
