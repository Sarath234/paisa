import { defineConfig, minimalPreset } from '@vite-pwa/assets-generator/config'

export default defineConfig({
  preset: {
    transparent: minimalPreset.transparent,
    maskable: minimalPreset.maskable,
    apple: { sizes: [] },
  },
  images: ['static/pwa-assets/paisa-icon.svg'],
  outDir: 'static/pwa-assets'
})
