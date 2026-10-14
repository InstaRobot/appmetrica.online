"use client";

import { useSyncExternalStore } from "react";
import { readTheme, type Theme } from "@/lib/theme";

// Тему выставляет inline-скрипт до гидрации, поэтому источник правды — атрибут <html data-theme>.
// На сервере темы нет: первая отрисовка идёт как light, затем React сверяется с атрибутом.
function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  return () => observer.disconnect();
}

const serverTheme = (): Theme => "light";

export function useTheme(): Theme {
  return useSyncExternalStore(subscribe, readTheme, serverTheme);
}
