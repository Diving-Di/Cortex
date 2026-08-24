import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    // Bound jsdom workers to avoid CPU and memory contention on developer
    // machines while keeping the UI suites parallel.
    maxWorkers: 6,
    setupFiles: ['./src/test/setup.ts'],
  },
});
