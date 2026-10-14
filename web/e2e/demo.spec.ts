import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Демо-режим (решение владельца): статичный синтетический датасет без входа и без бэкенда.
test.use({ storageState: { cookies: [], origins: [] } });

test("visitor explores the demo without signing in and leaves it", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("main").getByRole("link", { name: "Посмотреть демо-данные" }).click();

  await expect(page).toHaveURL("/portfolio");
  const banner = page.getByRole("region", { name: "Демо-режим" });
  await expect(banner).toBeVisible();
  await expect(page.getByRole("link", { name: "Notes", exact: true })).toBeVisible();
  // Документация открыта гостям, поэтому ссылка есть и в демо.
  await expect(
    page
      .getByRole("navigation", { name: "Основная навигация" })
      .getByRole("link", { name: "Документация" }),
  ).toBeVisible();
  await expectNoA11yViolations(page);

  await page.getByRole("link", { name: "Notes", exact: true }).click();
  await expect(page).toHaveURL(/\/apps\/[0-9a-f-]+/);
  await expect(page.getByRole("navigation", { name: "Разделы приложения" })).toBeVisible();

  await banner.getByRole("link", { name: "Выйти из демо" }).click();
  await expect(page).toHaveURL("/");

  await page.goto("/portfolio");
  await expect(page).toHaveURL(/\/login\?next=%2Fportfolio/);
});

// Тёмная и светлая темы на реальных экранах дашборда: демо-датасет даёт данные без backend,
// axe проверяет контраст и роли (NFR-008) в обеих темах.
for (const theme of ["light", "dark"] as const) {
  test(`dashboard screens are accessible in the ${theme} theme`, async ({ page }) => {
    await page.addInitScript((t) => localStorage.setItem("am-theme", t), theme);
    await page.goto("/demo");
    await expect(page).toHaveURL("/portfolio");
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    await expect(page.getByRole("link", { name: "Notes", exact: true })).toBeVisible();
    await expectNoA11yViolations(page);

    const appId = "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f";
    for (const path of ["", "/retention", "/realtime", "/integration", "/settings"]) {
      await page.goto(`/apps/${appId}${path}`);
      await expect(page.getByRole("navigation", { name: "Разделы приложения" })).toBeVisible();
      await expect(page.getByRole("status").or(page.getByRole("main"))).not.toHaveCount(0);
      await page.waitForLoadState("networkidle");
      await expectNoA11yViolations(page);
    }

    await page.goto("/settings");
    await page.waitForLoadState("networkidle");
    await expectNoA11yViolations(page);
  });
}
