import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    // Run the Go backend on its default address (127.0.0.1:8080).
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
})
