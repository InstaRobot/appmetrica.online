import { Skeleton } from "@/components/ui/skeleton";
import { LoadingStatus } from "./loading-status";

const FEED_ROWS = 8;

// Скелет S-07 (14-ux-ui-spec.md S-07, loading): счётчик 56 px, график 200 px и 8 строк ленты.
// На телефоне счётчик над графиком, как в RealtimePanel.
export function RealtimeSkeleton() {
  return (
    <LoadingStatus className="flex flex-col gap-6 animate-in fade-in fill-mode-backwards delay-200">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-[320px_1fr]">
        <div className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 md:p-6">
          <Skeleton className="h-4 w-32" />
          <Skeleton data-skeleton-counter className="h-14 w-40" />
          <Skeleton className="h-4 w-36" />
        </div>
        <Skeleton className="h-62 w-full rounded-xl" />
      </div>
      <div className="flex flex-col gap-px rounded-xl border border-border bg-card p-4">
        <Skeleton className="h-9 rounded-none bg-subtle" />
        {Array.from({ length: FEED_ROWS }, (_, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: строки скелета не переупорядочиваются
          <div key={i} data-skeleton-row className="flex h-11 items-center">
            <Skeleton className="h-4 w-full" />
          </div>
        ))}
      </div>
    </LoadingStatus>
  );
}
