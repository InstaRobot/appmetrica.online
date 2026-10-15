// Ошибки API → текст для пользователя на его языке (14-ux-ui-spec.md §8.3, §14.3). Тексты по коду —
// messages/<язык>/errors.ts; в компонентах — useUiError().
import { ApiError } from "@/lib/api/fetcher";
import { fmt } from "@/lib/i18n/format-message";
import type { Messages } from "@/messages";

export type UiError = { code: string; message: string; requestId?: string; status?: number };

type MessageOptions = { message?: string; retryAfter?: number };

export type ErrorMessages = Messages["errors"];
export type ApiErrorCode = keyof ErrorMessages["codes"];

function isKnown(code: string, m: ErrorMessages): code is ApiErrorCode {
  return Object.hasOwn(m.codes, code);
}

export function messageFor(code: string, m: ErrorMessages, opts: MessageOptions = {}): string {
  if (code === "invalid_request" && opts.message) {
    return fmt(m.invalidRequestDetail, { message: opts.message });
  }
  if (code === "rate_limited" && opts.retryAfter) {
    return fmt(m.rateLimitedRetry, { seconds: opts.retryAfter });
  }
  return isKnown(code, m) ? m.codes[code] : m.generic;
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

function fromCode(
  m: ErrorMessages,
  code: string,
  message: string,
  requestId?: string,
  status?: number,
): UiError {
  const text =
    !isKnown(code, m) && status !== undefined && status >= 500
      ? m.server
      : messageFor(code, m, { message });
  return {
    code,
    message: text,
    ...(requestId ? { requestId } : {}),
    ...(status !== undefined ? { status } : {}),
  };
}

// toUiError приводит любое пойманное значение к тексту для пользователя. Сообщение сервера показывается
// только для invalid_request: остальные тексты фиксированы, чтобы не выводить внутренние детали.
export function toUiError(error: unknown, m: ErrorMessages): UiError {
  if (error instanceof ApiError) {
    return fromCode(m, error.code, error.message, error.requestId, error.status);
  }
  if (isErrorLike(error)) {
    return fromCode(m, error.code, error.message, error.request_id, error.status);
  }
  // fetch вне apiFetch бросает TypeError («Failed to fetch») при обрыве сети.
  if (error instanceof TypeError) {
    return { code: "network_error", message: m.codes.network_error };
  }
  return { code: "unknown", message: m.generic };
}
