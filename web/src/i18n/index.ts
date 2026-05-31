import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

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

export const LOCALE_KEY = 'dune_admin_locale'
export const DEFAULT_LOCALE = 'en-US'

export const LANGUAGES = [
  { code: 'en-US', label: 'English', flag: '🇨🇦' },
  { code: 'de', label: 'Deutsch', flag: '🇩🇪' },
  { code: 'fr', label: 'Français', flag: '🇫🇷' },
  { code: 'es', label: 'Español', flag: '🇪🇸' },
  { code: 'pt-BR', label: 'Português (BR)', flag: '🇧🇷' },
  { code: 'ru', label: 'Русский', flag: '🇷🇺' },
  { code: 'pl', label: 'Polski', flag: '🇵🇱' },
  { code: 'tr', label: 'Türkçe', flag: '🇹🇷' },
  { code: 'zh-CN', label: '中文 (简体)', flag: '🇨🇳' },
  { code: 'ja', label: '日本語', flag: '🇯🇵' },
] as const

const saved = localStorage.getItem(LOCALE_KEY) ?? DEFAULT_LOCALE

i18n
  .use(initReactI18next)
  .init({
    resources: {
      'en-US': { translation: enUS },
      'de': { translation: de },
      'fr': { translation: fr },
      'es': { translation: es },
      'pt-BR': { translation: ptBR },
      'ru': { translation: ru },
      'pl': { translation: pl },
      'tr': { translation: tr },
      'zh-CN': { translation: zhCN },
      'ja': { translation: ja },
    },
    lng: saved,
    fallbackLng: DEFAULT_LOCALE,
    interpolation: { escapeValue: false },
  })

export function setLocale(code: string): void {
  localStorage.setItem(LOCALE_KEY, code)
  void i18n.changeLanguage(code)
}

export default i18n
