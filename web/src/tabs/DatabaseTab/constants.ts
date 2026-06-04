import { createTheme } from '@uiw/codemirror-themes'
import { tags as hlTags } from '@lezer/highlight'

export const duneTheme = createTheme({
  theme: 'dark',
  settings: {
    background: 'var(--field-background)',
    foreground: 'var(--field-foreground)',
    caret: 'var(--accent)',
    selection: 'rgba(201,130,10,0.25)',
    selectionMatch: 'rgba(201,130,10,0.12)',
    lineHighlight: 'var(--surface)',
    gutterBackground: 'var(--surface)',
    gutterForeground: 'var(--muted)',
    gutterBorder: 'transparent',
    gutterActiveForeground: 'var(--accent)',
  },
  styles: [
    { tag: hlTags.comment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.lineComment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.blockComment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.keyword, color: 'var(--accent)', fontWeight: 'bold' },
    { tag: hlTags.definitionKeyword, color: 'var(--accent)' },
    { tag: hlTags.modifier, color: 'var(--accent)' },
    { tag: hlTags.operatorKeyword, color: 'var(--accent)' },
    { tag: hlTags.string, color: 'var(--success)' },
    { tag: hlTags.number, color: 'var(--warning)' },
    { tag: hlTags.bool, color: 'var(--warning)' },
    { tag: hlTags.null, color: 'var(--danger)' },
    { tag: hlTags.operator, color: 'var(--foreground)' },
    { tag: hlTags.punctuation, color: 'var(--muted)' },
    { tag: hlTags.name, color: 'var(--foreground)' },
    { tag: hlTags.typeName, color: 'var(--warning)' },
    { tag: hlTags.function(hlTags.variableName), color: 'var(--warning)' },
    { tag: hlTags.special(hlTags.name), color: 'var(--accent)' },
  ],
})

export type Section = 'tables' | 'describe' | 'sample' | 'search' | 'sql'
