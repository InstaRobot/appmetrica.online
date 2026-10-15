"use client";

import { Check, ChevronsUpDown } from "lucide-react";
import type { Route } from "next";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandSkeleton,
} from "@/components/common/lazy-command";
import { PlatformIcons } from "@/components/common/platform-icons";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { App } from "@/lib/api/gen/model";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";
import { type AppTab, appTabHref, reportQuery } from "./app-tabs";

type AppSwitcherProps = {
  apps: App[];
  currentAppId?: string | undefined;
  currentTab?: AppTab | undefined;
};

// Переключатель приложений (14-ux-ui-spec.md §6.1): поиск по имени, выбор ведёт на ту же вкладку
// другого приложения с теми же параметрами отчёта.
export function AppSwitcher({ apps, currentAppId, currentTab = "" }: AppSwitcherProps) {
  const locale = useLocale();
  const m = useMessages().layout.appSwitcher;
  const router = useRouter();
  const query = reportQuery(useSearchParams());
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const sorted = useMemo(
    () => apps.toSorted((a, b) => a.name.localeCompare(b.name, locale)),
    [apps, locale],
  );
  const needle = search.trim().toLocaleLowerCase(locale);
  const visible = needle
    ? sorted.filter((app) => app.name.toLocaleLowerCase(locale).includes(needle))
    : sorted;
  const current = apps.find((app) => app.id === currentAppId);

  const go = (href: Route) => {
    setOpen(false);
    setSearch("");
    router.push(href);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          role="combobox"
          aria-label={m.label}
          aria-expanded={open}
          className="h-9 max-w-64 gap-2 px-2 text-body-md font-medium"
        >
          {current ? <PlatformIcons platforms={current.platforms} /> : null}
          <span className="truncate">{current ? current.name : m.allApps}</span>
          <ChevronsUpDown aria-hidden="true" className="text-fg-subtle" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-0">
        <Suspense fallback={<CommandSkeleton />}>
          <Command shouldFilter={false}>
            <CommandInput placeholder={m.search} value={search} onValueChange={setSearch} />
            <CommandList>
              <CommandEmpty>{needle ? m.notFound : m.empty}</CommandEmpty>
              {visible.length > 0 ? (
                <CommandGroup>
                  {visible.map((app) => (
                    <CommandItem
                      key={app.id}
                      value={app.id}
                      onSelect={() => go(appTabHref(app.id, currentTab, query))}
                    >
                      <PlatformIcons platforms={app.platforms} />
                      <span className="truncate">{app.name}</span>
                      <Check
                        aria-hidden="true"
                        className={cn(
                          "ml-auto",
                          app.id === currentAppId ? "opacity-100" : "opacity-0",
                        )}
                      />
                    </CommandItem>
                  ))}
                </CommandGroup>
              ) : null}
              <CommandSeparator alwaysRender />
              <CommandGroup forceMount>
                <CommandItem value="new" forceMount onSelect={() => go("/apps/new" as Route)}>
                  {m.add}
                </CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </Suspense>
      </PopoverContent>
    </Popover>
  );
}
