"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Card } from "@/components/ui/card";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";

type RealtimeCounterProps = {
  value: number;
  /** Realtime.generated_at. */
  updatedAt: string;
  /** TZ приложения для «обновлено 14:32:05». */
  timezone: string;
  paused: boolean;
  /** 3 неудачных опроса подряд. */
  stale?: boolean;
};

// Скринридер объявляет счётчик не чаще раза в 30 секунд (14-ux-ui-spec.md S-07, Accessibility).
const ANNOUNCE_EVERY_MS = 30_000;

const DOT = {
  live: "bg-success-500 motion-safe:animate-pulse",
  paused: "bg-fg-subtle",
  stale: "bg-warning-500",
} as const;

// «Активны сейчас» (14-ux-ui-spec.md §16.10): люди с открытой сессией за последние 30 минут и
// индикатор опроса — обновлено, на паузе или нет связи.
export function RealtimeCounter({
  value,
  updatedAt,
  timezone,
  paused,
  stale = false,
}: RealtimeCounterProps) {
  const m = useMessages().realtime.counter;
  const f = useFormat();
  const labelId = useId();
  const latest = useRef(value);
  const [announced, setAnnounced] = useState(value);

  useEffect(() => {
    latest.current = value;
  }, [value]);

  // В live-region попадает значение на последний 30-секундный тик, а не каждый ответ опроса.
  useEffect(() => {
    const id = setInterval(() => setAnnounced(latest.current), ANNOUNCE_EVERY_MS);
    return () => clearInterval(id);
  }, []);

  const state = paused ? "paused" : stale ? "stale" : "live";
  const status =
    state === "paused"
      ? m.paused
      : state === "stale"
        ? m.offline
        : fmt(m.updated, { time: f.formatClock(updatedAt, timezone) });

  return (
    <Card role="region" aria-labelledby={labelId} className="gap-1 px-4 md:px-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={labelId} className="text-label-sm text-fg-muted uppercase">
          {m.title}
        </h2>
        <span className="flex items-center gap-1.5 text-caption text-fg-muted">
          <span aria-hidden data-live={state} className={cn("size-2 rounded-full", DOT[state])} />
          {status}
        </span>
      </div>
      <p data-value={value} className="mt-2 text-kpi-xl">
        {f.formatInt(value)}
      </p>
      <p className="text-body-sm text-fg-muted">{m.window}</p>
      <p aria-live="polite" className="sr-only">
        {fmt(m.announce, { count: f.formatInt(announced) })}
      </p>
    </Card>
  );
}
