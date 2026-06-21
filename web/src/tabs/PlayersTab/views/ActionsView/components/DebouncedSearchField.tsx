import * as React from 'react'
import { SearchField } from '@heroui/react'
import { useDebounce } from '../hooks/useDebounce'
import type { DebouncedSearchFieldProps } from './interfaces'

export const DebouncedSearchField: React.FC<DebouncedSearchFieldProps> = ({
  onSearch,
  placeholder,
  className,
}) => {
  const [value, setValue] = React.useState('')
  const debounced = useDebounce(value)
  React.useEffect(() => {
    onSearch(debounced)
  }, [debounced, onSearch])
  return (
    <SearchField aria-label="Search" {...(className !== undefined ? { className } : {})} value={value} onChange={setValue}>
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input {...(placeholder !== undefined ? { placeholder } : {})} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  )
}
