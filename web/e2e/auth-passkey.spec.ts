import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
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

async function expectNoA11yViolations(page: Page): Promise<void> {
  const { violations } = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(violations).toEqual([]);
}

async function sessionCookie(page: Page) {
  return (await page.context().cookies()).find((c) => c.name === "am_session");
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
  await expect(page).toHaveURL("/");
  expect(await sessionCookie(page)).toMatchObject({ httpOnly: true, sameSite: "Lax", path: "/" });

  // Вошедшего владельца /login сразу отправляет в портфель.
  await page.goto("/login");
  await expect(page).toHaveURL("/");

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
  await page.getByRole("button", { name: "Войти с passkey" }).click();
  await expect(page.getByRole("button", { name: "Ожидаем подтверждение…" })).toBeDisabled();
  await authenticator.setUserPresence(true);
  await expect(page).toHaveURL("/settings?tab=passkeys");
  expect(await sessionCookie(page)).toBeDefined();

  // Без next — в портфель.
  await page.context().clearCookies();
  await authenticator.setUserPresence(false);
  await page.goto("/login");
  await page.getByRole("button", { name: "Войти с passkey" }).click();
  await authenticator.setUserPresence(true);
  await expect(page).toHaveURL("/");
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
