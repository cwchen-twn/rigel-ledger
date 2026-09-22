import { defineConfig } from 'vite'
import { resolve } from 'path'
import solidPlugin from 'vite-plugin-solid'
import tailwindcss from '@tailwindcss/vite'

// One IIFE (main.js) and one stylesheet (main.css) in web/static/dist, embedded
// into the Go binary and linked from web/templates/partials/base.tmpl.
export default defineConfig(({ mode }) => ({
  root: resolve(__dirname, 'src'),
  base: '/static/dist/',
  plugins: [solidPlugin(), tailwindcss()],
  resolve: {
    alias: { '~': resolve(__dirname, 'src') },
  },
  build: {
    outDir: resolve(__dirname, 'static/dist'),
    emptyOutDir: true,
    sourcemap: mode === 'development',
    minify: mode === 'production',
    chunkSizeWarningLimit: 900,
    // One stylesheet for the whole app, written as main.css (see assetFileNames).
    cssCodeSplit: false,
    rollupOptions: {
      input: resolve(__dirname, 'src/index.tsx'),
      output: {
        format: 'iife',
        name: 'RigelLedger',
        entryFileNames: 'main.js',
        chunkFileNames: '[name].js',
        assetFileNames: (info) => (info.names?.[0]?.endsWith('.css') ? 'main.css' : '[name][extname]'),
      },
    },
  },
}))
