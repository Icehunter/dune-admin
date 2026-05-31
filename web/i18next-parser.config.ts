const config = {
  input: ['src/**/*.{ts,tsx}'],
  output: 'src/locales/$LOCALE/translation.json',
  locales: ['en-US', 'de', 'fr', 'es', 'pt-BR', 'ru', 'pl', 'tr', 'zh-CN', 'ja'],
  defaultNamespace: 'translation',
  defaultValue: (locale: string) => (locale === 'en-US' ? '' : 'MISSING'),
  keySeparator: '.',
  namespaceSeparator: false,
  sort: true,
}

export default config
