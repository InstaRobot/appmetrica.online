import { type NextRequest, NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";

// Выход из демо-режима: снимает am_demo и возвращает на лендинг.
export function GET(request: NextRequest) {
  const res = NextResponse.redirect(new URL("/", request.url), 307);
  res.cookies.set(DEMO_COOKIE, "", { path: "/", maxAge: 0 });
  return res;
}
