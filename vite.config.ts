import { sveltekit } from "@sveltejs/kit/vite";
import { nodePolyfills } from "vite-plugin-node-polyfills";
import { VitePWA } from "vite-plugin-pwa";

/** @type {import('vite').UserConfig} */
const config = {
  build: {
    target: 'es2021'
  },
  plugins: [
    sveltekit(),
    nodePolyfills({
      globals: {
        Buffer: true
      }
    }),
    VitePWA({
      registerType: 'autoUpdate',
      injectRegister: 'auto',
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,ico,png,svg}']
      },
      includeAssets: ['pwa-assets/icon-192.png', 'pwa-assets/icon-512.png', 'pwa-assets/maskable-icon-512x512.png'],
      manifest: {
        name: 'Paisa',
        short_name: 'Paisa',
        description: 'Personal finance manager — track spending, investments, and net worth.',
        theme_color: '#b388ff',
        background_color: '#ffffff',
        display: 'standalone',
        start_url: '/',
        scope: '/',
        icons: [
          {
            src: '/pwa-assets/icon-192.png',
            sizes: '192x192',
            type: 'image/png',
            purpose: 'any'
          },
          {
            src: '/pwa-assets/icon-512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'any'
          },
          {
            src: '/pwa-assets/maskable-icon-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable'
          }
        ]
      }
    })
  ],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:7500"
      }
    },
    fs: {
      allow: ["./fonts"]
    }
  }
};

export default config;
