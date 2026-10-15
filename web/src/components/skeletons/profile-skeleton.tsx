import { Skeleton } from "@/components/ui/skeleton";
import { LoadingStatus } from "./loading-status";

function TableCard({ rows }: { rows: number }) {
  return (
    <div className="flex flex-col gap-px rounded-xl border border-border bg-card p-6">
      <Skeleton className="h-9 rounded-none bg-subtle" />
      {Array.from({ length: rows }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: строки скелета не переупорядочиваются
        <div key={i} data-skeleton-row className="flex h-11 items-center">
          <Skeleton className="h-4 w-full" />
        </div>
      ))}
    </div>
  );
}

// Скелет S-09 (14-ux-ui-spec.md S-09, loading): карточка профиля и две таблицы — устройства и сессии.
export function ProfileSkeleton() {
  return (
    <LoadingStatus className="flex flex-col gap-6 animate-in fade-in fill-mode-backwards delay-200">
      <div
        data-skeleton-card
        className="flex flex-col gap-3 rounded-xl border border-border bg-card p-6"
      >
        <Skeleton className="h-6 w-72" />
        <Skeleton className="h-4 w-96" />
        <Skeleton className="h-4 w-48" />
      </div>
      <TableCard rows={2} />
      <TableCard rows={5} />
    </LoadingStatus>
  );
}
