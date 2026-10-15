import { Skeleton } from "@/components/ui/skeleton";
import { LoadingStatus } from "./loading-status";

const ROWS = 10;
const COLUMNS = 12;

// Скелет S-06 (14-ux-ui-spec.md §8.2): график 220 px и сетка когорт 10 × 12 ячеек по 56 × 32.
export function RetentionSkeleton() {
  return (
    <LoadingStatus className="flex flex-col gap-6 animate-in fade-in fill-mode-backwards delay-200">
      <Skeleton className="h-55 w-full rounded-xl" />
      <div className="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
        {Array.from({ length: ROWS }, (_, row) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: строки скелета не переупорядочиваются
          <div key={row} className="flex gap-1">
            {Array.from({ length: COLUMNS }, (_, col) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: ячейки скелета не переупорядочиваются
              <Skeleton key={col} data-skeleton-cell className="h-8 w-14 rounded-sm" />
            ))}
          </div>
        ))}
      </div>
    </LoadingStatus>
  );
}
