"use client";

import { type ComponentProps, lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { useMessages } from "@/lib/i18n/client";
import type { LocalizedCalendar } from "./localized-calendar";

// react-day-picker с date-fns (≈ 30 KB gz) нужен только в открытом popover выбора даты, поэтому
// календарь грузится отдельным чанком при первом открытии (бюджет initial JS, NFR-006, T-044).
const CalendarChunk = lazy(() =>
  import("./localized-calendar").then((m) => ({ default: m.LocalizedCalendar })),
);

export function LazyCalendar(props: ComponentProps<typeof LocalizedCalendar>) {
  const label = useMessages().common.loadingCalendar;
  const months = props.numberOfMonths ?? 1;
  return (
    <Suspense
      fallback={
        <Skeleton
          role="status"
          aria-label={label}
          className="m-2 h-60"
          style={{ width: `${months * 13}rem` }}
        />
      }
    >
      <CalendarChunk {...props} />
    </Suspense>
  );
}
