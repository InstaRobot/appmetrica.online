"use client";

import { useSyncExternalStore } from "react";

const QUERY = "(prefers-reduced-motion: reduce)";

// Без matchMedia (SSR, старые окружения) анимации считаются разрешёнными.
function mediaQuery(): MediaQueryList | null {
  return typeof window !== "undefined" && typeof window.matchMedia === "function"
    ? window.matchMedia(QUERY)
    : null;
}

function subscribe(onChange: () => void): () => void {
  const mql = mediaQuery();
  mql?.addEventListener("change", onChange);
  return () => mql?.removeEventListener("change", onChange);
}

const getSnapshot = () => mediaQuery()?.matches ?? false;
const getServerSnapshot = () => false;

// usePrefersReducedMotion — включён ли у пользователя `prefers-reduced-motion: reduce`
// (14-ux-ui-spec.md §10.3): EventFeed не подсвечивает новые строки, Recharts рисует без анимации.
export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
