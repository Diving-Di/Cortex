import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    // Bound jsdom workers to avoid CPU and memory contention on developer
    // machines while keeping the UI suites parallel.
    maxWorkers: 2,
    testTimeout: 15_000,
    hookTimeout: 15_000,
    setupFiles: ['./src/test/setup.ts'],
  },
});
