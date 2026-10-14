import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-00 Лендинг: публичная страница для анонимного посетителя (без сессии владельца).
test.use({ storageState: { cookies: [], origins: [] } });

test("anonymous visitor sees the landing and can go to sign-in", async ({ page }) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "Аналитика ваших приложений — по одному HTTP-запросу",
    }),
  ).toBeVisible();
  await expectNoA11yViolations(page);

  await page.getByRole("main").getByRole("link", { name: "Войти в панель" }).click();
  await expect(page).toHaveURL("/login");
});

test("theme choice survives a reload", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("banner").getByRole("button", { name: "Включить тёмную тему" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.reload();

  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expectNoA11yViolations(page);
});
