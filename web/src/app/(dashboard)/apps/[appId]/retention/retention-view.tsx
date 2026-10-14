"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { Users } from "lucide-react";
import { useParams } from "next/navigation";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { EnvironmentSwitch } from "@/components/reports/environment-switch";
import { FilterBar } from "@/components/reports/filter-bar";
import { RetentionAverageChart } from "@/components/reports/lazy-charts";
import { PeriodPicker } from "@/components/reports/period-picker";
import { PlatformSwitcher } from "@/components/reports/platform-switcher";
import { RetentionHeatmap } from "@/components/reports/retention-heatmap";
import { RetentionSkeleton } from "@/components/skeletons/retention-skeleton";
import { Card } from "@/components/ui/card";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useFilterOptions } from "@/hooks/use-filter-options";
import { useObservedPlatforms } from "@/hooks/use-observed-platforms";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetRetention } from "@/lib/api/gen/endpoints/retention/retention";
import type { App } from "@/lib/api/gen/model";
import { API_ERRORS } from "@/lib/errors";
import { todayIn } from "@/lib/period";
import {
  COHORT_GRANULARITIES,
  filtersSearch,
  periodSearch,
  REPORT_FILTER_DIMENSIONS,
  RETENTION_PRESETS,
  reportParams,
  retentionParams,
  useRetentionState,
} from "@/lib/url-state";

const COHORT_LABELS = { day: "по дням", week: "по неделям" } as const;

// S-06 Retention (FR-014). Приложение (TZ) берётся из кэша AppShell.
export function RetentionView() {
  const { appId } = useParams<{ appId: string }>();
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <RetentionReport app={app.data.data} /> : <RetentionSkeleton />;
}

function RetentionReport({ app }: { app: App }) {
  const today = todayIn(app.timezone);
  const { state, update } = useRetentionState(app.timezone, today);
  const retention = useGetRetention(app.id, retentionParams(state), {
    query: { placeholderData: keepPreviousData },
  });
  const params = reportParams(state);
  const filterOptions = useFilterOptions(app.id, params, REPORT_FILTER_DIMENSIONS);
  const observedPlatforms = useObservedPlatforms(app.id, params);

  // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
  const error = retention.isError ? (retention.error as unknown as ApiError | null) : null;
  const rangeError = error?.code === "range_too_large" ? API_ERRORS.range_too_large : undefined;
  const matrix = retention.data?.status === 200 ? retention.data.data : undefined;

  let content: React.ReactNode = null;
  if (error && !rangeError) {
    content = (
      <Card>
        <ErrorState error={error} onRetry={() => void retention.refetch()} />
      </Card>
    );
  } else if (matrix?.cohorts.every((c) => c.size === 0)) {
    content = (
      <Card>
        <EmptyState
          icon={Users}
          title="Нет новых пользователей в выбранных когортах"
          description="Retention считается по людям, чья первая сессия пришлась на период."
        />
      </Card>
    );
  } else if (matrix) {
    content = (
      <>
        <Card className="gap-4 px-6">
          <h2 className="text-heading-3">Средняя кривая удержания</h2>
          <RetentionAverageChart average={matrix.average} granularity={state.cohort} />
        </Card>
        <Card className="gap-4 px-6">
          <h2 className="text-heading-3">Когорты</h2>
          <RetentionHeatmap matrix={matrix} granularity={state.cohort} unit={state.unit} />
        </Card>
      </>
    );
  } else if (retention.isPending) {
    content = <RetentionSkeleton />;
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start gap-3">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={0}
          aria-label="Когорты"
          value={state.cohort}
          onValueChange={(next) => {
            const cohort = COHORT_GRANULARITIES.find((c) => c === next);
            if (cohort) void update({ cohort });
          }}
        >
          {COHORT_GRANULARITIES.map((c) => (
            <ToggleGroupItem key={c} value={c} className="px-3">
              {COHORT_LABELS[c]}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <PeriodPicker
          value={state.period}
          onChange={(period) => void update(periodSearch(period))}
          timezone={app.timezone}
          presets={RETENTION_PRESETS}
          error={rangeError}
        />
        <PlatformSwitcher
          appPlatforms={app.platforms}
          observed={observedPlatforms}
          value={state.filters.platform?.[0] ?? null}
          onChange={(platform) => void update({ platform: platform ? [platform] : null })}
        />
        <FilterBar
          dims={REPORT_FILTER_DIMENSIONS}
          value={state.filters}
          onChange={(filters) => void update(filtersSearch(filters))}
          options={filterOptions.options}
          onOpenChange={filterOptions.onOpenChange}
          loading={filterOptions.loading}
        />
        <div className="ml-auto">
          <EnvironmentSwitch value={state.env} onChange={(env) => void update({ env })} />
        </div>
      </div>
      {content}
    </div>
  );
}
