import { defineConfig, devices } from '@playwright/test'

/**
 * E2E_BASE_URL — where the SPA is served (e.g. Vite preview or docker web-e2e).
 * E2E_SKIP_WEB_SERVER — set to 1/true to not start `vite preview` (compose already serves the app).
 * E2E_API_BASE_URL — passed into `vite build` when webServer builds the app (must reach API from the browser).
 * E2E_IGNORE_HTTPS_ERRORS — set when the API uses a self-signed cert (e.g. local HTTPS).
 */
const baseURL = process.env.E2E_BASE_URL || 'http://127.0.0.1:4173'
const skipWeb =
  process.env.E2E_SKIP_WEB_SERVER === '1' || process.env.E2E_SKIP_WEB_SERVER === 'true'
const apiBase = process.env.E2E_API_BASE_URL || 'http://127.0.0.1:8080'
const ignoreHTTPS =
  process.env.E2E_IGNORE_HTTPS_ERRORS === '1' ||
  process.env.E2E_IGNORE_HTTPS_ERRORS === 'true'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  use: {
    baseURL,
    trace: 'on-first-retry',
    ignoreHTTPSErrors: ignoreHTTPS,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: skipWeb
    ? undefined
    : {
        command: `npm run build && npm run preview -- --host 127.0.0.1 --port 4173 --strictPort`,
        url: baseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
        env: {
          ...process.env,
          VITE_API_BASE_URL: apiBase,
        },
      },
})
