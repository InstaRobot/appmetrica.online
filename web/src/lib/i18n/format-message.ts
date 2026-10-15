import type { Locale } from "./locale";

// Сообщение словаря — строка с плейсхолдерами {name} или формы плюрала по CLDR. Формы few и many нужны
// русскому («3 события», «11 событий»), английскому хватает one и other.
export type Plural = { one: string; few?: string; many?: string; other: string };

type Vars = Record<string, string | number>;

const PLACEHOLDER = /\{(\w+)\}/g;

export function fmt(message: string, vars: Vars = {}): string {
  return message.replace(PLACEHOLDER, (match, name: string) =>
    name in vars ? String(vars[name]) : match,
  );
}

const INTL: Record<Locale, string> = { en: "en-US", ru: "ru-RU" };
const rules = new Map<Locale, Intl.PluralRules>();
const numbers = new Map<Locale, Intl.NumberFormat>();

function cached<T>(map: Map<Locale, T>, locale: Locale, make: () => T): T {
  let value = map.get(locale);
  if (value === undefined) {
    value = make();
    map.set(locale, value);
  }
  return value;
}

// Форма по числу n и подстановка {count} (число в формате языка) и остальных плейсхолдеров.
export function plural(locale: Locale, forms: Plural, n: number, vars: Vars = {}): string {
  const category = cached(rules, locale, () => new Intl.PluralRules(INTL[locale])).select(n);
  const form = forms[category as keyof Plural] ?? forms.other;
  const count = cached(numbers, locale, () => new Intl.NumberFormat(INTL[locale]))
    .format(n)
    .replace(/[  ]/g, " ");
  return fmt(form, { ...vars, count });
}
