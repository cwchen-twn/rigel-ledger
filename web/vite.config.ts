import { defineConfig } from 'vite'
import { resolve } from 'path'
import solidPlugin from 'vite-plugin-solid'

export default defineConfig(({ mode }) => ({
  root: resolve(__dirname, 'src'),
  base: '/static/dist/',
  plugins: [solidPlugin()],
  build: {
    outDir: resolve(__dirname, 'static/dist'),
    emptyOutDir: true,
    sourcemap: mode === 'development',
    minify: mode === 'production',
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      input: resolve(__dirname, 'src/index.tsx'),
      output: {
        format: 'iife',
        name: 'RigelLedger',
        entryFileNames: 'main.js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name][extname]',
      },
    },
  },
}))
