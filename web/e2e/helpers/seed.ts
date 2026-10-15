import { randomUUID } from "node:crypto";
import { type APIRequestContext, chromium, expect, type Page } from "@playwright/test";
import { addVirtualAuthenticator } from "./webauthn";

// Служебные маршруты api, открытые только при APP_ENV=e2e (backend/internal/api/e2e_handlers.go).
const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";
// Локальный ingest, пишущий в ту же базу ClickHouse, что читает api.
export const INGEST_URL = process.env.E2E_INGEST_URL ?? "http://localhost:8081";

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
    // Тот же язык, что у спецификаций (locale в playwright.config.ts): страница входа отвечает на языке браузера.
    const context = await browser.newContext({ baseURL, locale: "ru-RU" });
    const page = await context.newPage();
    await addVirtualAuthenticator(page);
    await registerPasskey(page, "E2E");
    await context.storageState({ path: OWNER_STATE });
  } finally {
    await browser.close();
  }
}

type AppList = { items: { id: string; name: string }[] };

// appIdByName — id приложения по точному названию среди активных и архивных; undefined — такого нет.
export async function appIdByName(
  request: APIRequestContext,
  name: string,
): Promise<string | undefined> {
  const res = await request.get("/api/v1/apps?include_archived=true");
  expect(res.ok()).toBe(true);
  return ((await res.json()) as AppList).items.find((a) => a.name === name)?.id;
}

type CreatedApp = { app: { id: string }; write_keys: { key: string; platform: string | null }[] };

// createApp создаёт приложение через admin API (как форма S-04) и возвращает его id и write-ключ платформы.
// Запрос идёт с cookie владельца из storageState; X-Requested-With — CSRF-защита мутаций admin API.
export async function createApp(
  request: APIRequestContext,
  name: string,
  platforms: string[],
): Promise<{ id: string; key: (platform: string) => string }> {
  const res = await request.post("/api/admin/v1/apps", {
    headers: { "X-Requested-With": "appmetrica-web" },
    data: { name, platforms, timezone: "Europe/Moscow" },
  });
  expect(res.status(), `createApp ${name}: ${await res.text()}`).toBe(201);
  const body = (await res.json()) as CreatedApp;
  return {
    id: body.app.id,
    key: (platform) => {
      const k = body.write_keys.find((w) => w.platform === platform);
      if (!k) throw new Error(`У ${name} нет ключа платформы ${platform}`);
      return k.key;
    },
  };
}

// sendSession отправляет session_start новой установки «сейчас» без context.platform — платформа события
// берётся из ключа — и возвращает install_id. Новый или перенесённый ключ ingest видит после перезагрузки
// кэша ключей (KEY_CACHE_REFRESH, 15 с): до неё ответ 401, поэтому запрос повторяется до 200.
export async function sendSession(request: APIRequestContext, key: string): Promise<string> {
  const installId = randomUUID();
  await expect
    .poll(
      async () => {
        const now = new Date().toISOString();
        const res = await request.post(`${INGEST_URL}/v1/events`, {
          headers: { "X-App-Key": key },
          data: {
            sent_at: now,
            context: { app_version: "1.0.0", locale: "ru-RU", environment: "production" },
            events: [
              {
                insert_id: randomUUID(),
                name: "session_start",
                timestamp: now,
                install_id: installId,
                session_id: randomUUID(),
              },
            ],
          },
        });
        return res.status();
      },
      { message: `ingest ${INGEST_URL} принимает ключ`, timeout: 30_000, intervals: [1_000] },
    )
    .toBe(200);
  return installId;
}
