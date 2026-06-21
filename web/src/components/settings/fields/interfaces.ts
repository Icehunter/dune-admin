export interface SearchableSelectOption {
  id: string
  label: string
}

export interface SearchableSelectProps {
  value: string
  onChange: (id: string) => void
  options: SearchableSelectOption[]
  placeholder?: string
  isDisabled?: boolean
  ariaLabel?: string
}
