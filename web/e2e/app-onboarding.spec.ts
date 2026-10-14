import { execFileSync } from "node:child_process";
import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

// S-10 онбординг (T-041): новое приложение → curl-сниппет из буфера обмена → «Первое событие получено».
// web собран с NEXT_PUBLIC_INGEST_URL = адрес локального ingest (E2E_INGEST_URL, по умолчанию :8081),
// ingest пишет в ту же базу ClickHouse, что читает api. Владелец уже вошёл: storageState из global-setup.

const INGEST_URL = process.env.E2E_INGEST_URL ?? "http://localhost:8081";

async function createApp(page: Page, name: string): Promise<void> {
  await page.goto("/apps/new");
  await page.getByRole("textbox", { name: "Название" }).fill(name);
  await page.getByRole("combobox", { name: "Платформа" }).click();
  await page.getByRole("option", { name: "iOS" }).click();
  await page.getByRole("button", { name: "Создать" }).click();
  await expect(page).toHaveURL(/\/apps\/[0-9a-f-]{36}\/integration$/);
}

async function expectNoA11yViolations(page: Page): Promise<void> {
  const { violations } = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(violations).toEqual([]);
}

// runSnippet выполняет curl-сниппет в bash и возвращает ответ ingest.
function runSnippet(snippet: string): string {
  return execFileSync("bash", ["-c", snippet], { encoding: "utf8", timeout: 15_000 });
}

test("first event turns onboarding green", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const name = `Onboarding ${Date.now().toString(36)}`;
  await createApp(page, name);

  const status = page.getByRole("status", { name: "Первое событие" });
  await expect(status).toContainText("Ждём первое событие…");
  await expect(page.getByRole("textbox", { name: "эндпоинт" })).toHaveValue(
    `${INGEST_URL}/v1/events`,
  );

  await expectNoA11yViolations(page);

  await page.getByRole("tab", { name: "curl" }).click();
  await page.getByRole("button", { name: "Копировать код" }).click();
  const snippet = await page.evaluate(() => navigator.clipboard.readText());
  expect(snippet).toContain(`curl -sS -X POST "${INGEST_URL}/v1/events"`);

  // Новый write-ключ попадает в кэш ingest при очередной перезагрузке (KEY_CACHE_REFRESH, 15 с):
  // до этого ingest отвечает 401, поэтому сниппет повторяется до первого принятого события.
  let response = "";
  await expect
    .poll(
      () => {
        response = runSnippet(snippet);
        return response;
      },
      { timeout: 30_000, intervals: [1_000] },
    )
    .toContain('"accepted":1');

  const accepted = Date.now();
  await expect(status).toContainText(`Первое событие получено`, { timeout: 10_000 });
  expect(Date.now() - accepted).toBeLessThanOrEqual(10_000);
  await expect(status).toContainText(`${name} подключено!`);
  await expect(page.getByRole("link", { name: "Открыть Аудиторию" })).toBeVisible();

  // После перезагрузки страница открывается в режиме здоровья.
  await page.reload();
  await expect(page.getByRole("group", { name: "Принято за 24 ч" })).toContainText("1");
  await expect(page.getByRole("button", { name: "Показать ключ и сниппеты" })).toBeVisible();
  await expect(page.getByText("Отказов за 24 часа нет ✓")).toBeVisible();
  await expectNoA11yViolations(page);
});
