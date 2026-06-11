import type { RawSection, ServerSetting } from '../../../api/client'

export interface CategorySectionProps {
  title: string
  description: string
  categories: [string, ServerSetting[]][]
  expandedCategory: string | null
  onToggle: (cat: string) => void
  searching: boolean
  pending: Map<string, string>
  onChange: (item: ServerSetting, value: string) => void
  onDelete: (item: ServerSetting) => Promise<void>
  isAmpManaged: (item: ServerSetting) => boolean
}

export interface CategoryButtonProps {
  cat: string
  catItems: ServerSetting[]
  isOpen: boolean
  fullWidth: boolean
  onToggle: (cat: string) => void
}

export interface CategoryPanelProps {
  cat: string
  catItems: ServerSetting[]
  searching: boolean
  pending: Map<string, string>
  onChange: (item: ServerSetting, value: string) => void
  onDelete: (item: ServerSetting) => Promise<void>
  onToggle: (cat: string) => void
  isAmpManaged: (item: ServerSetting) => boolean
}

export interface RawSectionPanelProps {
  sections: RawSection[]
  onSaved: () => void
}

export interface SettingRowProps {
  item: ServerSetting
  pending: string | undefined
  onChange: (value: string) => void
  onDelete: () => Promise<void>
  // True when the active control plane is AMP and this is a curated, AMP-managed
  // setting (written through the AMP API rather than the INI files).
  ampManaged?: boolean
}
