// Тексты ошибок API для UI (14-ux-ui-spec.md §8.3, §14.3). Коды — из components.responses
// contracts/openapi/api.yaml; тест errors.test.ts проверяет, что у каждого кода контракта есть текст.
import { ApiError } from "@/lib/api/fetcher";

export type UiError = { code: string; message: string; requestId?: string; status?: number };

type MessageOptions = { message?: string; retryAfter?: number };

export const GENERIC_ERROR = "Что-то пошло не так. Повторите попытку.";
const SERVER_ERROR = "Ошибка сервера. Повторите через минуту.";

export const API_ERRORS = {
  invalid_request: "Некорректный запрос.",
  range_too_large: "Слишком большой период: максимум 366 дней, для детализации по часам — 7 дней.",
  unauthenticated: "Сессия истекла. Войдите снова.",
  insufficient_scope: "Недостаточно прав для этого действия.",
  csrf_check_failed: "Сессия устарела. Обновите страницу.",
  // Не код контракта: так отвечает статичный датасет демо-режима на любые изменения (lib/demo).
  demo_read_only: "Демо-режим: изменения недоступны",
  app_not_found: "Приложение не найдено или удалено",
  profile_not_found: "Профиль не найден",
  not_found: "Объект не найден или был удалён.",
  last_passkey: "Нельзя удалить последний passkey — сначала добавьте другой.",
  app_name_taken: "Приложение с таким названием уже есть",
  confirm_name_mismatch: "Название не совпадает",
  deletion_pending: "Приложение ожидает удаления: ключи и возврат из архива недоступны",
  setup_token_expired: "Ссылка устарела или уже использована",
  webauthn_verification_failed:
    "Не удалось проверить passkey. Попробуйте ещё раз или используйте другой ключ.",
  rate_limited: "Слишком много запросов. Повторим автоматически.",
  internal_error: SERVER_ERROR,
  storage_unavailable: "Хранилище данных временно недоступно. Повторим автоматически.",
  network_error: "Нет соединения с сервером. Проверьте подключение.",
} as const satisfies Record<string, string>;

export type ApiErrorCode = keyof typeof API_ERRORS;

function isKnown(code: string): code is ApiErrorCode {
  return Object.hasOwn(API_ERRORS, code);
}

export function messageFor(code: string, opts: MessageOptions = {}): string {
  if (code === "invalid_request" && opts.message) return `Некорректный запрос: ${opts.message}`;
  if (code === "rate_limited" && opts.retryAfter) {
    return `Слишком много запросов. Повторим через ${opts.retryAfter} с.`;
  }
  return isKnown(code) ? API_ERRORS[code] : GENERIC_ERROR;
}

type ErrorLike = { code: string; message: string; request_id?: string; status?: number };

function isErrorLike(value: unknown): value is ErrorLike {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { code?: unknown }).code === "string" &&
    typeof (value as { message?: unknown }).message === "string"
  );
}

function fromCode(code: string, message: string, requestId?: string, status?: number): UiError {
  const text =
    !isKnown(code) && status !== undefined && status >= 500
      ? SERVER_ERROR
      : messageFor(code, { message });
  return {
    code,
    message: text,
    ...(requestId ? { requestId } : {}),
    ...(status !== undefined ? { status } : {}),
  };
}

// toUiError приводит любое пойманное значение к тексту для пользователя. Сообщение сервера показывается
// только для invalid_request: остальные тексты фиксированы, чтобы не выводить внутренние детали.
export function toUiError(error: unknown): UiError {
  if (error instanceof ApiError) {
    return fromCode(error.code, error.message, error.requestId, error.status);
  }
  if (isErrorLike(error)) {
    return fromCode(error.code, error.message, error.request_id, error.status);
  }
  // fetch вне apiFetch бросает TypeError («Failed to fetch») при обрыве сети.
  if (error instanceof TypeError) {
    return { code: "network_error", message: API_ERRORS.network_error };
  }
  return { code: "unknown", message: GENERIC_ERROR };
}
