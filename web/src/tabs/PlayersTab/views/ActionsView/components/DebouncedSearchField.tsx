import { useEffect, useState } from 'react'
import { SearchField } from '@heroui/react'
import { useDebounce } from '../hooks/useDebounce'

interface DebouncedSearchFieldProps {
  onSearch: (q: string) => void
  placeholder?: string
  className?: string
}

export function DebouncedSearchField({
  onSearch,
  placeholder,
  className,
}: DebouncedSearchFieldProps) {
  const [value, setValue] = useState('')
  const debounced = useDebounce(value)
  useEffect(() => {
    onSearch(debounced)
  }, [debounced, onSearch])
  return (
    <SearchField aria-label="Search" className={className} value={value} onChange={setValue}>
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input placeholder={placeholder} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  )
}
