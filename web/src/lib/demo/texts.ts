// Тексты демо-датасета на языке интерфейса. Демо-слой — отдельный чанк, который грузится только в
// демо-режиме (lib/api/fetcher.ts), поэтому он берёт из словаря лишь пространство имён demoData обоих
// языков: «@/messages» целиком в клиентский бандл не попадает.
import {
  DEFAULT_LOCALE,
  isLocale,
  LOCALE_COOKIE,
  type Locale,
  resolveLocale,
} from "@/lib/i18n/locale";
import { demoData as en } from "@/messages/en/demoData";
import { demoData as ru } from "@/messages/ru/demoData";

export type DemoTexts = typeof en;

const TEXTS: Record<Locale, DemoTexts> = { en, ru };

export function demoTextsFor(locale: Locale): DemoTexts {
  return TEXTS[locale];
}

// Язык ответа демо-API. В браузере — <html lang>: его ставит inline-скрипт до отрисовки и уточняет
// HtmlLang по языку сервера, то есть это язык, на котором показан интерфейс. На сервере — тот же выбор,
// что у getRequestLocale (cookie → Accept-Language → EN); вне запроса — EN.
export async function demoLocale(): Promise<Locale> {
  if (typeof document !== "undefined") {
    const lang = document.documentElement.lang;
    return isLocale(lang) ? lang : DEFAULT_LOCALE;
  }
  try {
    const { cookies, headers } = await import("next/headers");
    const [jar, list] = await Promise.all([cookies(), headers()]);
    return resolveLocale({
      cookie: jar.get(LOCALE_COOKIE)?.value,
      acceptLanguage: list.get("accept-language"),
    });
  } catch {
    return DEFAULT_LOCALE;
  }
}
