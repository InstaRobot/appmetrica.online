import { KpiSkeleton } from "@/components/skeletons/kpi-skeleton";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Skeleton } from "@/components/ui/skeleton";

// Скелет S-05 (14-ux-ui-spec.md S-05, loading): панель периода, 4 KPI по 132 px, график 360 px, таблица 6 строк.
export default function AudienceLoading() {
  return (
    <div className="flex flex-col gap-6">
      <Skeleton className="h-8 w-[640px]" />
      <KpiSkeleton />
      <Skeleton className="h-90 w-full rounded-xl" />
      <div className="rounded-xl border border-border bg-card shadow-xs">
        <TableSkeleton rows={6} />
      </div>
    </div>
  );
}
