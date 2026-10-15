import { useSyncExternalStore } from "react";
import type { Locale } from "@/lib/i18n/locale";

// Язык страницы для компонентов вне MessagesProvider (корневые границы ошибок): <html lang> ставит
// inline-скрипт до первой отрисовки (lib/i18n/lang-script.ts), HtmlLang уточняет его после гидратации.
// Модуль lib/i18n/locale не импортируется: обе границы ошибок — отдельные чанки initial JS, и каждая
// получила бы свою копию.
function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["lang"] });
  return () => observer.disconnect();
}

// Любой язык, кроме русского, — язык по умолчанию (английский).
const readLocale = (): Locale => (document.documentElement.lang === "ru" ? "ru" : "en");
const serverLocale = (): Locale => "en";

export function useDocumentLocale(): Locale {
  return useSyncExternalStore(subscribe, readLocale, serverLocale);
}
