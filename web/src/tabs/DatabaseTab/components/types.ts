export type TableData = { headers: string[], rows: string[][] }

export interface TableSearchInputProps {
  value: string
  onChange: (v: string) => void
  onRun: () => void
  tableNames: string[]
  ariaLabel: string
  placeholder: string
}
