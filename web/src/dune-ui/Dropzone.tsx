import { useState, type ReactNode } from 'react'
import { Spinner, toast } from '@heroui/react'
import { Icon } from './Icon'

type Props = {
  /** Comma-separated list of accepted file extensions, e.g. ".json" or ".backup,.zip". */
  accept: string
  /** Called with the chosen file (drag-drop or click-to-pick). */
  onSelect: (file: File) => void
  /** Show this file's name + size as a "selected" state inside the dropzone. */
  file?: File | null
  /** Override the default prompt text shown when nothing is selected. */
  prompt?: ReactNode
  /** Spinner overlay — drive from parent state when an upload is in flight. */
  uploading?: boolean
  /** Compact (less vertical padding). */
  compact?: boolean
  className?: string
}

/**
 * Drag-and-drop file picker. Click to open native file dialog; drop a file
 * to select it. Validates the extension against `accept` and toasts on
 * mismatch. Used by BlueprintsTab Import and BattlegroupTab Restore.
 */
export function Dropzone({
  accept, onSelect, file, prompt, uploading, compact, className = '',
}: Props) {
  const [dragging, setDragging] = useState(false)

  const validateAndSelect = (f: File | undefined | null) => {
    if (!f) return
    const exts = accept.split(',').map((x) => x.trim().toLowerCase()).filter(Boolean)
    if (exts.length > 0) {
      const ok = exts.some((ext) => f.name.toLowerCase().endsWith(ext))
      if (!ok) {
        toast.warning(`Drop a ${accept} file`)
        return
      }
    }
    onSelect(f)
  }

  const openPicker = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = accept
    input.onchange = () => validateAndSelect(input.files?.[0])
    input.click()
  }

  return (
    <div
      className={
        'rounded-[var(--radius)] flex flex-col items-center justify-center gap-2 text-sm cursor-pointer transition-all border-2 border-dashed '
        + (compact ? 'py-2 px-3' : 'py-6 px-4') + ' '
        + (dragging
          ? 'border-warning bg-warning/10 text-warning'
          : 'border-border bg-background text-muted hover:border-warning/60 hover:text-warning')
        + ' ' + className
      }
      onDragOver={(e) => {
        e.preventDefault()
        setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault()
        setDragging(false)
        validateAndSelect(e.dataTransfer.files[0])
      }}
      onClick={openPicker}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          openPicker()
        }
      }}
    >
      {uploading
        ? (
            <span className="flex items-center gap-2">
              <Spinner size="sm" color="current" />
              {' '}
              Uploading…
            </span>
          )
        : file
          ? (
              <span className="flex flex-col items-center gap-0.5">
                <span className="flex items-center gap-1.5 text-foreground">
                  <Icon name="file-check" />
                  {' '}
                  <span className="font-mono">{file.name}</span>
                </span>
                <span className="text-xs text-muted">
                  {(file.size / 1024).toFixed(1)}
                  {' '}
                  KB · click to replace
                </span>
              </span>
            )
          : (
              <span className="flex items-center gap-1.5">
                <Icon name="upload" />
                {' '}
                {prompt ?? `Drop or click to upload a ${accept} file`}
              </span>
            )}
    </div>
  )
}
