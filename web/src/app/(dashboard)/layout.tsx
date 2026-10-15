import { cookies } from "next/headers";
import { Suspense } from "react";
import { AppFooter } from "@/components/layout/app-footer";
import { AppHeader } from "@/components/layout/app-header";
import { DemoBanner } from "@/components/layout/demo-banner";
import { SkipLink } from "@/components/layout/skip-link";
import { DEMO_COOKIE } from "@/lib/demo-mode";
import { MessagesProvider } from "@/lib/i18n/client";
import { HtmlLang } from "@/lib/i18n/html-lang";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import { Providers } from "./providers";

// DashboardShell (14-ux-ui-spec.md §5): SkipLink → AppHeader → <main id="main"> → AppFooter.
// Язык (cookie → Accept-Language) и демо-режим читаются из запроса, поэтому оболочка стоит под Suspense:
// при cacheComponents статическая часть ответа — полоса шапки. AppHeader читает query-параметры отчётов
// (useSearchParams) — ему этот Suspense тоже нужен.
export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <Providers>
      <Suspense fallback={<div className="h-16 border-b border-border bg-header" />}>
        <DashboardShell>{children}</DashboardShell>
      </Suspense>
    </Providers>
  );
}

// Демо-режим определяется на сервере по cookie, чтобы шапка и плашка совпали с гидратацией.
async function DashboardShell({ children }: { children: React.ReactNode }) {
  const [locale, jar] = await Promise.all([getRequestLocale(), cookies()]);
  const demo = jar.get(DEMO_COOKIE)?.value === "1";
  return (
    <MessagesProvider locale={locale} messages={getMessages(locale)}>
      <HtmlLang locale={locale} />
      <SkipLink />
      {demo && <DemoBanner />}
      <AppHeader demo={demo} />
      <main id="main" tabIndex={-1} className="mx-auto max-w-[1600px] px-8 pt-6 pb-12 outline-none">
        {children}
      </main>
      <AppFooter />
    </MessagesProvider>
  );
}
