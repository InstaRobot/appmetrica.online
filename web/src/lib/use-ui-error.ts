import { useCallback } from "react";
import { toUiError, type UiError } from "@/lib/errors";
import { useMessages } from "@/lib/i18n/client";

// Текст ошибки API на языке интерфейса: const uiError = useUiError(); toast.error(uiError(error).message).
export function useUiError(): (error: unknown) => UiError {
  const m = useMessages().errors;
  return useCallback((error: unknown) => toUiError(error, m), [m]);
}
