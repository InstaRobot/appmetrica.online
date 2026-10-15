// Адаптер церемоний go-webauthn ↔ @simplewebauthn/browser 14.0.0 для S-01 /login и S-02 /setup/[token].
// Бэкенд отдаёт options = {"publicKey": …, "mediation"?: "conditional"}; в браузер уходит options.publicKey
// как optionsJSON, ответ аутентификатора возвращается в finish как есть (06-api-contracts.md /admin/v1/auth).
import {
  browserSupportsWebAuthn,
  browserSupportsWebAuthnAutofill,
  type PublicKeyCredentialCreationOptionsJSON,
  type PublicKeyCredentialRequestOptionsJSON,
  startAuthentication,
  startRegistration,
  WebAuthnAbortService,
  WebAuthnError,
} from "@simplewebauthn/browser";
import { ApiError } from "@/lib/api/fetcher";
import {
  beginLogin,
  beginSetup,
  finishLogin,
  finishSetup,
} from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import {
  beginPasskeyRegistration,
  finishPasskeyRegistration,
} from "@/lib/api/gen/endpoints/admin-passkeys/admin-passkeys";
import type { WebAuthnCeremonyOptionsOptions } from "@/lib/api/gen/model";
import type { Messages } from "@/messages";

export type PasskeyFlow = "login" | "setup";
export type DeviceNameErrors = Messages["auth"]["deviceName"];
export type PasskeyErrorTexts = Messages["auth"]["passkeyErrors"];

// Русские тексты для вызовов без словаря (S-12 «Добавить passkey», пока настройки не переведены);
// удаляются вместе с русскими обёртками lib/format. Экраны входа передают словарь auth.
const RU_DEVICE_NAME_ERRORS: DeviceNameErrors = {
  required: "Введите название устройства",
  tooLong: "Не длиннее 64 символов",
};
const RU_PASSKEY_ERRORS: PasskeyErrorTexts = {
  login: "Не удалось войти. Подтверждение отменено или ключ не подошёл. Попробуйте ещё раз.",
  setup: "Не удалось создать passkey. Попробуйте ещё раз.",
  network: "Нет соединения с сервером. Проверьте подключение.",
  rateLimited: "Слишком много попыток. Повторите через минуту.",
  sessionExpired: "Сессия устарела. Обновите страницу.",
};

export const DEVICE_NAME_MAX = 64;

export function supportsPasskeys(): boolean {
  return browserSupportsWebAuthn();
}

export function supportsPasskeyAutofill(): Promise<boolean> {
  return browserSupportsWebAuthnAutofill();
}

// Название passkey по умолчанию на S-02 (14-ux-ui-spec.md S-02, состояние idle); fallback — «Устройство»
// на языке интерфейса.
export function deviceNameFromUserAgent(userAgent: string, fallback: string): string {
  if (/iPhone/.test(userAgent)) return "iPhone";
  if (/Macintosh|Mac OS X/.test(userAgent)) return "MacBook";
  if (/Windows/.test(userAgent)) return "Windows PC";
  return fallback;
}

// F-6: название обязательно, 1–64 символа после trim. null — ошибок нет.
export function validateDeviceName(
  name: string,
  m: DeviceNameErrors = RU_DEVICE_NAME_ERRORS,
): string | null {
  const trimmed = name.trim();
  if (!trimmed) return m.required;
  if ([...trimmed].length > DEVICE_NAME_MAX) return m.tooLong;
  return null;
}

function publicKeyOf(options: WebAuthnCeremonyOptionsOptions): object {
  const publicKey = options.publicKey;
  if (!publicKey || typeof publicKey !== "object") {
    throw new Error("webauthn: ceremony options have no publicKey");
  }
  return publicKey;
}

// Ответ бэкенда приходит от go-webauthn и уже имеет форму *OptionsJSON: структуру проверяет сам браузер.
const requestOptionsJSON = (options: WebAuthnCeremonyOptionsOptions) =>
  publicKeyOf(options) as PublicKeyCredentialRequestOptionsJSON;
const creationOptionsJSON = (options: WebAuthnCeremonyOptionsOptions) =>
  publicKeyOf(options) as PublicKeyCredentialCreationOptionsJSON;

// Вход по discoverable passkey. autofill=true — conditional UI: браузер предлагает passkey в автозаполнении
// поля `autocomplete="username webauthn"`, промис ждёт выбора пользователя. signal отменяет вход до обращения
// к аутентификатору: иначе запоздавший conditional UI прервал бы церемонию, начатую кнопкой.
export async function loginWithPasskey({
  autofill,
  signal,
}: {
  autofill: boolean;
  signal?: AbortSignal;
}): Promise<void> {
  const begin = await beginLogin(signal ? { signal } : undefined);
  if (begin.status !== 200) throw new Error(`webauthn: beginLogin ${begin.status}`);
  signal?.throwIfAborted();
  const credential = await startAuthentication({
    optionsJSON: requestOptionsJSON(begin.data.options),
    useBrowserAutofill: autofill,
  });
  await finishLogin({ ceremony_id: begin.data.ceremony_id, credential: { ...credential } });
}

// Регистрация passkey по одноразовой ссылке из `appmetrica admin passkey-link`; успех ставит cookie сессии.
export async function registerPasskeyBySetupLink(token: string, name: string): Promise<void> {
  const begin = await beginSetup(token);
  if (begin.status !== 200) throw new Error(`webauthn: beginSetup ${begin.status}`);
  const credential = await startRegistration({
    optionsJSON: creationOptionsJSON(begin.data.options),
  });
  await finishSetup(token, {
    ceremony_id: begin.data.ceremony_id,
    credential: { ...credential },
    name,
  });
}

// Дополнительный passkey вошедшего владельца (S-12 «Добавить passkey»): ещё одно устройство или ключ.
export async function registerAdditionalPasskey(name: string): Promise<void> {
  const begin = await beginPasskeyRegistration();
  if (begin.status !== 200) throw new Error(`webauthn: beginPasskeyRegistration ${begin.status}`);
  const credential = await startRegistration({
    optionsJSON: creationOptionsJSON(begin.data.options),
  });
  await finishPasskeyRegistration({
    ceremony_id: begin.data.ceremony_id,
    credential: { ...credential },
    name,
  });
}

// Отменяет ожидающую церемонию (conditional UI при уходе со страницы).
export function cancelPasskeyCeremony(): void {
  WebAuthnAbortService.cancelCeremony();
}

export function isCeremonyAborted(err: unknown): boolean {
  if (err instanceof WebAuthnError) return err.code === "ERROR_CEREMONY_ABORTED";
  return err instanceof DOMException && err.name === "AbortError";
}

// 410 setup_token_expired — ссылка истекла или использована; 404 — такой ссылки нет. Обе — состояние expired.
export function isSetupLinkExpired(err: unknown): boolean {
  return err instanceof ApiError && (err.status === 410 || err.status === 404);
}

// Текст ошибки под кнопкой (14-ux-ui-spec.md S-01/S-02 и маппинг кодов ошибок). Отмена в системном диалоге
// (NotAllowedError), 422 webauthn_verification_failed и прочие сбои церемонии — общий текст экрана.
export function passkeyErrorMessage(
  err: unknown,
  flow: PasskeyFlow,
  m: PasskeyErrorTexts = RU_PASSKEY_ERRORS,
): string {
  if (err instanceof ApiError) {
    if (err.code === "network_error") return m.network;
    if (err.status === 429) return m.rateLimited;
    if (err.code === "csrf_check_failed") return m.sessionExpired;
  }
  return m[flow];
}
