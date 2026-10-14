import { Skeleton } from "@/components/ui/skeleton";

type TableSkeletonProps = { rows?: number; rowHeight?: "default" | "portfolio" };

// Скелет таблицы (14-ux-ui-spec.md §8.2): шапка и `rows` строк по 44 px (в портфеле 52 px).
// Появляется с задержкой 200 мс, чтобы быстрые ответы не мигали.
export function TableSkeleton({ rows = 5, rowHeight = "default" }: TableSkeletonProps) {
  return (
    <div
      role="status"
      aria-label="Загрузка…"
      className="flex flex-col gap-px animate-in fade-in fill-mode-backwards delay-200"
    >
      <Skeleton className="h-9 rounded-none bg-subtle" />
      {Array.from({ length: rows }, (_, i) => (
        <div
          // biome-ignore lint/suspicious/noArrayIndexKey: строки скелета не переупорядочиваются
          key={i}
          data-skeleton-row
          className={
            rowHeight === "portfolio" ? "flex h-13 items-center" : "flex h-11 items-center"
          }
        >
          <Skeleton className="h-4 w-full" />
        </div>
      ))}
    </div>
  );
}
