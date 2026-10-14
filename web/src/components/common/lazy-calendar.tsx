"use client";

import { type ComponentProps, lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import type { RuCalendar } from "./ru-calendar";

// react-day-picker с date-fns (≈ 30 KB gz) нужен только в открытом popover выбора даты, поэтому
// календарь грузится отдельным чанком при первом открытии (бюджет initial JS, NFR-006, T-044).
const RuCalendarChunk = lazy(() =>
  import("./ru-calendar").then((m) => ({ default: m.RuCalendar })),
);

export function LazyCalendar(props: ComponentProps<typeof RuCalendar>) {
  const months = props.numberOfMonths ?? 1;
  return (
    <Suspense
      fallback={
        <Skeleton
          role="status"
          aria-label="Загрузка календаря…"
          className="m-2 h-60"
          style={{ width: `${months * 13}rem` }}
        />
      }
    >
      <RuCalendarChunk {...props} />
    </Suspense>
  );
}
