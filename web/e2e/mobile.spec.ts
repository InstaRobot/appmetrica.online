import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Мобильная адаптация (проект chromium-mobile, 390 px): документ не шире окна — прокручиваются только
// таблицы внутри карточек, навигация — через бургер. Панель — в демо-режиме: данные статичны.
const APP = "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f";

async function expectNoHorizontalScroll(page: Page) {
  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  expect(scrollWidth, `${page.url()}: документ шире окна`).toBeLessThanOrEqual(innerWidth);
}

test.use({ storageState: { cookies: [], origins: [] } });

test("the landing fits a phone and the menu leads to the docs", async ({ page }) => {
  await page.goto("/ru");
  await expectNoHorizontalScroll(page);
  await expectNoA11yViolations(page);

  await page.getByRole("button", { name: "Открыть меню" }).click();
  const panel = page.getByRole("dialog", { name: "Навигация" });
  await panel.getByRole("link", { name: "Документация" }).click();
  await expect(page).toHaveURL(/\/ru\/docs$/);
  await expectNoHorizontalScroll(page);
});

test("sign-in and a docs page fit a phone", async ({ page }) => {
  for (const path of ["/login", "/ru/docs/recipes/curl", "/ru/docs/concepts/initialization"]) {
    await page.goto(path);
    await expectNoHorizontalScroll(page);
  }
  await expectNoA11yViolations(page);
});

test.describe("dashboard on a phone", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/demo");
    await expect(page).toHaveURL("/portfolio");
  });

  test("the portfolio fits and the menu switches sections", async ({ page }) => {
    await expectNoHorizontalScroll(page);
    await expectNoA11yViolations(page);

    await page.getByRole("button", { name: "Открыть меню" }).click();
    await page
      .getByRole("dialog", { name: "Навигация" })
      .getByRole("link", { name: "Настройки" })
      .click();
    await expect(page).toHaveURL(/\/settings/);
    await expectNoHorizontalScroll(page);
  });

  test("every app section fits the screen", async ({ page }) => {
    for (const tab of ["", "/retention", "/realtime", "/profiles", "/integration", "/settings"]) {
      await page.goto(`/apps/${APP}${tab}`);
      await expect(page.getByRole("navigation", { name: "Разделы приложения" })).toBeVisible();
      await expectNoHorizontalScroll(page);
    }
    await expectNoA11yViolations(page);
  });
});
