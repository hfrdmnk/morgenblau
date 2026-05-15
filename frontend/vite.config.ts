import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '..', '')
  const apiPort = env.PORT || '8000'

  return {
    plugins: [react(), tailwindcss()],
    server: {
      allowedHosts: ['local.morgen.blue'],
      proxy: {
        '/api': `http://localhost:${apiPort}`,
      },
    },
  }
})
