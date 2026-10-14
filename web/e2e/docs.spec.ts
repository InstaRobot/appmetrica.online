import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Каркас дашборда (T-036) и документация (T-047, T-048). Владелец уже вошёл: storageState из global-setup.

test.describe("without session", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  // AC-047.2: без cookie proxy отправляет на вход и сохраняет адрес страницы в next.
  test("docs require login", async ({ page }) => {
    await page.goto("/docs/recipes/ios-swift");

    await expect(page).toHaveURL(/\/login\?next=%2Fdocs%2Frecipes%2Fios-swift$/);
    await expect(
      page.getByRole("heading", { level: 1, name: "Вход в appmetrica.online" }),
    ).toBeVisible();
    await expectNoA11yViolations(page);
  });

  // Cookie есть, но сессия недействительна: proxy пропускает, layout доков проверяет её через API.
  test("docs reject an invalid session cookie", async ({ page, context, baseURL }) => {
    await context.addCookies([{ name: "am_session", value: "invalid", url: baseURL ?? "" }]);

    await page.goto("/docs/api-reference/ingest/ingest-events");

    await expect(page).toHaveURL(/\/login\?next=%2Fdocs$/);
    await expect(page.getByText("Ingestion API")).toHaveCount(0);
    await expectNoA11yViolations(page);
  });
});

test("docs render for owner without caching", async ({ page }) => {
  const res = await page.goto("/docs");

  expect(res?.status()).toBe(200);
  expect(res?.headers()["cache-control"]).toMatch(/private/);
  expect(res?.headers()["cache-control"]).toMatch(/no-store/);
  await expect(page.getByRole("banner")).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: "Быстрый старт" })).toBeVisible();
  const sidebar = page.locator("#nd-sidebar");
  await expect(sidebar.getByText("Ingestion API")).toBeVisible();
  await expect(sidebar.getByText("Read API")).toBeVisible();
  await expectNoA11yViolations(page);
});

test("api reference lists public operations without playground", async ({ page }) => {
  await page.goto("/docs/api-reference/ingest/ingest-events");

  const main = page.locator("#nd-page");
  await expect(main.getByText("/v1/events").first()).toBeVisible();
  await expect(main.getByText("POST").first()).toBeVisible();
  await expect(main.getByRole("button", { name: /^(Send|Отправить)$/ })).toHaveCount(0);

  const sidebar = page.locator("#nd-sidebar");
  await sidebar.getByText("Read API").click();
  await expect(sidebar.getByRole("link", { name: /portfolio/i })).toBeVisible();
  await expect(sidebar.getByRole("link", { name: /auth|passkey|settings/i })).toHaveCount(0);
  await expectNoA11yViolations(page);
});

test("unknown docs page shows not found", async ({ page }) => {
  const res = await page.goto("/docs/no-such-page");

  expect(res?.status()).toBe(404);
  await expect(page.getByRole("link", { name: "К быстрому старту" })).toHaveAttribute(
    "href",
    "/docs",
  );
  await expectNoA11yViolations(page);
});

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
