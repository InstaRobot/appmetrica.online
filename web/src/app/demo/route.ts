import { type NextRequest, NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";

// Вход в демо-режим (кнопка «Посмотреть демо-данные» на лендинге): ставит am_demo=1 и открывает портфель.
export function GET(request: NextRequest) {
  const res = NextResponse.redirect(new URL("/portfolio", request.url), 307);
  res.cookies.set(DEMO_COOKIE, "1", {
    path: "/",
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
  });
  return res;
}
