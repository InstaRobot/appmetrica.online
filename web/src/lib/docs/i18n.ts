import { defineI18n } from "fumadocs-core/i18n";
import { DEFAULT_LOCALE, LOCALES } from "@/lib/i18n/locale";

// Документация на двух языках: page.mdx — английский, page.ru.mdx — русский (parser "dot"). Английские
// адреса без префикса (/docs/…), русские — /ru/docs/…; маршрутизирует proxy.ts. Страница без русского
// перевода (справочник API из OpenAPI) показывается по-английски.
export const i18n = defineI18n({
  languages: [...LOCALES],
  defaultLanguage: DEFAULT_LOCALE,
  hideLocale: "default-locale",
  parser: "dot",
});
