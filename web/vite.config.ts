import { defineConfig } from 'vite'
import { resolve } from 'path'

export default defineConfig(({ mode }) => ({
  root: resolve(__dirname, 'src'),
  base: '/static/dist/',
  build: {
    outDir: resolve(__dirname, 'static/dist'),
    emptyOutDir: true,
    sourcemap: mode === 'development',
    minify: mode === 'production',
    rollupOptions: {
      input: resolve(__dirname, 'src/main.ts'),
      output: {
        entryFileNames: 'main.js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name][extname]',
      },
    },
  },
}))
