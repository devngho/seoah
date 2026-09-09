import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'

export default defineConfig({
  plugins: [solid()],
  server: {
    allowedHosts: ['devngho-fedora-nb1.tail372bfc.ts.net'],
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
      },
    },
  }
})
