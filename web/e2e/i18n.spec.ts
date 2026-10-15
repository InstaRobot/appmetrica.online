import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Языки RU/EN: английский — в корне, русский — /ru; выбор живёт в cookie am_lang год; панель берёт язык из
// cookie без префикса в адресе.
const lang = (page: Page) => page.evaluate(() => document.documentElement.lang);
const savedLocale = async (page: Page) =>
  (await page.context().cookies()).find((c) => c.name === "am_lang")?.value;

test.describe("guest with an English browser", () => {
  test.use({ locale: "en-US", storageState: { cookies: [], origins: [] } });

  test("gets the English landing at the root and can switch to Russian for good", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page).toHaveURL("/");
    expect(await lang(page)).toBe("en");
    await expectNoA11yViolations(page);

    await page.getByRole("button", { name: "Switch to Русский" }).first().click();
    await expect(page).toHaveURL("/ru");
    expect(await lang(page)).toBe("ru");
    expect(await savedLocale(page)).toBe("ru");

    // Следующий визит в корень помнит выбор.
    await page.goto("/");
    await expect(page).toHaveURL("/ru");

    await page.getByRole("button", { name: "Переключить на English" }).first().click();
    await expect(page).toHaveURL("/");
    expect(await savedLocale(page)).toBe("en");
  });

  test("keeps the paired docs page when switching", async ({ page }) => {
    await page.goto("/docs/recipes/curl");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("curl");
    await page.getByRole("button", { name: "Switch to Русский" }).first().click();
    await expect(page).toHaveURL("/ru/docs/recipes/curl");
  });

  test("sends /en back to the address without a prefix", async ({ page }) => {
    await page.goto("/en/docs");
    await expect(page).toHaveURL("/docs");
  });
});

test.describe("guest with a Russian browser", () => {
  test.use({ locale: "ru-RU", storageState: { cookies: [], origins: [] } });

  test("lands on /ru and the Russian docs link to Russian pages", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL("/ru");

    await page.goto("/ru/docs");
    const firstGuide = page.locator('#nd-sidebar a[href^="/ru/docs/"]').first();
    await firstGuide.click();
    await expect(page).toHaveURL(/^http:\/\/[^/]+\/ru\/docs\//);
    expect(await lang(page)).toBe("ru");
    await expectNoA11yViolations(page);
  });
});

test.describe("owner", () => {
  test("sees the panel in the saved language and switches it in place", async ({
    page,
    context,
    baseURL,
  }) => {
    await context.addCookies([{ name: "am_lang", value: "en", url: baseURL ?? "" }]);
    await page.goto("/portfolio");
    await expect(page).toHaveURL("/portfolio");
    expect(await lang(page)).toBe("en");

    await page.getByRole("button", { name: "Switch to Русский" }).click();
    await expect.poll(() => lang(page)).toBe("ru");
    await expect(page).toHaveURL("/portfolio");
    expect(await savedLocale(page)).toBe("ru");
  });
});
