import { type NextRequest, NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";
import {
  LOCALE_COOKIE,
  LOCALE_COOKIE_MAX_AGE,
  LOCALES,
  type Locale,
  resolveLocale,
} from "@/lib/i18n/locale";

// Next.js 16 proxy (бывший middleware): только дешёвая проверка НАЛИЧИЯ cookie сессии.
// Валидность сессии проверяет Go API (401 unauthenticated → клиентский редирект на /login).
const SESSION_COOKIE = "am_session";

// /api/* — rewrite на Go API и собственный /api/health: API отвечает 401 сам, HTML-редирект ему не нужен.
// «/» — публичный лендинг S-00 (только сам корень, без вложенных путей); /demo и /demo/exit —
// вход в демо-режим и выход из него; /docs — публичная документация (решение владельца от 2026-10-14).
const PUBLIC_PREFIXES = ["/login", "/setup/", "/api/", "/demo", "/docs"];

function isPublic(pathname: string): boolean {
  if (pathname === "/") return true;
  return PUBLIC_PREFIXES.some((prefix) =>
    prefix.endsWith("/")
      ? pathname.startsWith(prefix)
      : pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

// Лендинг и документация — на двух языках: EN без префикса (внутри — /en/…), RU — /ru/…. Поиск по
// документации языком не маршрутизируется: Fumadocs передаёт locale в запросе.
function isLocalizedPublic(pathname: string): boolean {
  if (pathname === "/" || pathname === "/docs") return true;
  return pathname.startsWith("/docs/") && pathname !== "/docs/search";
}

function localePrefix(pathname: string): { locale: Locale; rest: string } | null {
  for (const locale of LOCALES) {
    if (pathname === `/${locale}`) return { locale, rest: "/" };
    if (pathname.startsWith(`/${locale}/`))
      return { locale, rest: pathname.slice(locale.length + 1) };
  }
  return null;
}

function saveLocale(res: NextResponse, locale: Locale): NextResponse {
  res.cookies.set(LOCALE_COOKIE, locale, {
    path: "/",
    maxAge: LOCALE_COOKIE_MAX_AGE,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
  });
  return res;
}

// Заход на /ru… сохраняет русский; /en… — английский и ведёт на адрес без префикса. Без префикса — выбор
// из cookie, при первом визите — язык браузера: русский уходит на /ru…, английский отдаётся как есть.
function routeLocale(request: NextRequest): NextResponse | null {
  const { pathname, search } = request.nextUrl;
  const prefixed = localePrefix(pathname);
  if (prefixed && isLocalizedPublic(prefixed.rest)) {
    if (prefixed.locale === "ru") return saveLocale(NextResponse.next(), "ru");
    return saveLocale(
      NextResponse.redirect(new URL(`${prefixed.rest}${search}`, request.url), 308),
      "en",
    );
  }
  if (!isLocalizedPublic(pathname)) return null;
  const locale = resolveLocale({
    cookie: request.cookies.get(LOCALE_COOKIE)?.value,
    acceptLanguage: request.headers.get("accept-language"),
  });
  const target = `/${locale}${pathname === "/" ? "" : pathname}${search}`;
  return locale === "ru"
    ? NextResponse.redirect(new URL(target, request.url), 307)
    : NextResponse.rewrite(new URL(target, request.url));
}

export function proxy(request: NextRequest): NextResponse {
  const localized = routeLocale(request);
  if (localized) return localized;

  const { pathname, search } = request.nextUrl;
  // В демо-режиме (cookie am_demo=1) дашборд открыт без сессии: данные идут из статичного датасета.
  const demo = request.cookies.get(DEMO_COOKIE)?.value === "1";
  if (isPublic(pathname) || request.cookies.get(SESSION_COOKIE)?.value || demo) {
    return NextResponse.next();
  }

  const login = new URL("/login", request.url);
  const next = `${pathname}${search}`;
  if (next !== "/") login.searchParams.set("next", next);
  return NextResponse.redirect(login, 307);
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon\\.ico|.*\\.(?:svg|png|jpg|jpeg|ico|webp|txt)$).*)",
  ],
};
