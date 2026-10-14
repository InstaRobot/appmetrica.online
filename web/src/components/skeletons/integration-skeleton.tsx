import { Skeleton } from "@/components/ui/skeleton";

// Скелет S-10 (14-ux-ui-spec.md S-10, loading): заголовок, карточка ключа и карточка сниппетов.
export function IntegrationSkeleton() {
  return (
    <div
      role="status"
      aria-label="Загрузка…"
      className="flex flex-col gap-6 animate-in fade-in fill-mode-backwards delay-200"
    >
      <Skeleton className="h-6 w-40" />
      <div className="flex flex-col gap-3 rounded-xl border border-border bg-card p-6">
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-4 w-80" />
      </div>
      <Skeleton data-skeleton-code className="h-80 w-full rounded-xl" />
    </div>
  );
}
