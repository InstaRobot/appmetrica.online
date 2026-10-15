"use client";

import { Button } from "@/components/ui/button";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import {
  LANGUAGE_NAMES,
  LOCALE_COOKIE,
  LOCALE_COOKIE_MAX_AGE,
  type Locale,
} from "@/lib/i18n/locale";
import { openPage, reloadPage } from "@/lib/i18n/navigate";
import { switchLocaleHref } from "@/lib/i18n/routes";
import { cn } from "@/lib/utils";

// Кнопка второго языка: «RU» на английских страницах, «EN» на русских. Выбор живёт в cookie год.
// Публичная страница открывается по парному адресу (/docs/x ↔ /ru/docs/x), панель перезагружается на
// месте: её адреса языка не содержат, язык сервер берёт из cookie.
export function LanguageSwitcher({ className }: { className?: string }) {
  const locale = useLocale();
  const m = useMessages();
  const target: Locale = locale === "en" ? "ru" : "en";
  const { name, short } = LANGUAGE_NAMES[target];

  function switchLanguage() {
    // biome-ignore lint/suspicious/noDocumentCookie: выбор языка читают proxy и сервер, cookieStore есть не везде
    document.cookie = `${LOCALE_COOKIE}=${target}; path=/; max-age=${LOCALE_COOKIE_MAX_AGE}; samesite=lax`;
    const { pathname, search, hash } = window.location;
    const href = switchLocaleHref(pathname, target);
    if (href) openPage(`${href}${search}${hash}`);
    else reloadPage();
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      lang={target}
      aria-label={fmt(m.layout.language.switchTo, { language: name })}
      className={cn("font-semibold", className)}
      onClick={switchLanguage}
    >
      {short}
    </Button>
  );
}
