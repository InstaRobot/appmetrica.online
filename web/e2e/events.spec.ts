import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// FR-102 пользовательские события. Данные — `make seed` в базу e2e: «Seed Notes» шлёт внутри сессий
// note_created {source}, search_used {results}, note_shared {target}, paywall_shown {place}, purchase {plan}
// за 35 дней по вчерашний день; у части людей есть user_id вида seed-NNNNN (профиль с событиями).
// Владелец уже вошёл: storageState из global-setup.

type AppList = { items: { id: string; name: string }[] };
type ProfileEvents = { events: unknown[] };

async function notesAppId(page: Page): Promise<string> {
  const res = await page.request.get("/api/v1/apps");
  expect(res.ok()).toBe(true);
  const app = ((await res.json()) as AppList).items.find((a) => a.name === "Seed Notes");
  if (!app) throw new Error("Нет приложения «Seed Notes»: выполните `make seed` для базы e2e");
  return app.id;
}

// Первый человек из seed с собственными событиями: у каждого user_id они появляются с первой же сессии,
// но не обязаны быть в окне ответа, поэтому перебираем несколько.
async function profileWithEvents(page: Page, appId: string): Promise<string> {
  for (let n = 1; n <= 60; n++) {
    const userId = `seed-${String(n).padStart(5, "0")}`;
    const res = await page.request.get(
      `/api/v1/apps/${appId}/profiles/${encodeURIComponent(`u:${userId}`)}/events?limit=1`,
    );
    if (res.ok() && ((await res.json()) as ProfileEvents).events.length > 0) return userId;
  }
  throw new Error("В «Seed Notes» нет человека с событиями: выполните `make seed` для базы e2e");
}

for (const theme of ["light", "dark"] as const) {
  test(`list, event page, breakdown and profile events in the ${theme} theme`, async ({ page }) => {
    await page.addInitScript((t) => localStorage.setItem("am-theme", t), theme);
    const appId = await notesAppId(page);

    // Вкладка «События» — в навигации приложения.
    await page.goto(`/apps/${appId}`);
    const nav = page.getByRole("navigation", { name: "Разделы приложения" });
    await nav.getByRole("link", { name: "События" }).click();
    await expect(page).toHaveURL(new RegExp(`/apps/${appId}/events`));
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);

    // Список: имена из seed, системные session_* не входят.
    const list = page.getByRole("table", { name: "Пользовательские события" });
    await expect(list).toBeVisible();
    for (const name of ["note_created", "search_used", "note_shared", "paywall_shown"]) {
      await expect(list.getByRole("link", { name, exact: true })).toBeVisible();
    }
    await expect(list.getByRole("link", { name: "session_start" })).toHaveCount(0);
    await expectNoA11yViolations(page);

    // Страница события: заголовок, итоги, разбивка по свойству по умолчанию.
    await list.getByRole("link", { name: "note_created", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/apps/${appId}/events/note_created`));
    await expect(page.getByRole("heading", { level: 1, name: "note_created" })).toBeVisible();
    const totals = page.getByRole("region", { name: "Итоги события" });
    await expect(totals.getByText(/\d/).first()).toBeVisible();

    // Разбивка: выбираем свойство явно — оно попадает в URL, таблица строится по нему.
    await page.getByRole("combobox", { name: "Свойство" }).click();
    await page.getByRole("option", { name: "source" }).click();
    await expect(page).toHaveURL(/[?&]property=source/);
    const breakdown = page.getByRole("table", { name: "note_created по свойству source" });
    await expect(breakdown).toBeVisible();
    for (const value of ["keyboard", "voice", "share"]) {
      await expect(breakdown.getByText(value, { exact: true })).toBeVisible();
    }
    await expectNoA11yViolations(page);

    // Профиль: раздел «События» с собственными событиями человека.
    const userId = await profileWithEvents(page, appId);
    await page.goto(`/apps/${appId}/profiles/${encodeURIComponent(`u:${userId}`)}`);
    await expect(page.getByRole("heading", { name: `u:${userId}` })).toBeVisible();
    await expect(page.getByRole("heading", { level: 3, name: "События" })).toBeVisible();
    const own = page.getByRole("table", { name: "Собственные события" });
    await expect(own).toBeVisible();
    await expect(
      own.getByText(/^(note_created|search_used|note_shared|paywall_shown|purchase)$/).first(),
    ).toBeVisible();
    await expectNoA11yViolations(page);
  });
}
