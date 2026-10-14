import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Skeleton } from "@/components/ui/skeleton";

// Скелет портфеля (14-ux-ui-spec.md §8.2): заголовок страницы и таблица из 4 строк по 52 px.
export default function DashboardLoading() {
  return (
    <>
      <Skeleton className="mb-6 h-7 w-40" />
      <div className="rounded-xl border border-border bg-card shadow-xs">
        <TableSkeleton rows={4} rowHeight="portfolio" />
      </div>
    </>
  );
}
