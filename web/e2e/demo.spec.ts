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
  await expect(page.getByRole("link", { name: "Notes iOS" })).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Основная навигация" })
      .getByRole("link", { name: "Документация" }),
  ).toHaveCount(0);
  await expectNoA11yViolations(page);

  await page.getByRole("link", { name: "Notes iOS" }).click();
  await expect(page).toHaveURL(/\/apps\/[0-9a-f-]+/);
  await expect(page.getByRole("navigation", { name: "Разделы приложения" })).toBeVisible();

  await banner.getByRole("link", { name: "Выйти из демо" }).click();
  await expect(page).toHaveURL("/");

  await page.goto("/portfolio");
  await expect(page).toHaveURL(/\/login\?next=%2Fportfolio/);
});
