"use client";

import {
  BookOpen,
  ChevronsUpDown,
  Ellipsis,
  LayoutGrid,
  type LucideIcon,
  Settings,
} from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { lazy, Suspense } from "react";
import { buttonVariants } from "@/components/ui/button";
import { useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { publicHref } from "@/lib/i18n/routes";
import { cn } from "@/lib/utils";
import type { Messages } from "@/messages";
import { parseAppPath } from "./app-tabs";
import { LanguageSwitcher } from "./language-switcher";
import { Logo } from "./logo";
import { MobileNav } from "./mobile-nav";
import { ThemeToggle } from "./theme-toggle";

// AppSwitcher (Radix Popover) и меню пользователя (Radix DropdownMenu) — отдельные чанки: их HTML
// приходит с сервера, а код догружается сразу после первой загрузки страницы (бюджет initial JS
// NFR-006, T-044). Заглушки того же размера видны только при монтировании на клиенте.
const AppSwitcher = lazy(() => import("./app-switcher").then((m) => ({ default: m.AppSwitcher })));
const UserMenu = lazy(() => import("./user-menu").then((m) => ({ default: m.UserMenu })));

type NavItem = { href: Route; label: keyof Messages["layout"]["header"]; icon: LucideIcon };

const within = (prefix: string, path: string) => path === prefix || path.startsWith(`${prefix}/`);

// Шапка дашборда (14-ux-ui-spec.md §6.1, макет «Dashboard v2»): 64 px, sticky, полупрозрачная с размытием. Список приложений для AppSwitcher
// загружается на клиенте, чтобы после создания или архивации приложения обновляться без перезагрузки.
// demo — режим статичного датасета (lib/demo-mode.ts): меню предлагает выйти из демо вместо выхода
// из аккаунта.
export function AppHeader({ demo = false }: { demo?: boolean }) {
  const locale = useLocale();
  const m = useMessages().layout.header;
  const pathname = usePathname();
  const current = parseAppPath(pathname);
  const { data } = useListApps();
  const apps = data?.status === 200 ? data.data.items : [];
  // Документация публичная и живёт по адресу языка: /docs или /ru/docs.
  const nav: NavItem[] = [
    { href: "/portfolio", label: "portfolio", icon: LayoutGrid },
    { href: publicHref(locale, "/docs"), label: "docs", icon: BookOpen },
    { href: "/settings", label: "settings", icon: Settings },
  ];

  return (
    <header className="sticky top-0 z-sticky h-16 border-b border-border bg-header backdrop-blur-md">
      <div className="mx-auto flex h-full max-w-[1600px] items-center gap-2 px-4 md:gap-4 md:px-8">
        <Link href="/portfolio" aria-label={m.home} className="rounded-md">
          <Logo />
        </Link>
        <Suspense
          fallback={
            <span
              aria-hidden="true"
              className={cn(buttonVariants({ variant: "ghost" }), "h-9 gap-2 px-2 text-body-md")}
            >
              <ChevronsUpDown className="text-fg-subtle" />
            </span>
          }
        >
          <AppSwitcher apps={apps} currentAppId={current?.appId} currentTab={current?.tab} />
        </Suspense>
        <nav aria-label={m.nav} className="ml-auto hidden md:block">
          <ul className="flex items-center gap-1">
            {nav.map(({ href, label, icon: Icon }) => {
              const isActive = within(href, pathname);
              return (
                <li key={label}>
                  <Link
                    href={href}
                    aria-current={isActive ? "page" : undefined}
                    className={cn(
                      "flex h-9 items-center gap-2 rounded-md px-3 text-label-md text-fg-muted transition-colors hover:bg-subtle hover:text-foreground",
                      isActive && "bg-selected text-fg-link hover:bg-selected hover:text-fg-link",
                    )}
                  >
                    <Icon aria-hidden="true" className="size-5" />
                    {m[label]}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
        <MobileNav
          variant="panel"
          demo={demo}
          className="ml-auto"
          items={nav.map(({ href, label }) => ({
            href,
            label: m[label],
            active: within(href, pathname),
          }))}
        />
        <LanguageSwitcher className="hidden md:inline-flex" />
        <ThemeToggle className="hidden md:inline-flex" />
        <div className="hidden md:contents">
          <Suspense
            fallback={
              <span
                aria-hidden="true"
                className={buttonVariants({ variant: "ghost", size: "icon-lg" })}
              >
                <Ellipsis className="size-5" />
              </span>
            }
          >
            <UserMenu demo={demo} />
          </Suspense>
        </div>
      </div>
    </header>
  );
}
