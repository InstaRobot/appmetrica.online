import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

// S-03 портфель и S-04 создание приложения (T-037). Владелец уже вошёл: storageState из global-setup.

async function expectNoA11yViolations(page: Page): Promise<void> {
  const { violations } = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(violations).toEqual([]);
}

test("creates app from portfolio", async ({ page }) => {
  // Имя уникально на прогон: база e2e переживает прогоны, а имена приложений уникальны без учёта регистра.
  const name = `E2E ${Date.now().toString(36)}`;

  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "Портфель" })).toBeVisible();
  await page.getByRole("main").getByRole("link", { name: "Добавить приложение" }).first().click();

  await expect(page).toHaveURL("/apps/new");
  await expect(page.getByRole("heading", { level: 1, name: "Новое приложение" })).toBeVisible();
  await expectNoA11yViolations(page);

  await page.getByRole("textbox", { name: "Название" }).fill(name);
  await page.getByRole("combobox", { name: "Платформа" }).click();
  await page.getByRole("option", { name: "iOS" }).click();
  await expect(page.getByRole("combobox", { name: "Часовой пояс" })).not.toHaveText(
    "Выберите часовой пояс",
  );
  await page.getByRole("button", { name: "Создать" }).click();

  await expect(page).toHaveURL(/\/apps\/[0-9a-f-]{36}\/integration$/);
  const appId = new URL(page.url()).pathname.split("/")[2];

  await page.goto("/");
  const table = page.getByRole("table", { name: /Портфель приложений за/ });
  const link = table.getByRole("link", { name, exact: true });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("href", `/apps/${appId}`);
  await expectNoA11yViolations(page);
});

test("portfolio keeps environment in the URL", async ({ page }) => {
  await page.goto("/");
  const env = page.getByRole("group", { name: "Окружение" });
  await env.getByRole("radio", { name: "development" }).click();

  await expect(page).toHaveURL(/[?&]env=development/);
  await expect(env.getByRole("radio", { name: "development" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await page.reload();
  await expect(
    page.getByRole("group", { name: "Окружение" }).getByRole("radio", { name: "development" }),
  ).toHaveAttribute("aria-checked", "true");
});
