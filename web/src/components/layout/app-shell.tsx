"use client";

import { SearchX } from "lucide-react";
import { useParams } from "next/navigation";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import { lazy, Suspense } from "react";
import { platformLabel } from "@/components/common/platform-icon";
import { PlatformIcons } from "@/components/common/platform-icons";
import { ArchivedBanner } from "@/components/feedback/archived-banner";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { UnitBadge } from "@/components/reports/unit-badge";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { Unit } from "@/lib/api/gen/model";
import { API_ERRORS } from "@/lib/errors";
import { AppTabs } from "./app-tabs";
import { PageHeader } from "./page-header";

// Баннер слияния с опросом Admin API — отдельный чанк: initial JS /apps/[appId] у бюджета 250 KB gz.
const MergeProgressBanner = lazy(() =>
  import("@/components/apps/merge-progress-banner").then((m) => ({
    default: m.MergeProgressBanner,
  })),
);

const unitParser = parseAsStringLiteral(Object.values(Unit)).withDefault(Unit.persons);

export function AppShellSkeleton() {
  return (
    <div role="status" aria-label="Загрузка…">
      <Skeleton className="mb-6 h-12 w-full" />
      <Skeleton className="mb-6 h-7 w-56" />
    </div>
  );
}

// AppShell (14-ux-ui-spec.md S-05…S-11): вкладки приложения, название с платформами, баннеры архива
// и слияния в это приложение.
// Приложение загружается на клиенте (useGetApp); отчёты вкладок берут его из того же кэша запросов.
export function AppShell({ children }: { children: React.ReactNode }) {
  const { appId } = useParams<{ appId: string }>();
  const [unit] = useQueryState("unit", unitParser);
  const query = useGetApp(appId);
  const app = query.data?.status === 200 ? query.data.data : undefined;

  if (query.isError && !app) {
    // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
    const error = query.error as unknown as ApiError;
    // 400 — некорректный app_id в адресе: для владельца это то же «не найдено».
    if (error.status === 404 || error.status === 400) {
      return (
        <Card className="mt-6">
          <EmptyState
            icon={SearchX}
            title={API_ERRORS.app_not_found}
            description="Проверьте адрес или выберите приложение в портфеле."
            action={{ label: "В портфель", href: "/portfolio" }}
          />
        </Card>
      );
    }
    return <ErrorState error={error} onRetry={() => void query.refetch()} />;
  }

  if (!app) return <AppShellSkeleton />;

  return (
    <>
      <AppTabs appId={appId} />
      <PageHeader
        title={app.name}
        adornment={
          <>
            <Badge variant="outline" className="gap-1">
              <PlatformIcons
                platforms={app.platforms}
                max={app.platforms.length}
                className="[&_svg]:size-3.5"
              />
              {app.platforms.map(platformLabel).join(", ")}
            </Badge>
            <UnitBadge unit={unit} />
          </>
        }
      />
      {app.status === "archived" ? (
        <div className="mb-6">
          <ArchivedBanner />
        </div>
      ) : null}
      <Suspense fallback={null}>
        <MergeProgressBanner appId={appId} />
      </Suspense>
      {children}
    </>
  );
}
