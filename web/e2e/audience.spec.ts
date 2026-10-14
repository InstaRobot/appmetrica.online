import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-05 аудитория (T-038). Данные — `make seed` в базу e2e: приложение «Seed iOS» с 35 днями событий
// по вчерашний день (версии 1.2.0, 1.3.0, 1.4.0). Владелец уже вошёл: storageState из global-setup.

type AppList = { items: { id: string; name: string }[] };

async function seedAppId(page: Page): Promise<string> {
  const res = await page.request.get("/api/v1/apps");
  expect(res.ok()).toBe(true);
  const app = ((await res.json()) as AppList).items.find((a) => a.name === "Seed iOS");
  if (!app) throw new Error("Нет приложения «Seed iOS»: выполните `make seed` для базы e2e");
  return app.id;
}

const activeKpi = (page: Page) =>
  page.getByRole("region", { name: "Ключевые метрики" }).getByRole("button", { name: /^Активные/ });

// Значение KPI «Активные» после загрузки (до неё карточка — скелет без числа).
async function activeValue(page: Page): Promise<string> {
  const value = activeKpi(page).locator("[data-kpi-value]");
  await expect(value).toHaveText(/\d/);
  return (await value.textContent()) ?? "";
}

test("filters and period are reflected in URL", async ({ page }) => {
  const appId = await seedAppId(page);
  await page.goto(`/apps/${appId}`);
  await expect(page.getByRole("heading", { level: 1, name: "Seed iOS" })).toBeVisible();
  await expect(page.getByRole("radio", { name: "7 дней" })).toHaveAttribute("aria-checked", "true");
  const week = await activeValue(page);
  const chart = page.getByRole("img", { name: /^График активных пользователей по дням/ });
  await expect(chart).toBeVisible();
  const weekChart = await chart.getAttribute("aria-label");

  await page.getByRole("radio", { name: "30 дней" }).click();
  await expect(page).toHaveURL(/[?&]period=30d/);
  await expect(activeKpi(page).locator("[data-kpi-value]")).not.toHaveText(week);
  const month = await activeValue(page);
  await expect(chart).not.toHaveAttribute("aria-label", weekChart ?? "");

  await page
    .getByRole("table", { name: "Разбивка: Версия приложения" })
    .getByRole("button", { name: "Фильтр: Версия приложения 1.4.0" })
    .click();
  await expect(page).toHaveURL(/[?&]app_version=1\.4\.0/);
  await expect(page.getByText("Версия приложения: 1.4.0")).toBeVisible();
  await expect(activeKpi(page).locator("[data-kpi-value]")).not.toHaveText(month);

  await expectNoA11yViolations(page);

  await page.reload();
  await expect(page.getByText("Версия приложения: 1.4.0")).toBeVisible();
  await expect(page.getByRole("radio", { name: "30 дней" })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  await page.goBack();
  await expect(page).not.toHaveURL(/app_version=/);
  await expect(page).toHaveURL(/[?&]period=30d/);
  await expect(activeKpi(page).locator("[data-kpi-value]")).toHaveText(month);

  await page.goBack();
  await expect(page).not.toHaveURL(/period=/);
  await expect(page.getByRole("radio", { name: "7 дней" })).toHaveAttribute("aria-checked", "true");
  await expect(activeKpi(page).locator("[data-kpi-value]")).toHaveText(week);
});
