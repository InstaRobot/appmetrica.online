"use client";

import Link from "next/link";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { publicHref } from "@/lib/i18n/routes";
import { cn } from "@/lib/utils";
import { LanguageSwitcher } from "./language-switcher";
import { Logo } from "./logo";
import { MobileNav } from "./mobile-nav";
import { ThemeToggle } from "./theme-toggle";

// Шапка публичных страниц: лендинг S-00 и документация для гостя. Без запросов к API — шапка
// дашборда грузит список приложений, и 401 увёл бы гостя на вход.
export function PublicHeader({ className }: { className?: string }) {
  const locale = useLocale();
  const m = useMessages().layout.publicHeader;
  return (
    <header className={cn("flex h-16 w-full items-center gap-2 px-4 md:gap-4 md:px-8", className)}>
      <Link href={publicHref(locale, "/")} aria-label={m.home} className="rounded-md">
        <Logo />
      </Link>
      <div className="ml-auto flex items-center gap-2">
        <Link
          href={publicHref(locale, "/docs")}
          className="inline-flex h-9 items-center rounded-lg max-md:hidden px-3 text-sm font-semibold text-fg-muted transition-colors outline-none hover:bg-subtle hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          {m.docs}
        </Link>
        <LanguageSwitcher className="max-md:hidden" />
        <ThemeToggle className="border border-(--border-strong) bg-card max-md:hidden" />
        <Link
          href="/login"
          className="inline-flex h-9 items-center rounded-lg bg-linear-to-b from-(--primary-top) to-primary px-4 text-sm font-semibold text-primary-foreground shadow-(--shadow-primary) transition-colors outline-none hover:from-(--primary-hover) hover:to-(--primary-hover) focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          {m.signIn}
        </Link>
        <MobileNav
          variant="public"
          className="md:hidden"
          items={[{ href: publicHref(locale, "/docs"), label: m.docs }]}
        />
      </div>
    </header>
  );
}
