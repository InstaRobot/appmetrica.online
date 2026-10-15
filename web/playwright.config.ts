import { defineConfig, devices } from "@playwright/test";

// E2E против production-сборки web (`pnpm build`), локальных ingest (:8081) и api (:8080) с APP_ENV=e2e.
// Аутентификация: e2e/global-setup.ts регистрирует passkey через виртуальный аутентификатор CDP
// по ссылке из служебного POST /internal/e2e/passkey-link и сохраняет storageState в e2e/.auth/owner.json.
// E2E_PORT — порт web, если 3000 занят (тогда PUBLIC_BASE_URL и WEBAUTHN_RP_ORIGINS api — тот же origin),
// E2E_API_URL — адрес api для служебных маршрутов (e2e/helpers/seed.ts).
const port = Number(process.env.E2E_PORT ?? 3000);
const baseURL = `http://localhost:${port}`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : [["list"]],
  globalSetup: "./e2e/global-setup.ts",
  use: {
    baseURL,
    storageState: "e2e/.auth/owner.json",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    locale: "ru-RU",
    timezoneId: "Europe/Moscow",
  },
  projects: [
    {
      name: "chromium-desktop",
      testIgnore: /mobile\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
    // Телефон: Chromium с мобильной эмуляцией (в CI установлен только Chromium), ширина iPhone 13.
    {
      name: "chromium-mobile",
      testMatch: /mobile\.spec\.ts/,
      use: { ...devices["Pixel 7"], viewport: { width: 390, height: 844 } },
    },
  ],
  webServer: {
    command: `pnpm exec next start --port ${port}`,
    url: `${baseURL}/api/health`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
