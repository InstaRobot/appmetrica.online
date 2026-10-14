"use client";

import {
  type LucideIcon,
  PlugZap,
  Radio,
  Repeat,
  SlidersHorizontal,
  UserSearch,
  Users,
} from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { cn } from "@/lib/utils";

export type AppTab = "" | "retention" | "realtime" | "profiles" | "integration" | "settings";

const TABS: { tab: AppTab; label: string; icon: LucideIcon }[] = [
  { tab: "", label: "Аудитория", icon: Users },
  { tab: "retention", label: "Retention", icon: Repeat },
  { tab: "realtime", label: "Realtime", icon: Radio },
  { tab: "profiles", label: "Профили", icon: UserSearch },
  { tab: "integration", label: "Интеграция", icon: PlugZap },
  { tab: "settings", label: "Настройки", icon: SlidersHorizontal },
];

// Параметры отчётов, которые переживают переход между вкладками и приложениями (14-ux-ui-spec.md §6.1).
const REPORT_PARAMS = new Set([
  "period",
  "from",
  "to",
  "env",
  "platform",
  "app_version",
  "os_name",
  "country",
  "unit",
]);

export function reportQuery(search: URLSearchParams): string {
  const kept = new URLSearchParams();
  for (const [key, value] of search) {
    if (REPORT_PARAMS.has(key)) kept.append(key, value);
  }
  const query = kept.toString();
  return query ? `?${query}` : "";
}

// parseAppPath разбирает `/apps/<appId>[/<tab>[/…]]`; `/apps/new` — не приложение.
export function parseAppPath(pathname: string): { appId: string; tab: AppTab } | null {
  const [, root, appId, tab = ""] = pathname.split("/");
  if (root !== "apps" || !appId || appId === "new") return null;
  return { appId, tab: TABS.some((t) => t.tab === tab) ? (tab as AppTab) : "" };
}

export function appTabHref(appId: string, tab: AppTab, query = ""): Route {
  return `/apps/${appId}${tab ? `/${tab}` : ""}${query}` as Route;
}

type AppTabsProps = { appId: string };

// Вкладки приложения (AppShell): активная вкладка — aria-current="page" и синяя нижняя граница.
export function AppTabs({ appId }: AppTabsProps) {
  const pathname = usePathname();
  const query = reportQuery(useSearchParams());
  const active = parseAppPath(pathname)?.tab;

  return (
    <nav aria-label="Разделы приложения" className="mb-6 h-12 border-b border-border">
      <ul className="flex h-full items-stretch gap-6">
        {TABS.map(({ tab, label, icon: Icon }) => {
          const current = tab === active;
          return (
            <li key={tab} className="flex">
              <Link
                href={appTabHref(appId, tab, query)}
                aria-current={current ? "page" : undefined}
                className={cn(
                  "-mb-px flex items-center gap-2 border-b-2 border-transparent text-label-md text-fg-muted transition-colors hover:text-foreground",
                  current && "border-primary text-fg-link hover:text-fg-link",
                )}
              >
                <Icon aria-hidden="true" className="size-4" />
                {label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
