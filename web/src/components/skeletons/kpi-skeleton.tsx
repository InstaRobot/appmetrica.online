import { Skeleton } from "@/components/ui/skeleton";
import { LoadingStatus } from "./loading-status";

type KpiSkeletonProps = { count?: number };

// Скелет ряда KPI-карточек (14-ux-ui-spec.md §8.2): заголовок 80×12, значение 120×28, дельта 60×14,
// спарклайн 100 % × 32. Появляется с задержкой 200 мс.
export function KpiSkeleton({ count = 4 }: KpiSkeletonProps) {
  return (
    <LoadingStatus className="grid grid-cols-4 gap-4 animate-in fade-in fill-mode-backwards delay-200">
      {Array.from({ length: count }, (_, i) => (
        <div
          // biome-ignore lint/suspicious/noArrayIndexKey: карточки скелета не переупорядочиваются
          key={i}
          data-skeleton-kpi
          className="flex flex-col gap-3 rounded-xl border border-border bg-card p-5 shadow-xs"
        >
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-7 w-30" />
          <Skeleton className="h-3.5 w-15" />
          <Skeleton className="h-8 w-full" />
        </div>
      ))}
    </LoadingStatus>
  );
}
