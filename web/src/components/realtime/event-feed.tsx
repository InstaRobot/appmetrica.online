"use client";

import type { Route } from "next";
import Link from "next/link";
import { useState } from "react";
import { PLATFORM_LABELS } from "@/components/common/platform-icon";
import { ScrollArea } from "@/components/ui/scroll-area";
import { usePrefersReducedMotion } from "@/hooks/use-prefers-reduced-motion";
import type { RecentEvent } from "@/lib/api/gen/model";
import { formatClock, shortId } from "@/lib/format";
import { cn } from "@/lib/utils";

type EventFeedProps = {
  events: RecentEvent[];
  timezone: string;
  /** Для ссылок на профиль. */
  appId: string;
};

const COLUMNS = [
  "Время",
  "Событие",
  "Платформа",
  "Версия",
  "Страна / город",
  "Install",
  "Человек",
] as const;

function platformLabel(platform: string): string {
  return Object.hasOwn(PLATFORM_LABELS, platform)
    ? PLATFORM_LABELS[platform as keyof typeof PLATFORM_LABELS]
    : platform || "—";
}

function place(e: RecentEvent): string {
  if (!e.country) return "—";
  return e.city ? `${e.country} · ${e.city}` : e.country;
}

// freshIds — insert_id, которых не было в предыдущем ответе; у первого ответа подсветки нет.
function freshIds(prev: RecentEvent[], next: RecentEvent[]): ReadonlySet<string> {
  const seen = new Set(prev.map((e) => e.insert_id));
  return new Set(next.filter((e) => !seen.has(e.insert_id)).map((e) => e.insert_id));
}

// Последние 50 событий (14-ux-ui-spec.md §16.10, S-07): ключ строки — insert_id, новые строки
// подсвечиваются фоном --bg-selected с затуханием; при prefers-reduced-motion подсветки нет.
// Install и Человек ведут на профиль person_key. Лента — не live-region.
export function EventFeed({ events, timezone, appId }: EventFeedProps) {
  const reducedMotion = usePrefersReducedMotion();
  const [prev, setPrev] = useState(events);
  const [fresh, setFresh] = useState<ReadonlySet<string>>(() => new Set());
  if (events !== prev) {
    setFresh(freshIds(prev, events));
    setPrev(events);
  }

  return (
    <ScrollArea className="h-[520px]">
      <table className="w-full text-body-sm">
        <caption className="sr-only">Последние события</caption>
        <thead className="sticky top-0 z-10 bg-card">
          <tr className="border-b border-border text-left text-label-sm text-fg-muted uppercase">
            {COLUMNS.map((c) => (
              <th key={c} scope="col" className="h-9 px-3 font-medium whitespace-nowrap">
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {events.map((e) => {
            const isNew = !reducedMotion && fresh.has(e.insert_id);
            const profile = `/apps/${appId}/profiles/${encodeURIComponent(e.person_key)}` as Route;
            return (
              <tr
                key={e.insert_id}
                data-new={isNew || undefined}
                className={cn(
                  "h-11 border-b border-border last:border-0",
                  isNew && "animate-row-highlight",
                )}
              >
                <td className="px-3 tabular-nums whitespace-nowrap">
                  {formatClock(e.event_time, timezone)}
                </td>
                <td className="px-3 font-mono text-code">{e.name}</td>
                <td className="px-3">{platformLabel(e.platform)}</td>
                <td className="px-3 tabular-nums">{e.app_version || "—"}</td>
                <td className="px-3">{place(e)}</td>
                <td className="px-3">
                  <Link
                    href={profile}
                    title={e.install_id}
                    aria-label={`Установка ${e.install_id}`}
                    className="font-mono text-code text-fg-link hover:underline"
                  >
                    {shortId(e.install_id)}
                  </Link>
                </td>
                <td className="px-3">
                  <Link
                    href={profile}
                    title={e.person_key}
                    aria-label={`Профиль ${e.person_key}`}
                    className="font-mono text-code text-fg-link hover:underline"
                  >
                    {shortId(e.person_key)}
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </ScrollArea>
  );
}
