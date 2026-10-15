import { expect, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// Каркас дашборда (T-036) и документация (T-047, T-048). Владелец уже вошёл: storageState из global-setup.

test.describe("without session", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  // Документация публичная (решение владельца от 2026-10-14): гость читает её без входа,
  // а в шапке видит «Войти» вместо меню дашборда.
  test("docs are public", async ({ page }) => {
    const res = await page.goto("/docs/recipes/ios-swift");

    expect(res?.status()).toBe(200);
    await expect(page).toHaveURL(/\/docs\/recipes\/ios-swift$/);
    await expect(page.getByRole("heading", { level: 1, name: "iOS (Swift)" })).toBeVisible();
    await expect(page.getByRole("banner").getByRole("link", { name: "Войти" })).toHaveAttribute(
      "href",
      "/login",
    );
    await expectNoA11yViolations(page);
  });

  // Недействительная cookie сессии не уводит на вход: гость видит ту же документацию.
  test("docs ignore an invalid session cookie", async ({ page, context, baseURL }) => {
    await context.addCookies([{ name: "am_session", value: "invalid", url: baseURL ?? "" }]);

    await page.goto("/docs/api-reference/ingest/ingest-events");

    await expect(page).toHaveURL(/\/docs\/api-reference\/ingest\/ingest-events$/);
    await expect(page.locator("#nd-page").getByText("/v1/events").first()).toBeVisible();
    await expectNoA11yViolations(page);
  });

  test("docs search works without a session", async ({ request }) => {
    const res = await request.get("/docs/search?query=session_start");

    expect(res.status()).toBe(200);
    expect(Array.isArray(await res.json())).toBe(true);
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
  await expect(sidebar.getByText("Ingestion API", { exact: true })).toBeVisible();
  await expect(sidebar.getByText("Read API", { exact: true })).toBeVisible();
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
    "data-active",
    "true",
  );
  await expect(sidebar.getByRole("link", { name: "Android (Kotlin)" })).toBeVisible();
  await page.waitForFunction(() =>
    document.getAnimations().every((a) => a.playState !== "running"),
  );
  await expectNoA11yViolations(page);
});

// Гайды связаны между собой: ссылки быстрого старта ведут на существующие страницы, и каждая проходит axe.
test("docs guides link to recipes and concepts", async ({ page }) => {
  await page.goto("/docs");

  const main = page.locator("#nd-page");
  await main.getByRole("link", { name: "Веб (JavaScript)" }).first().click();
  await expect(page).toHaveURL(/\/docs\/recipes\/web-js$/);
  await expect(page.getByRole("heading", { level: 1, name: "Веб (JavaScript)" })).toBeVisible();
  await expect(page.locator("#nd-page").getByText("navigator.sendBeacon").first()).toBeVisible();

  await page.goto("/docs");
  await page
    .locator("#nd-page")
    .getByRole("link", { name: "Где инициализировать метрику" })
    .first()
    .click();
  await expect(page).toHaveURL(/\/docs\/concepts\/initialization$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Где инициализировать метрику" }),
  ).toBeVisible();

  for (const path of [
    "/docs/concepts/initialization",
    "/docs/concepts/identity",
    "/docs/concepts/sessions",
    "/docs/concepts/time-and-offline",
    "/docs/recipes/curl",
    "/docs/recipes/android-kotlin",
    "/docs/recipes/react-native",
    "/docs/recipes/flutter",
    "/docs/recipes/kotlin-multiplatform",
    "/docs/recipes/bots-and-backends",
    "/docs/recipes/go",
    "/docs/errors-and-retries",
    "/docs/privacy-and-gdpr",
    "/docs/read-api/overview",
  ]) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("Страница не найдена");
    await expectNoA11yViolations(page);
  }
});

// Рецепт «Go и curl» разделён на «curl» и «Go»: старый адрес навсегда ведёт на рецепты curl.
test("old go-and-curl recipe redirects to curl", async ({ page }) => {
  const res = await page.goto("/docs/recipes/go-and-curl");

  // Первый шаг — постоянный редирект next.config; русский браузер прогона proxy затем ведёт на /ru/docs/….
  let first = res?.request();
  while (first?.redirectedFrom()) first = first.redirectedFrom() ?? undefined;
  expect((await first?.response())?.status()).toBe(308);
  await expect(page).toHaveURL(/\/docs\/recipes\/curl$/);
  await expect(page.getByRole("heading", { level: 1, name: "curl", exact: true })).toBeVisible();
  // Клиент на Go переехал на свою страницу: рецепт curl ссылается на неё.
  await expect(
    page.locator("#nd-page").getByRole("link", { name: "Go", exact: true }).first(),
  ).toHaveAttribute("href", "/ru/docs/recipes/go");
  await expectNoA11yViolations(page);
});

test("api reference lists public operations without playground", async ({ page }) => {
  await page.goto("/docs/api-reference/ingest/ingest-events");

  const main = page.locator("#nd-page");
  await expect(main.getByText("/v1/events").first()).toBeVisible();
  await expect(main.getByText("POST").first()).toBeVisible();
  await expect(main.getByRole("button", { name: /^(Send|Отправить)$/ })).toHaveCount(0);

  const sidebar = page.locator("#nd-sidebar");
  await sidebar.getByText("Read API", { exact: true }).click();
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
    "/ru/docs",
  );
  await expectNoA11yViolations(page);
});

test("footer shows DB-IP attribution", async ({ page }) => {
  await page.goto("/portfolio");

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
