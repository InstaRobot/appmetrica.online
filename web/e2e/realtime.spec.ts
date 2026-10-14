import { randomUUID } from "node:crypto";
import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-07 realtime (T-040). Приложение «Seed iOS» из `make seed`, его write-ключ — DEV_WRITE_KEY.
// Событие отправляется в локальный ingest (E2E_INGEST_URL, по умолчанию :8081), который пишет
// в ту же базу ClickHouse, что читает api. Владелец уже вошёл: storageState из global-setup.

const INGEST_URL = process.env.E2E_INGEST_URL ?? "http://localhost:8081";
const WRITE_KEY = process.env.E2E_WRITE_KEY ?? "pk_devseedkey0000000000000000000000";

type AppList = { items: { id: string; name: string }[] };

async function seedAppId(page: Page): Promise<string> {
  const res = await page.request.get("/api/v1/apps");
  expect(res.ok()).toBe(true);
  const app = ((await res.json()) as AppList).items.find((a) => a.name === "Seed iOS");
  if (!app) throw new Error("Нет приложения «Seed iOS»: выполните `make seed` для базы e2e");
  return app.id;
}

// sendSessionStart — батч из одного session_start новой установки «сейчас».
async function sendSessionStart(page: Page, installId: string): Promise<void> {
  const now = new Date().toISOString();
  const res = await page.request.post(`${INGEST_URL}/v1/events`, {
    headers: { "X-App-Key": WRITE_KEY },
    data: {
      sent_at: now,
      context: {
        platform: "ios",
        os_name: "iOS",
        os_version: "26.1",
        app_version: "9.9.9",
        device_model: "iPhone17,1",
        device_type: "phone",
        locale: "ru-RU",
        environment: "production",
      },
      events: [
        {
          insert_id: randomUUID(),
          name: "session_start",
          timestamp: now,
          install_id: installId,
          session_id: randomUUID(),
        },
      ],
    },
  });
  expect(res.status(), `ingest ${INGEST_URL}: ${await res.text()}`).toBe(200);
}

test("shows new event within 15 seconds", async ({ page }) => {
  const appId = await seedAppId(page);
  await page.goto(`/apps/${appId}/realtime`);
  await expect(page.getByRole("heading", { level: 1, name: "Seed iOS" })).toBeVisible();

  const counter = page.getByRole("region", { name: "Активны сейчас" }).locator("[data-value]");
  await expect(counter).toBeVisible();
  const before = Number(await counter.getAttribute("data-value"));

  await expectNoA11yViolations(page);

  const installId = randomUUID();
  await sendSessionStart(page, installId);

  const feed = page.getByRole("table", { name: "Последние события" });
  await expect(feed.getByRole("link", { name: `Установка ${installId}` })).toBeVisible({
    timeout: 15_000,
  });
  await expect
    .poll(async () => Number(await counter.getAttribute("data-value")), { timeout: 5_000 })
    .toBeGreaterThan(before);

  // На паузе опрос стоит: новое событие в ленте не появляется.
  await page.getByRole("button", { name: "Пауза" }).click();
  await expect(page.getByRole("button", { name: "Продолжить" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  const pausedInstall = randomUUID();
  await sendSessionStart(page, pausedInstall);
  await page.waitForTimeout(7_000);
  await expect(feed.getByRole("link", { name: `Установка ${pausedInstall}` })).toHaveCount(0);

  await page.getByRole("button", { name: "Продолжить" }).click();
  await expect(feed.getByRole("link", { name: `Установка ${pausedInstall}` })).toBeVisible({
    timeout: 10_000,
  });
});
