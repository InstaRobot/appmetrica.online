import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Каркас дашборда (T-036) и документация (T-047, T-048). Владелец уже вошёл: storageState из global-setup.

test("footer shows DB-IP attribution", async ({ page }) => {
  await page.goto("/");

  const footer = page.getByRole("contentinfo");
  await expect(footer.getByRole("link", { name: "IP Geolocation by DB-IP" })).toHaveAttribute(
    "href",
    "https://db-ip.com",
  );

  // Landmarks и порядок фокуса каркаса: SkipLink первым, затем шапка.
  await expect(page.getByRole("banner")).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Основная навигация" })).toBeVisible();
  await expect(page.locator("main#main")).toBeVisible();
  await page.keyboard.press("Tab");
  const skip = page.getByRole("link", { name: "Перейти к содержимому" });
  await expect(skip).toBeFocused();
  await expect(skip).toBeVisible();
  await expectNoA11yViolations(page);
});
