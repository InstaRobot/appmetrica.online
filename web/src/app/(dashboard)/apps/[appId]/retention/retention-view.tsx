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
import { STRETCH_GROUP, STRETCH_ITEM, TOUCH_TOGGLE } from "@/components/reports/responsive";
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
import { useMessages } from "@/lib/i18n/client";
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
import { cn } from "@/lib/utils";

// S-06 Retention (FR-014). Приложение (TZ) берётся из кэша AppShell.
export function RetentionView() {
  const { appId } = useParams<{ appId: string }>();
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <RetentionReport app={app.data.data} /> : <RetentionSkeleton />;
}

function RetentionReport({ app }: { app: App }) {
  const messages = useMessages();
  const errorTexts = messages.errors.codes;
  const m = messages.retention;
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
  const rangeError = error?.code === "range_too_large" ? errorTexts.range_too_large : undefined;
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
        <EmptyState icon={Users} title={m.empty.title} description={m.empty.description} />
      </Card>
    );
  } else if (matrix) {
    content = (
      <>
        <Card className="gap-4 px-4 md:px-6">
          <h2 className="text-heading-3">{m.average.title}</h2>
          <RetentionAverageChart average={matrix.average} granularity={state.cohort} />
        </Card>
        <Card className="gap-4 px-4 md:px-6">
          <h2 className="text-heading-3">{m.cohorts}</h2>
          <RetentionHeatmap matrix={matrix} granularity={state.cohort} unit={state.unit} />
        </Card>
      </>
    );
  } else if (retention.isPending) {
    content = <RetentionSkeleton />;
  }

  // Панель отчёта: на телефоне когорты, период, платформа и окружение — каждый на всю ширину, фильтры
  // переносятся строкой; с md — одной строкой, как на десктопе.
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start gap-3">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={0}
          aria-label={m.cohorts}
          className={STRETCH_GROUP}
          value={state.cohort}
          onValueChange={(next) => {
            const cohort = COHORT_GRANULARITIES.find((c) => c === next);
            if (cohort) void update({ cohort });
          }}
        >
          {COHORT_GRANULARITIES.map((c) => (
            <ToggleGroupItem key={c} value={c} className={cn(STRETCH_ITEM, TOUCH_TOGGLE, "px-3")}>
              {m.cohortOptions[c]}
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
        <div className="w-full md:ml-auto md:w-auto">
          <EnvironmentSwitch value={state.env} onChange={(env) => void update({ env })} />
        </div>
      </div>
      {content}
    </div>
  );
}
