// Заглушка для красного теста T-036; реализация — в следующем коммите.
export type UiError = { code: string; message: string; requestId?: string; status?: number };
export const GENERIC_ERROR = "";
export const API_ERRORS: Record<string, string> = {};
export function messageFor(
  _code: string,
  _opts?: { message?: string; retryAfter?: number },
): string {
  return "";
}
export function toUiError(_error: unknown): UiError {
  return { code: "", message: "" };
}
