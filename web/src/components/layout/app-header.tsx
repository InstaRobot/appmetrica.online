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
import { cn } from "@/lib/utils";
import { parseAppPath } from "./app-tabs";
import { Logo } from "./logo";
import { ThemeToggle } from "./theme-toggle";

// AppSwitcher (Radix Popover) и меню пользователя (Radix DropdownMenu) — отдельные чанки: их HTML
// приходит с сервера, а код догружается сразу после первой загрузки страницы (бюджет initial JS
// NFR-006, T-044). Заглушки того же размера видны только при монтировании на клиенте.
const AppSwitcher = lazy(() => import("./app-switcher").then((m) => ({ default: m.AppSwitcher })));
const UserMenu = lazy(() => import("./user-menu").then((m) => ({ default: m.UserMenu })));

type NavItem = { href: string; label: string; icon: LucideIcon; active: (path: string) => boolean };

const within = (prefix: string) => (path: string) =>
  path === prefix || path.startsWith(`${prefix}/`);

const NAV: NavItem[] = [
  { href: "/portfolio", label: "Портфель", icon: LayoutGrid, active: within("/portfolio") },
  { href: "/docs", label: "Документация", icon: BookOpen, active: within("/docs") },
  { href: "/settings", label: "Настройки", icon: Settings, active: within("/settings") },
];

// Шапка дашборда (14-ux-ui-spec.md §6.1, макет «Dashboard v2»): 64 px, sticky, полупрозрачная с размытием. Список приложений для AppSwitcher
// загружается на клиенте, чтобы после создания или архивации приложения обновляться без перезагрузки.
// demo — режим статичного датасета (lib/demo-mode.ts): меню предлагает выйти из демо вместо выхода
// из аккаунта.
export function AppHeader({ demo = false }: { demo?: boolean }) {
  const pathname = usePathname();
  const current = parseAppPath(pathname);
  const { data } = useListApps();
  const apps = data?.status === 200 ? data.data.items : [];

  return (
    <header className="sticky top-0 z-sticky h-16 border-b border-border bg-header backdrop-blur-md">
      <div className="mx-auto flex h-full max-w-[1600px] items-center gap-4 px-8">
        <Link href="/portfolio" aria-label="appmetrica.online — в портфель" className="rounded-md">
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
        <nav aria-label="Основная навигация" className="ml-auto">
          <ul className="flex items-center gap-1">
            {NAV.map(({ href, label, icon: Icon, active }) => {
              const isActive = active(pathname);
              return (
                <li key={href}>
                  <Link
                    href={href as Route}
                    aria-current={isActive ? "page" : undefined}
                    className={cn(
                      "flex h-9 items-center gap-2 rounded-md px-3 text-label-md text-fg-muted transition-colors hover:bg-subtle hover:text-foreground",
                      isActive && "bg-selected text-fg-link hover:bg-selected hover:text-fg-link",
                    )}
                  >
                    <Icon aria-hidden="true" className="size-5" />
                    {label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
        <ThemeToggle />
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
    </header>
  );
}
