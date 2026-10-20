import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './e2e',
  timeout: 120000,
  expect: { timeout: 60000 },
  workers: 1,
  use: {
    baseURL: process.env.ADMIN_URL || 'http://127.0.0.1:8082',
    headless: true,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
})
