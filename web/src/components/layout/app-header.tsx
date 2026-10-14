"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { BookOpen, Ellipsis, LayoutGrid, LogOut, type LucideIcon, Settings } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { logout } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { toUiError } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { AppSwitcher } from "./app-switcher";
import { parseAppPath } from "./app-tabs";
import { Logo } from "./logo";

type NavItem = { href: string; label: string; icon: LucideIcon; active: (path: string) => boolean };

const within = (prefix: string) => (path: string) =>
  path === prefix || path.startsWith(`${prefix}/`);

const NAV: NavItem[] = [
  { href: "/", label: "Портфель", icon: LayoutGrid, active: (path) => path === "/" },
  { href: "/docs", label: "Документация", icon: BookOpen, active: within("/docs") },
  { href: "/settings", label: "Настройки", icon: Settings, active: within("/settings") },
];

// Шапка дашборда (14-ux-ui-spec.md §6.1): 56 px, sticky. Список приложений для AppSwitcher
// загружается на клиенте, чтобы после создания или архивации приложения обновляться без перезагрузки.
export function AppHeader() {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const current = parseAppPath(pathname);
  const { data } = useListApps();
  const apps = data?.status === 200 ? data.data.items : [];

  const signOut = useMutation({
    mutationFn: () => logout(),
    onSuccess: () => {
      queryClient.clear();
      router.replace("/login");
    },
    onError: (error) => toast.error(toUiError(error).message),
  });

  return (
    <header className="sticky top-0 z-sticky h-14 border-b border-border bg-card">
      <div className="mx-auto flex h-full max-w-[1600px] items-center gap-4 px-8">
        <Link href="/" aria-label="appmetrica.online — в портфель" className="rounded-md">
          <Logo />
        </Link>
        <AppSwitcher apps={apps} currentAppId={current?.appId} currentTab={current?.tab} />
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
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-lg" aria-label="Меню пользователя">
              <Ellipsis aria-hidden="true" className="size-5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
              <LogOut aria-hidden="true" />
              Выйти
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
