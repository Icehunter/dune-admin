import { useMemo, useState } from 'react'

export type SortDir = 'asc' | 'desc'

export function useTableSort<T, K extends string>(
  rows: T[],
  initialKey: K,
  getValue: (row: T, key: K) => string | number | null | undefined,
  initialDir: SortDir = 'asc',
) {
  const [sortKey, setSortKey] = useState<K>(initialKey)
  const [sortDir, setSortDir] = useState<SortDir>(initialDir)

  const sorted = useMemo(() => {
    const out = [...rows]
    out.sort((a, b) => {
      const av = getValue(a, sortKey)
      const bv = getValue(b, sortKey)
      let cmp = 0
      if (typeof av === 'number' && typeof bv === 'number') cmp = av - bv
      else cmp = String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true })
      return sortDir === 'asc' ? cmp : -cmp
    })
    return out
  }, [rows, sortKey, sortDir, getValue])

  const toggle = (key: K) => {
    if (key === sortKey) setSortDir(d => (d === 'asc' ? 'desc' : 'asc'))
    else { setSortKey(key); setSortDir('asc') }
  }

  return { sorted, sortKey, sortDir, toggle }
}
