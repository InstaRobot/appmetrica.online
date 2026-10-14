import { NextResponse } from "next/server";
import { DEMO_COOKIE } from "@/lib/demo-mode";

// Выход из демо-режима: снимает am_demo и возвращает на лендинг (Location относительный, см. ../route.ts).
export function GET() {
  const res = new NextResponse(null, { status: 307, headers: { Location: "/" } });
  res.cookies.set(DEMO_COOKIE, "", { path: "/", maxAge: 0 });
  return res;
}
