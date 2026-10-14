// Демо-режим (решение владельца по макету «Dashboard v2»): дашборд на статичном синтетическом
// датасете без входа. Признак — cookie am_demo=1: его ставит GET /demo, снимает GET /demo/exit.
// Cookie не HttpOnly: браузерный fetcher читает его, чтобы не ходить в API. Секретом он не является —
// в демо-режиме данные берутся только из lib/demo, Go API не вызывается.
export const DEMO_COOKIE = "am_demo";

export function hasDemoCookie(header: string | null | undefined): boolean {
  if (!header) return false;
  return header.split(";").some((part) => part.trim() === `${DEMO_COOKIE}=1`);
}

export function isDemoInBrowser(): boolean {
  return typeof document !== "undefined" && hasDemoCookie(document.cookie);
}
