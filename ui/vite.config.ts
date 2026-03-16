import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/s.js': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/s/': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/i': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/c.js': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/consent': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/tm': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
      '/health': {
        target: 'http://localhost:3456',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
})
