import { defineConfig } from 'vitest/config';

// The API target must be supplied explicitly for a split dev server.
const target = process.env.KORE_EDGE_API_URL;
export default defineConfig({
  base: '/ui/',
  server: {
    host: '127.0.0.1',
    proxy: target ? Object.fromEntries(['/api', '/healthz', '/readyz', '/player', '/hls'].map(p => [p, { target }])) : undefined,
  },
  build: { outDir: '../internal/webui/dist', emptyOutDir: true, sourcemap: false },
  test: { environment: 'node', include: ['src/**/*.test.ts', 'src/**/*.test.tsx'] },
});
