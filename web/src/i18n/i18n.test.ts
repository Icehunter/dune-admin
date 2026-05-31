import { describe, it, expect } from 'vitest'
import enUS from '../locales/en-US/translation.json'
import de from '../locales/de/translation.json'
import fr from '../locales/fr/translation.json'
import es from '../locales/es/translation.json'
import ptBR from '../locales/pt-BR/translation.json'
import ru from '../locales/ru/translation.json'
import pl from '../locales/pl/translation.json'
import tr from '../locales/tr/translation.json'
import zhCN from '../locales/zh-CN/translation.json'
import ja from '../locales/ja/translation.json'

const LOCALES: Record<string, Record<string, unknown>> = {
  de,
  fr,
  es,
  'pt-BR': ptBR,
  ru,
  pl,
  tr,
  'zh-CN': zhCN,
  ja,
}

function flatKeys(obj: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) => {
    const key = prefix ? `${prefix}.${k}` : k
    return typeof v === 'object' && v !== null
      ? flatKeys(v as Record<string, unknown>, key)
      : [key]
  })
}

describe('i18n completeness', () => {
  const enKeys = flatKeys(enUS as Record<string, unknown>)

  it('en-US has no empty values', () => {
    enKeys.forEach((key) => {
      const parts = key.split('.')
      let val: unknown = enUS
      for (const p of parts) val = (val as Record<string, unknown>)[p]
      expect(typeof val === 'string' && val.length > 0, `en-US key "${key}" is empty`).toBe(true)
    })
  })

  Object.entries(LOCALES).forEach(([locale, translations]) => {
    it(`${locale} has all en-US keys`, () => {
      const localeKeys = new Set(flatKeys(translations))
      enKeys.forEach((key) => {
        expect(localeKeys.has(key), `${locale} missing key "${key}"`).toBe(true)
      })
    })
  })
})
