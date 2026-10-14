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

    await expect(page).toHaveURL(/\/login\?next=(%2F|\/)docs$/);
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

// AC-048.3: рецепт — Swift-код с URLSession, install_id в Keychain и офлайн-очередью, вкладки кода и оглавление.
test("docs recipe page renders for owner", async ({ page }) => {
  await page.goto("/docs/recipes/ios-swift");

  await expect(page.getByRole("heading", { level: 1, name: "iOS (Swift)" })).toBeVisible();
  const main = page.locator("#nd-page");
  await expect(main.getByText("URLSession.shared").first()).toBeVisible();
  await expect(main.getByText("kSecClassGenericPassword").first()).toBeVisible();
  await expect(main.getByText("OfflineQueue").first()).toBeVisible();

  // Вкладки кода: соседние блоки с tab="…" собраны в одну группу, вторая вкладка открывает свой код.
  const tabs = main.getByRole("tablist").first();
  await expect(tabs.getByRole("tab", { name: "SwiftUI" })).toHaveAttribute("aria-selected", "true");
  await tabs.getByRole("tab", { name: "UIKit" }).click();
  await expect(tabs.getByRole("tab", { name: "UIKit" })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByText("didEnterBackgroundNotification").first()).toBeVisible();

  const toc = page.locator("#nd-toc");
  await expect(toc.getByRole("link", { name: "Офлайн-очередь" })).toBeVisible();
  await toc.getByRole("link", { name: "Офлайн-очередь" }).click();
  await expect(page).toHaveURL(/#offline-queue$/);

  const sidebar = page.locator("#nd-sidebar");
  await expect(sidebar.getByRole("link", { name: "iOS (Swift)" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(sidebar.getByRole("link", { name: "Android (Kotlin)" })).toBeVisible();
  await page.waitForFunction(() =>
    document.getAnimations().every((a) => a.playState !== "running"),
  );
  await expectNoA11yViolations(page);
});

// Гайды связаны между собой: ссылки быстрого старта ведут на существующие страницы.
test("docs guides link to recipes and concepts", async ({ page }) => {
  await page.goto("/docs");

  const main = page.locator("#nd-page");
  await main.getByRole("link", { name: "Веб (JavaScript)" }).first().click();
  await expect(page).toHaveURL(/\/docs\/recipes\/web-js$/);
  await expect(page.getByRole("heading", { level: 1, name: "Веб (JavaScript)" })).toBeVisible();
  await expect(page.locator("#nd-page").getByText("navigator.sendBeacon").first()).toBeVisible();

  for (const path of [
    "/docs/concepts/identity",
    "/docs/concepts/sessions",
    "/docs/concepts/time-and-offline",
    "/docs/recipes/android-kotlin",
    "/docs/recipes/bots-and-backends",
    "/docs/recipes/go-and-curl",
    "/docs/errors-and-retries",
    "/docs/privacy-and-gdpr",
    "/docs/read-api/overview",
  ]) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("Страница не найдена");
  }
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
  // Папка раскрывается с анимацией прозрачности: axe проверяет контраст после неё.
  await page.waitForFunction(() =>
    document.getAnimations().every((a) => a.playState !== "running"),
  );
  await expectNoA11yViolations(page);
});

// Оболочка доков стримится (Suspense), поэтому notFound() приходит со статусом 200 и noindex.
test("unknown docs page shows not found", async ({ page }) => {
  await page.goto("/docs/no-such-page");

  await expect(page.getByRole("heading", { level: 1, name: "Страница не найдена" })).toBeVisible();
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
