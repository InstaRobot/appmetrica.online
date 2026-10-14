import { cache } from "react";
import { getAuthState } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";

// isOwner — есть ли у запроса действующая сессия владельца. Документация публичная (S-13, решение
// владельца от 2026-10-14): от ответа зависит только шапка. proxy.ts пропускает любую cookie
// am_session, поэтому валидность спрашиваем у API; недоступный API — тоже гость, а не ошибка доков.
// cache() — один запрос на рендер.
export const isOwner = cache(async (): Promise<boolean> => {
  try {
    const state = await getAuthState();
    return state.data.authenticated;
  } catch {
    return false;
  }
});
