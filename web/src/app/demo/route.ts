import { NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";

// Вход в демо-режим (кнопка «Посмотреть демо-данные» на лендинге): ставит am_demo=1 и открывает портфель.
// Location относительный: за обратным прокси request.url standalone-сервера — внутренний 0.0.0.0:3000.
export function GET() {
  const res = new NextResponse(null, { status: 307, headers: { Location: "/portfolio" } });
  res.cookies.set(DEMO_COOKIE, "1", {
    path: "/",
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
  });
  return res;
}
