import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8443',
      '/ws': { target: 'ws://127.0.0.1:8443', ws: true },
      '/bootstrap': 'http://127.0.0.1:8443',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
