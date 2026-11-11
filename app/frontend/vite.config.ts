import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// iOS shows a startup image only when its pixel size exactly matches the screen, so each entry needs a pair in public/splash/.
// Portrait CSS width, height and pixel ratio.
const appleScreens = [
  [440, 956, 3], [420, 912, 3], [402, 874, 3], [430, 932, 3], [393, 852, 3], [428, 926, 3],
  [390, 844, 3], [375, 812, 3], [360, 780, 3], [414, 896, 3], [414, 896, 2], [414, 736, 3],
  [375, 667, 2], [320, 568, 2], [1032, 1376, 2], [1024, 1366, 2], [834, 1210, 2], [834, 1194, 2],
  [820, 1180, 2], [834, 1112, 2], [810, 1080, 2], [744, 1133, 2], [768, 1024, 2],
]

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '..', '')
  const apiPort = env.PORT || '8000'

  return {
    plugins: [
      react(),
      tailwindcss(),
      {
        // Without a preload the browser finds Geist only after parsing the CSS, so text paints in the fallback and reflows.
        name: 'preload-geist',
        transformIndexHtml: {
          order: 'post',
          handler(_html, ctx) {
            const font = Object.keys(ctx.bundle ?? {}).find((file) => /geist-latin-wght-normal-.*\.woff2$/.test(file))
            return font
              ? [{ tag: 'link', attrs: { rel: 'preload', as: 'font', type: 'font/woff2', href: `/${font}`, crossorigin: '' }, injectTo: 'head' }]
              : []
          },
        },
      },
      {
        name: 'apple-startup-images',
        transformIndexHtml: () =>
          appleScreens.flatMap(([width, height, ratio]) =>
            (['portrait', 'landscape'] as const).map((orientation) => {
              const [w, h] = orientation === 'portrait' ? [width * ratio, height * ratio] : [height * ratio, width * ratio]
              return {
                tag: 'link',
                attrs: {
                  rel: 'apple-touch-startup-image',
                  href: `/splash/${w}x${h}.png`,
                  // iOS keeps the portrait device-width and device-height in landscape.
                  media: `(device-width: ${width}px) and (device-height: ${height}px) and (-webkit-device-pixel-ratio: ${ratio}) and (orientation: ${orientation})`,
                },
                injectTo: 'head' as const,
              }
            }),
          ),
      },
      VitePWA({
        registerType: 'autoUpdate',
        includeAssets: ['favicon.ico', 'favicon.svg', 'apple-touch-icon.png'],
        manifest: {
          name: 'Morgenblau',
          short_name: 'Morgenblau',
          description: 'A personal daily newspaper',
          // Installed Android apps also paint the strip behind the gesture bar with these, in both color schemes.
          theme_color: '#f5f5f4',
          background_color: '#f5f5f4',
          display: 'standalone',
          start_url: '/',
          scope: '/',
          icons: [
            { src: '/pwa-192x192.png', sizes: '192x192', type: 'image/png' },
            { src: '/pwa-512x512.png', sizes: '512x512', type: 'image/png' },
            { src: '/pwa-maskable-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
          ],
        },
        workbox: {
          // Document requests must reach the server for session redirects.
          globIgnores: ['**/index.html'],
          navigateFallback: null,
        },
      }),
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      host: '127.0.0.1',
      allowedHosts: process.env.AMP_ORB ? true : undefined,
      proxy: {
        '/api': `http://localhost:${apiPort}`,
      },
    },
    build: {
      // Route split alone still left one >500kB chunk; splitting vendor code out clears the warning.
      rolldownOptions: {
        output: {
          codeSplitting: {
            groups: [{ name: 'vendor', test: /node_modules/ }],
          },
        },
      },
    },
  }
})
