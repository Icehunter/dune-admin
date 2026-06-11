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
