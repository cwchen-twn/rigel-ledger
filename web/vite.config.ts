import { defineConfig } from 'vite'
import { resolve } from 'path'
import solidPlugin from 'vite-plugin-solid'
import tailwindcss from '@tailwindcss/vite'

// One IIFE and one stylesheet in web/static/dist, embedded into the Go binary.
// Every file name carries a content hash (main-3f9a1c2b.js), and manifest.json
// maps the entry to them; internal/response/assets.go reads it to link the
// bundle from web/templates/partials/base.tmpl. A changed file is a new URL, so
// /static/dist is served as immutable and a stale bundle cannot stick.
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
    // One stylesheet for the whole app.
    cssCodeSplit: false,
    // Not the default .vite/manifest.json: go:embed skips dot directories.
    manifest: 'manifest.json',
    rollupOptions: {
      input: resolve(__dirname, 'src/index.tsx'),
      output: {
        format: 'iife',
        name: 'RigelLedger',
        entryFileNames: 'main-[hash].js',
        chunkFileNames: '[name]-[hash].js',
        assetFileNames: (info) => (info.names?.[0]?.endsWith('.css') ? 'main-[hash][extname]' : '[name]-[hash][extname]'),
      },
    },
  },
}))
