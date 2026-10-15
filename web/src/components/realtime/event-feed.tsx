"use client";

import type { Route } from "next";
import Link from "next/link";
import { useState } from "react";
import { usePlatformLabel } from "@/components/common/platform-icon";
import { ScrollArea } from "@/components/ui/scroll-area";
import { usePrefersReducedMotion } from "@/hooks/use-prefers-reduced-motion";
import type { RecentEvent } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";

type EventFeedProps = {
  events: RecentEvent[];
  timezone: string;
  /** Для ссылок на профиль. */
  appId: string;
};

const COLUMNS = ["time", "event", "platform", "version", "place", "install", "person"] as const;

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
  const platformLabel = usePlatformLabel();
  const m = useMessages().realtime.feed;
  const f = useFormat();
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
        <caption className="sr-only">{m.title}</caption>
        <thead className="sticky top-0 z-10 bg-card">
          <tr className="border-b border-border text-left text-label-sm text-fg-muted uppercase">
            {COLUMNS.map((c) => (
              <th key={c} scope="col" className="h-9 px-3 font-medium whitespace-nowrap">
                {m.columns[c]}
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
                  {f.formatClock(e.event_time, timezone)}
                </td>
                <td className="px-3 font-mono text-code">{e.name}</td>
                <td className="px-3">{platformLabel(e.platform)}</td>
                <td className="px-3 tabular-nums">{e.app_version || "—"}</td>
                <td className="px-3">{place(e)}</td>
                <td className="px-3">
                  <Link
                    href={profile}
                    title={e.install_id}
                    aria-label={fmt(m.install, { id: e.install_id })}
                    className="font-mono text-code text-fg-link hover:underline"
                  >
                    {f.shortId(e.install_id)}
                  </Link>
                </td>
                <td className="px-3">
                  <Link
                    href={profile}
                    title={e.person_key}
                    aria-label={fmt(m.profile, { id: e.person_key })}
                    className="font-mono text-code text-fg-link hover:underline"
                  >
                    {f.shortId(e.person_key)}
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
