import type { Route } from "next";
import type { Locale } from "./locale";

// Адреса публичных страниц: английские — без префикса, русские — под /ru (proxy.ts). typedRoutes видит
// эти страницы как /[lang]/…, поэтому ссылки на них собираются здесь.
export function publicHref(locale: Locale, path: "/" | `/docs${string}`): Route {
  if (locale === "en") return path as Route;
  return (path === "/" ? "/ru" : `/ru${path}`) as Route;
}

const PUBLIC = /^\/(ru(\/|$)|ru$|docs(\/|$)|$)/;

export function isPublicPath(pathname: string): boolean {
  return pathname === "/" || pathname === "/ru" || PUBLIC.test(pathname);
}

// Парный адрес публичной страницы на другом языке; у панели адрес не меняется — null.
export function switchLocaleHref(pathname: string, target: Locale): Route | null {
  if (!isPublicPath(pathname)) return null;
  const bare =
    pathname === "/ru" ? "/" : pathname.startsWith("/ru/") ? pathname.slice(3) : pathname;
  return publicHref(target, bare as "/" | `/docs${string}`);
}
