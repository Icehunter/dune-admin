import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/api/v1/logs/stream': { target: 'ws://localhost:8080', ws: true, changeOrigin: true },
    },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: 'react-vendor',
              test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/,
              priority: 50,
            },
            {
              name: 'router-vendor',
              test: /node_modules[\\/]react-router-dom[\\/]/,
              priority: 45,
            },
            {
              name: 'auth-vendor',
              test: /node_modules[\\/]@clerk[\\/]/,
              priority: 40,
            },
            {
              name: 'ui-vendor',
              test: /node_modules[\\/]@heroui[\\/]/,
              priority: 35,
            },
            {
              name: 'codemirror-vendor',
              test: /node_modules[\\/](@codemirror|@uiw|@lezer)[\\/]/,
              priority: 34,
            },
            {
              name: 'charts-vendor',
              test: /node_modules[\\/](recharts|d3-.+|d3)[\\/]/,
              priority: 33,
            },
            {
              name: 'maps-vendor',
              test: /node_modules[\\/](leaflet|react-leaflet)[\\/]/,
              priority: 32,
            },
            {
              name: 'icons-vendor',
              test: /node_modules[\\/](@iconify|@iconify-json)[\\/]/,
              priority: 31,
            },
            {
              name: 'i18n-vendor',
              test: /node_modules[\\/](i18next|react-i18next)[\\/]/,
              priority: 30,
            },
            {
              name: 'vendor',
              test: /node_modules[\\/]/,
              priority: 10,
            },
            {
              name: 'common',
              minShareCount: 2,
              minSize: 10000,
              priority: 5,
            },
          ],
        },
      },
    },
  },
})
