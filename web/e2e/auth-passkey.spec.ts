import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";
import { createSetupLink, registerPasskey, resetAuth, saveOwnerSession } from "./helpers/seed";
import { addVirtualAuthenticator } from "./helpers/webauthn";

// S-01 /login и S-02 /setup/[token] (T-019). Каждый тест начинает с пустой auth-базы и без cookie,
// поэтому после спецификации владелец и e2e/.auth/owner.json создаются заново для остальных спецификаций.
test.describe.configure({ mode: "serial" });
test.use({ storageState: { cookies: [], origins: [] } });

test.beforeEach(async () => {
  await resetAuth();
});

test.afterAll(async ({ baseURL }) => {
  await saveOwnerSession(baseURL ?? "http://localhost:3000");
});

async function sessionCookie(page: Page) {
  return (await page.context().cookies()).find((c) => c.name === "am_session");
}

// Автоподтверждение виртуального аутентификатора действует только на запросы, пришедшие после его включения:
// церемония, попавшая к нему раньше, висит до таймаута (так тест падал в CI). Поэтому begin церемонии по кнопке
// придерживается, пока тест проверяет состояние ожидания и включает присутствие; release отпускает запрос.
async function holdLoginBegin(page: Page) {
  let release = () => {};
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/admin/v1/auth/login/begin", async (route) => {
    await released;
    await route.continue().catch(() => {}); // запрос conditional UI мог быть отменён кликом
  });
  return release;
}

test("first setup registers passkey and lands on portfolio", async ({ page }) => {
  await addVirtualAuthenticator(page);
  const link = await createSetupLink();

  await page.goto(link);
  await expect(page.getByRole("heading", { level: 1, name: "Регистрация passkey" })).toBeVisible();
  await expectNoA11yViolations(page);
  await page.getByLabel("Название устройства").fill("MacBook");
  await page.getByRole("button", { name: "Создать passkey" }).click();

  await expect(page.getByText("Passkey создан. Переходим в дашборд…")).toBeVisible();
  await expect(page).toHaveURL("/portfolio");
  expect(await sessionCookie(page)).toMatchObject({ httpOnly: true, sameSite: "Lax", path: "/" });

  // Вошедшего владельца /login сразу отправляет в портфель.
  await page.goto("/login");
  await expect(page).toHaveURL("/portfolio");

  // Ссылка одноразовая: повторная регистрация по ней показывает состояние expired.
  await page.goto(link);
  await page.getByRole("button", { name: "Создать passkey" }).click();
  await expect(page.getByRole("status")).toContainText("Ссылка устарела или уже использована");
  await expect(page.getByLabel("Название устройства")).toHaveCount(0);
});

test("login with passkey", async ({ page }) => {
  // Пустая база: вместо кнопки — инструкция по passkey-link.
  await page.goto("/login");
  await expect(page.getByText("Passkey ещё не настроен")).toBeVisible();
  await expect(page.getByRole("button", { name: "Войти с passkey" })).toHaveCount(0);

  const authenticator = await addVirtualAuthenticator(page);
  await registerPasskey(page, "MacBook");
  await page.context().clearCookies();
  await authenticator.setUserPresence(false);

  // Без сессии proxy отправляет на /login?next=…, после входа — обратно на исходный путь.
  await page.goto("/settings?tab=passkeys");
  await expect(page).toHaveURL("/login?next=%2Fsettings%3Ftab%3Dpasskeys");
  await expect(
    page.getByRole("heading", { level: 1, name: "Вход в appmetrica.online" }),
  ).toBeVisible();
  await expectNoA11yViolations(page);
  let release = await holdLoginBegin(page);
  await page.getByRole("button", { name: "Войти с passkey" }).click();
  await expect(page.getByRole("button", { name: "Ожидаем подтверждение…" })).toBeDisabled();
  await authenticator.setUserPresence(true);
  release();
  await expect(page).toHaveURL("/settings?tab=passkeys");
  expect(await sessionCookie(page)).toBeDefined();

  // Без next — в портфель.
  await page.context().clearCookies();
  await authenticator.setUserPresence(false);
  await page.goto("/login");
  release = await holdLoginBegin(page);
  await page.getByRole("button", { name: "Войти с passkey" }).click();
  await authenticator.setUserPresence(true);
  release();
  await expect(page).toHaveURL("/portfolio");
  expect(await sessionCookie(page)).toBeDefined();
});

test("login with passkey autofill", async ({ page }) => {
  await addVirtualAuthenticator(page);
  await registerPasskey(page, "MacBook");
  await page.context().clearCookies();

  // Conditional UI стартует при загрузке /login; виртуальный аутентификатор подтверждает его без клика.
  await page.goto("/login?next=%2Fapps%2Fnew");
  await expect(page).toHaveURL("/apps/new");
  expect(await sessionCookie(page)).toBeDefined();
});
