import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-14a: глобальная 404 для вошедшего владельца (T-044 — axe на всех экранах S-01…S-14).
test("not found page is accessible", async ({ page }) => {
  const res = await page.goto("/no-such-page");

  expect(res?.status()).toBe(404);
  await expect(page.getByRole("heading", { level: 1, name: "Страница не найдена" })).toBeVisible();
  await expect(page.getByRole("link", { name: "В портфель" })).toHaveAttribute(
    "href",
    "/portfolio",
  );
  await expectNoA11yViolations(page);
});
