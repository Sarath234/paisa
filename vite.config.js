import { sveltekit } from "@sveltejs/kit/vite";
import { nodePolyfills } from "vite-plugin-node-polyfills";
import { resolve } from "path";

/** @type {import('vite').UserConfig} */
const config = {
  build: {
    target: 'es2021'
  },
  resolve: {
    alias: {
      "svelte-file-dropzone": resolve("./node_modules/svelte-file-dropzone/dist/index.js")
    }
  },
  optimizeDeps: {
    exclude: ["svelte-file-dropzone"]
  },
  plugins: [
    sveltekit(),
    nodePolyfills({
      globals: {
        Buffer: true
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
