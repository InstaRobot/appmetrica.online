import { randomUUID } from "node:crypto";
import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-08/S-09 профили и удаление данных (T-042). Человек создаётся событием в локальный ingest
// (E2E_INGEST_URL) для «Seed iOS» из `make seed`; user_id со слешем и пробелом проверяет кодирование
// person_key в URL. Полный цикл до «Удалено» выполняет worker стенда (job deletion, раз в минуту).

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

// sendSession — session_start и session_end новой установки пользователя userId «сейчас».
async function sendSession(page: Page, userId: string): Promise<void> {
  const start = new Date(Date.now() - 60_000).toISOString();
  const end = new Date().toISOString();
  const installId = randomUUID();
  const sessionId = randomUUID();
  const res = await page.request.post(`${INGEST_URL}/v1/events`, {
    headers: { "X-App-Key": WRITE_KEY },
    data: {
      sent_at: end,
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
          timestamp: start,
          install_id: installId,
          session_id: sessionId,
          user_id: userId,
        },
        {
          insert_id: randomUUID(),
          name: "session_end",
          timestamp: end,
          install_id: installId,
          session_id: sessionId,
          user_id: userId,
          duration_ms: 60_000,
        },
      ],
    },
  });
  expect(res.status(), `ingest ${INGEST_URL}: ${await res.text()}`).toBe(200);
}

async function searchProfile(page: Page, appId: string, query: string): Promise<void> {
  await page.goto(`/apps/${appId}/profiles`);
  const box = page.getByRole("searchbox", { name: "install_id или user_id" });
  await expect(box).toBeVisible();
  await expectNoA11yViolations(page);
  await box.fill(query);
  await box.press("Enter");
}

// openNewProfile создаёт человека, находит его и открывает карточку (один результат — сразу S-09).
async function openNewProfile(page: Page): Promise<{ appId: string; userId: string }> {
  const appId = await seedAppId(page);
  const userId = `e2e/${randomUUID()} user`;
  await sendSession(page, userId);
  await searchProfile(page, appId, userId);
  await expect(page).toHaveURL(`/apps/${appId}/profiles/${encodeURIComponent(`u:${userId}`)}`);
  await expect(page.getByRole("heading", { name: `u:${userId}` })).toBeVisible();
  await expect(page.getByRole("table", { name: "Сессии" }).getByRole("row")).toHaveCount(2);
  return { appId, userId };
}

async function requestDeletion(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Удалить данные" }).click();
  const dialog = page.getByRole("alertdialog", { name: "Удалить все данные этого человека?" });
  const confirm = dialog.getByRole("button", { name: "Удалить данные" });
  await expect(confirm).toBeDisabled();
  await dialog.getByRole("textbox", { name: "Введите УДАЛИТЬ" }).fill("УДАЛИТЬ");
  await confirm.click();
  await expect(dialog).toBeHidden();
}

test("requests deletion from the profile", async ({ page }) => {
  await openNewProfile(page);

  await expectNoA11yViolations(page);

  await requestDeletion(page);
  await expect(page.getByRole("group", { name: "Статус удаления" })).toContainText(
    /В очереди|Выполняется|Удалено/,
  );
  await expect(page.getByRole("button", { name: "Удалить данные" })).toBeDisabled();

  // После перезагрузки активный запрос подхватывается из журнала удалений.
  await page.reload();
  await expect(page.getByRole("group", { name: "Статус удаления" })).toContainText(
    /В очереди|Выполняется|Удалено/,
  );
});

test("deletes user data end to end", async ({ page }) => {
  test.setTimeout(180_000);
  const { appId, userId } = await openNewProfile(page);

  await requestDeletion(page);
  // Job deletion идёт раз в минуту, статус опрашивается раз в 10 с.
  await expect(page.getByRole("group", { name: "Статус удаления" })).toContainText("Удалено", {
    timeout: 150_000,
  });
  await expect(page).toHaveURL(`/apps/${appId}/profiles`, { timeout: 10_000 });

  await searchProfile(page, appId, userId);
  await expect(page.getByRole("heading", { name: "Ничего не найдено" })).toBeVisible();
});
