import { execFileSync } from "node:child_process";
import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-10 онбординг (T-041): новое приложение → curl-сниппет из буфера обмена → «Первое событие получено».
// web собран с NEXT_PUBLIC_INGEST_URL = адрес локального ingest (E2E_INGEST_URL, по умолчанию :8081),
// ingest пишет в ту же базу ClickHouse, что читает api. Владелец уже вошёл: storageState из global-setup.

const INGEST_URL = process.env.E2E_INGEST_URL ?? "http://localhost:8081";

async function createApp(page: Page, name: string): Promise<void> {
  await page.goto("/apps/new");
  await page.getByRole("textbox", { name: "Название" }).fill(name);
  const ios = page
    .getByRole("group", { name: "Платформы" })
    .getByRole("button", { name: "iOS", exact: true });
  await ios.click();
  await expect(ios).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "Создать" }).click();
  await expect(page).toHaveURL(/\/apps\/[0-9a-f-]{36}\/integration$/);
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
  const ios = page.getByRole("region", { name: "iOS" });
  const iosStatus = ios.getByRole("status", { name: "Статус iOS" });
  await expect(iosStatus).toContainText("Ждём первое событие с iOS");
  await expect(page.getByRole("link", { name: "Добавить платформу" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "эндпоинт" })).toHaveValue(
    `${INGEST_URL}/v1/events`,
  );

  await expectNoA11yViolations(page);

  await ios.getByRole("tab", { name: "curl" }).click();
  await ios.getByRole("button", { name: "Копировать код" }).click();
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
  // Статус секции — по breakdown платформ за 48 ч, опрос раз в 3 с.
  await expect(iosStatus).toContainText("Получено первое событие", { timeout: 10_000 });

  // После перезагрузки страница открывается в режиме здоровья.
  await page.reload();
  await expect(page.getByRole("group", { name: "Принято за 24 ч" })).toContainText("1");
  await expect(page.getByRole("button", { name: "Показать ключ и сниппеты" })).toBeVisible();
  await expect(page.getByText("Отказов за 24 часа нет ✓")).toBeVisible();
  await expectNoA11yViolations(page);
});
