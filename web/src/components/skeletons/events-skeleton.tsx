import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Skeleton } from "@/components/ui/skeleton";

// Скелет вкладки «События»: панель периода и таблица списка событий на 6 строк.
export function EventsSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      <Skeleton className="h-8 w-full max-w-[640px]" />
      <div className="rounded-xl border border-border bg-card shadow-xs">
        <TableSkeleton rows={6} />
      </div>
    </div>
  );
}
