import { type NextRequest, NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";

// Next.js 16 proxy (бывший middleware): только дешёвая проверка НАЛИЧИЯ cookie сессии.
// Валидность сессии проверяет Go API (401 unauthenticated → клиентский редирект на /login).
const SESSION_COOKIE = "am_session";

// /api/* — rewrite на Go API и собственный /api/health: API отвечает 401 сам, HTML-редирект ему не нужен.
// «/» — публичный лендинг S-00 (только сам корень, без вложенных путей); /demo и /demo/exit —
// вход в демо-режим и выход из него.
const PUBLIC_PREFIXES = ["/login", "/setup/", "/api/", "/demo"];

// В демо-режиме (cookie am_demo=1) дашборд открыт без сессии: данные идут из статичного датасета,
// а не из API. Документация остаётся только для владельца.
const isDocs = (pathname: string) => pathname === "/docs" || pathname.startsWith("/docs/");

function isPublic(pathname: string): boolean {
  if (pathname === "/") return true;
  return PUBLIC_PREFIXES.some((prefix) =>
    prefix.endsWith("/")
      ? pathname.startsWith(prefix)
      : pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

export function proxy(request: NextRequest): NextResponse {
  const { pathname, search } = request.nextUrl;
  const demo = request.cookies.get(DEMO_COOKIE)?.value === "1" && !isDocs(pathname);
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
