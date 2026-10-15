// Языки интерфейса: EN — по умолчанию (корень сайта), RU — /ru и выбор в cookie.
export const LOCALES = ["en", "ru"] as const;
export type Locale = (typeof LOCALES)[number];
export const DEFAULT_LOCALE: Locale = "en";

// Самоназвания языков для переключателя: не переводятся, каждый язык назван по-своему.
export const LANGUAGE_NAMES: Record<Locale, { name: string; short: string }> = {
  en: { name: "English", short: "EN" },
  ru: { name: "Русский", short: "RU" },
};

// Cookie выбранного языка: ставит переключатель и заход на /ru…, живёт год.
export const LOCALE_COOKIE = "am_lang";
export const LOCALE_COOKIE_MAX_AGE = 365 * 24 * 60 * 60;

export function isLocale(value: unknown): value is Locale {
  return value === "en" || value === "ru";
}

// Первый из поддерживаемых языков по убыванию q в Accept-Language; ни одного — null.
export function localeFromAcceptLanguage(header: string | null | undefined): Locale | null {
  if (!header) return null;
  let best: { locale: Locale; q: number } | null = null;
  for (const part of header.split(",")) {
    const [tag = "", ...params] = part.trim().split(";");
    const base = tag.trim().toLowerCase().split("-")[0];
    if (!isLocale(base)) continue;
    const qParam = params.map((p) => p.trim()).find((p) => p.startsWith("q="));
    const q = qParam ? Number(qParam.slice(2)) : 1;
    if (!(q > 0)) continue;
    if (!best || q > best.q) best = { locale: base, q };
  }
  return best?.locale ?? null;
}

// Язык панели и первого визита: сохранённый выбор → язык браузера → EN.
export function resolveLocale(input: {
  cookie?: string | undefined;
  acceptLanguage?: string | null | undefined;
}): Locale {
  if (isLocale(input.cookie)) return input.cookie;
  return localeFromAcceptLanguage(input.acceptLanguage) ?? DEFAULT_LOCALE;
}
