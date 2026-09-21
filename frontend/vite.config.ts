import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'https://localhost:8080',
        changeOrigin: true,
        secure: false,
        timeout: 600000,       // 10 minutes — matches backend WriteTimeout
        proxyTimeout: 600000,  // 10 minutes for Telegram chunk fetches
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
