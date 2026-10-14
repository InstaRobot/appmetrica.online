import { chromium, expect, type Page } from "@playwright/test";
import { addVirtualAuthenticator } from "./webauthn";

// Служебные маршруты api, открытые только при APP_ENV=e2e (backend/internal/api/e2e_handlers.go).
const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";

export const OWNER_STATE = "e2e/.auth/owner.json";

async function post(path: string): Promise<Response> {
  const res = await fetch(`${API_URL}${path}`, { method: "POST" });
  if (!res.ok) {
    throw new Error(`POST ${path}: ${res.status}. Запущен ли api с APP_ENV=e2e на ${API_URL}?`);
  }
  return res;
}

// resetAuth возвращает Postgres к состоянию «passkey ещё не настроен»: owner, passkey, сессии, ссылки и церемонии.
export async function resetAuth(): Promise<void> {
  await post("/internal/e2e/reset-auth");
}

// createSetupLink — путь `/setup/<token>` одноразовой ссылки, как из `appmetrica admin passkey-link`.
export async function createSetupLink(): Promise<string> {
  const { url } = (await (await post("/internal/e2e/passkey-link")).json()) as { url: string };
  return new URL(url).pathname;
}

// registerPasskey проходит S-02 на странице с уже добавленным виртуальным аутентификатором.
export async function registerPasskey(page: Page, name: string): Promise<void> {
  await page.goto(await createSetupLink());
  await page.getByLabel("Название устройства").fill(name);
  await page.getByRole("button", { name: "Создать passkey" }).click();
  await expect(page).toHaveURL("/portfolio");
}

// saveOwnerSession сбрасывает auth, регистрирует владельца с нуля и сохраняет cookie в OWNER_STATE
// для спецификаций, которым нужен вошедший владелец (storageState в playwright.config.ts).
export async function saveOwnerSession(baseURL: string): Promise<void> {
  await resetAuth();
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ baseURL });
    const page = await context.newPage();
    await addVirtualAuthenticator(page);
    await registerPasskey(page, "E2E");
    await context.storageState({ path: OWNER_STATE });
  } finally {
    await browser.close();
  }
}
