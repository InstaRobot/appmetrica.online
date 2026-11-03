import { Skeleton } from "@/components/ui/skeleton";
import { LoadingStatus } from "./loading-status";
import { TableSkeleton } from "./table-skeleton";

// Скелет страницы события: заголовок, панель периода, три KPI, график 360 px и таблица на 6 строк.
export function EventSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      <Skeleton className="h-8 w-full max-w-[320px]" />
      <Skeleton className="h-8 w-full max-w-[640px]" />
      <LoadingStatus className="grid grid-cols-1 gap-3 animate-in fade-in fill-mode-backwards delay-200 sm:grid-cols-3 md:gap-4">
        {Array.from({ length: 3 }, (_, i) => (
          <div
            // biome-ignore lint/suspicious/noArrayIndexKey: карточки скелета не переупорядочиваются
            key={i}
            className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-xs md:p-5"
          >
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-7 w-full max-w-30" />
            <Skeleton className="h-3.5 w-15" />
          </div>
        ))}
      </LoadingStatus>
      <Skeleton className="h-90 w-full rounded-xl" />
      <div className="rounded-xl border border-border bg-card shadow-xs">
        <TableSkeleton rows={6} />
      </div>
    </div>
  );
}
