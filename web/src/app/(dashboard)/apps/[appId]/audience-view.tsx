"use client";

import { keepPreviousData, useQueries } from "@tanstack/react-query";
import { Copy, PlugZap } from "lucide-react";
import type { Route } from "next";
import { useParams } from "next/navigation";
import { lazy, Suspense } from "react";
import { usePlatformLabel } from "@/components/common/platform-icon";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { AudienceKpis } from "@/components/reports/audience-kpis";
import { CompareToggle } from "@/components/reports/compare-toggle";
import { EnvironmentSwitch } from "@/components/reports/environment-switch";
import { FilterBar } from "@/components/reports/filter-bar";
import { GranularitySelect } from "@/components/reports/granularity-select";
import { MetricChart, RollingActiveChart } from "@/components/reports/lazy-charts";
import type { MetricChartSeries } from "@/components/reports/metric-chart";
import { PeriodPicker } from "@/components/reports/period-picker";
import { PlatformSwitcher, platformOptions } from "@/components/reports/platform-switcher";
import {
  STRETCH_GROUP,
  STRETCH_ITEM,
  TOUCH_BUTTON,
  TOUCH_TOGGLE,
} from "@/components/reports/responsive";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useFilterOptions } from "@/hooks/use-filter-options";
import { useObservedPlatforms } from "@/hooks/use-observed-platforms";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import {
  getGetAudienceTimeseriesQueryOptions,
  useGetAudienceBreakdown,
  useGetAudienceSummary,
  useGetAudienceTimeseries,
  useGetRollingActive,
} from "@/lib/api/gen/endpoints/audience/audience";
import { useGetIntegrationHealth } from "@/lib/api/gen/endpoints/integration/integration";
import {
  type App,
  Platform,
  type TimeSeries,
  TimeSeriesMetric,
  type TimeSeriesPoint,
} from "@/lib/api/gen/model";
import { audienceCurl } from "@/lib/copy-as-curl";
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { useMetricDefinitions } from "@/lib/metric-definitions";
import {
  addDays,
  allowedGranularities,
  coerceGranularity,
  type PeriodValue,
  todayIn,
} from "@/lib/period";
import { toast } from "@/lib/toast";
import {
  type ChartMetric,
  FILTER_DIMENSIONS,
  type Filters,
  filtersSearch,
  periodSearch,
  REPORT_FILTER_DIMENSIONS,
  reportParams,
  useAudienceState,
} from "@/lib/url-state";
import { cn } from "@/lib/utils";
import AudienceLoading from "./loading";

// Разбивка — последняя карточка страницы: таблица с Radix Select грузится отдельным чанком
// параллельно с данными (бюджет initial JS NFR-006, T-044).
const BreakdownTable = lazy(() =>
  import("@/components/reports/breakdown-table").then((m) => ({ default: m.BreakdownTable })),
);

const METRICS = Object.values(TimeSeriesMetric);
const keep = { query: { placeholderData: keepPreviousData } };

// Данные за последние 8 дней ещё дополняются офлайн-очередями (окно позднего прихода 7 дней).
const OPEN_DAYS = 8;

// apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
const apiError = (q: { error: unknown }) => q.error as ApiError | null;

const isRangeError = (q: { error: unknown }) => apiError(q)?.code === "range_too_large";

// Серии графика: итог с рядом предыдущего периода или линия на каждую платформу из splits. Без splits (нет
// событий за период) — итоговый ряд, чтобы график показал период и «Нет событий».
function chartSeries(
  data: TimeSeries,
  split: boolean,
  totalLabel: string,
  platformLabel: (platform: string) => string,
): MetricChartSeries[] {
  if (split && data.splits && data.splits.length > 0) {
    return data.splits.map((s) => ({
      key: s.platform,
      label: platformLabel(s.platform),
      points: s.points,
    }));
  }
  return [
    {
      key: "total",
      label: totalLabel,
      points: data.points,
      ...(data.compare_points ? { compare: data.compare_points } : {}),
    },
  ];
}

// Ошибка карточки: range_too_large показывается один раз под выбором периода.
function cardError(q: { isError: boolean; error: unknown; refetch: () => unknown }) {
  const error = apiError(q);
  if (!q.isError || !error || isRangeError(q)) return undefined;
  return <ErrorState error={error} onRetry={() => void q.refetch()} />;
}

// S-05 Аудитория (FR-013). Приложение (TZ) берётся из кэша AppShell.
export function AudienceView() {
  const { appId } = useParams<{ appId: string }>();
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <AudienceReport app={app.data.data} /> : <AudienceLoading />;
}

function AudienceReport({ app }: { app: App }) {
  const platformLabel = usePlatformLabel();
  const messages = useMessages();
  const errorTexts = messages.errors.codes;
  const m = messages.reports;
  const metrics = useMetricDefinitions();
  const today = todayIn(app.timezone);
  const { state, update } = useAudienceState(app.timezone, today);
  const params = reportParams(state);
  const observedPlatforms = useObservedPlatforms(app.id, params);
  const platform = state.filters.platform?.[0] ?? null;
  // «По платформам» — только при «Все» и 2+ платформах переключателя.
  const canSplit =
    platform === null && platformOptions(app.platforms, observedPlatforms, null).length > 1;
  const split = canSplit && state.split;

  // Каждая карточка грузится своим запросом и не ждёт остальные.
  const summary = useGetAudienceSummary(app.id, { ...params, compare: "previous_period" }, keep);
  const chart = useGetAudienceTimeseries(
    app.id,
    {
      ...params,
      granularity: state.granularity,
      metric: state.metric,
      ...(split
        ? { split: "platform", compare: "none" }
        : { compare: state.compare ? "previous_period" : "none" }),
    },
    keep,
  );
  // Спарклайны KPI — дневные ряды всех метрик; ряд текущей метрики делит кэш с графиком.
  const sparklineQueries = useQueries({
    queries: METRICS.map((metric) => ({
      ...getGetAudienceTimeseriesQueryOptions(app.id, {
        ...params,
        granularity: "day",
        metric,
        compare: "none",
      }),
      placeholderData: keepPreviousData,
    })),
  });
  const rolling = useGetRollingActive(app.id, params, keep);
  const breakdown = useGetAudienceBreakdown(
    app.id,
    { ...params, dimension: state.dim, metric: state.bmetric, limit: 50 },
    keep,
  );
  const filterOptions = useFilterOptions(app.id, params, REPORT_FILTER_DIMENSIONS);

  const summaryData = summary.data?.status === 200 ? summary.data.data : undefined;
  // Пустое приложение отличается от пустого периода только наличием первого события.
  const health = useGetIntegrationHealth(app.id, undefined, {
    query: { enabled: summaryData?.active === 0 },
  });
  const neverReceived =
    summaryData?.active === 0 &&
    health.data?.status === 200 &&
    health.data.data.first_event_at === null;

  const sparklines: Partial<Record<ChartMetric, TimeSeriesPoint[]>> = {};
  METRICS.forEach((metric, i) => {
    const q = sparklineQueries[i];
    if (q?.data?.status === 200) sparklines[metric] = q.data.data.points;
  });

  const rangeError = [summary, chart, rolling, breakdown].some(isRangeError)
    ? errorTexts.range_too_large
    : undefined;
  const chartData = chart.data?.status === 200 ? chart.data.data : undefined;
  const rollingData = rolling.data?.status === 200 ? rolling.data.data : undefined;
  const breakdownData = breakdown.data?.status === 200 ? breakdown.data.data : undefined;

  const changePeriod = (next: PeriodValue) => {
    const g = coerceGranularity(state.granularity, allowedGranularities(next));
    if (g !== state.granularity) {
      toast.info(fmt(m.granularity.changed, { granularity: m.granularity.options[g] }));
    }
    void update({ ...periodSearch(next), ...(g !== state.granularity ? { g } : {}) });
  };
  const changeFilters = (filters: Filters) => void update(filtersSearch(filters));
  // Выбор платформы выключает режим «По платформам»: он имеет смысл только при «Все».
  const changePlatform = (next: Platform | null) =>
    void update(next ? { platform: [next], split: null } : { platform: null });
  const addFilter = (value: string) => {
    const dim = state.dim;
    if (dim === "platform") {
      // Платформа — одно значение переключателя: строка разбивки заменяет выбранную.
      const platform = Object.values(Platform).find((p) => p === value);
      if (platform) changePlatform(platform);
      return;
    }
    if (!(FILTER_DIMENSIONS as readonly string[]).includes(dim)) return;
    const key = dim as (typeof FILTER_DIMENSIONS)[number];
    const current: string[] = state.filters[key] ?? [];
    if (current.includes(value)) return;
    changeFilters({ ...state.filters, [key]: [...current, value] } as Filters);
  };
  const copyRequest = async () => {
    await navigator.clipboard.writeText(audienceCurl(app.id, state));
    toast.success(m.audience.requestCopied, { duration: 1500 });
  };

  // Панель отчёта: на телефоне период, детализация, платформа и окружение — каждый на всю ширину,
  // сравнение, запрос и фильтры переносятся строками; с md — две строки, как на десктопе.
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start gap-3">
          <PeriodPicker
            value={state.period}
            onChange={changePeriod}
            timezone={app.timezone}
            error={rangeError}
          />
          <GranularitySelect
            value={state.granularity}
            allowed={allowedGranularities(state.period)}
            onChange={(g) => void update({ g })}
          />
          <div className="flex h-11 items-center md:h-8">
            <CompareToggle
              checked={state.compare}
              onCheckedChange={(compare) => void update({ compare })}
              {...(split ? { disabledReason: m.compare.unavailableInSplit } : {})}
            />
          </div>
          <Button
            variant="outline"
            size="sm"
            className={cn(TOUCH_BUTTON, "md:ml-auto")}
            onClick={() => void copyRequest()}
          >
            <Copy aria-hidden="true" />
            {m.audience.copyRequest}
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <PlatformSwitcher
            appPlatforms={app.platforms}
            observed={observedPlatforms}
            value={platform}
            onChange={changePlatform}
          />
          <FilterBar
            dims={REPORT_FILTER_DIMENSIONS}
            value={state.filters}
            onChange={changeFilters}
            options={filterOptions.options}
            onOpenChange={filterOptions.onOpenChange}
            loading={filterOptions.loading}
          />
          <div className="w-full md:ml-auto md:w-auto">
            <EnvironmentSwitch value={state.env} onChange={(env) => void update({ env })} />
          </div>
        </div>
        {state.period.to >= addDays(today, 1 - OPEN_DAYS) ? (
          <p className="text-caption text-fg-muted">{m.audience.lateData}</p>
        ) : null}
      </div>

      {neverReceived ? (
        <Card>
          <EmptyState
            icon={PlugZap}
            title={m.audience.noEvents.title}
            description={m.audience.noEvents.description}
            action={{
              label: m.audience.noEvents.action,
              href: `/apps/${app.id}/integration` as Route,
            }}
          />
        </Card>
      ) : (
        <>
          <AudienceKpis
            summary={summaryData}
            sparklines={sparklines}
            selected={state.metric}
            onSelect={(metric) => void update({ metric })}
            loading={summary.isPending}
            error={cardError(summary)}
          />

          <Card className="gap-4 px-4 md:px-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              {/* На узком телефоне подписи метрик не помещаются в строку — переключатель прокручивается. */}
              <div className="w-full overflow-x-auto md:w-auto md:overflow-visible">
                <ToggleGroup
                  type="single"
                  variant="outline"
                  size="sm"
                  spacing={0}
                  aria-label={m.audience.chartMetric}
                  className={STRETCH_GROUP}
                  value={state.metric}
                  onValueChange={(next) => {
                    const metric = METRICS.find((x) => x === next);
                    if (metric) void update({ metric });
                  }}
                >
                  {METRICS.map((metric) => (
                    <ToggleGroupItem
                      key={metric}
                      value={metric}
                      className={cn("flex-auto md:flex-none", TOUCH_TOGGLE, "px-3")}
                    >
                      {metrics[metric].label}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
              {canSplit ? (
                <ToggleGroup
                  type="single"
                  variant="outline"
                  size="sm"
                  spacing={0}
                  aria-label={m.audience.chartMode}
                  className={STRETCH_GROUP}
                  value={split ? "platform" : "total"}
                  onValueChange={(next) => {
                    if (next) void update({ split: next === "platform" ? "platform" : null });
                  }}
                >
                  <ToggleGroupItem value="total" className={cn(STRETCH_ITEM, TOUCH_TOGGLE, "px-3")}>
                    {m.chart.total}
                  </ToggleGroupItem>
                  <ToggleGroupItem
                    value="platform"
                    className={cn(STRETCH_ITEM, TOUCH_TOGGLE, "px-3")}
                  >
                    {m.audience.byPlatform}
                  </ToggleGroupItem>
                </ToggleGroup>
              ) : null}
            </div>
            {cardError(chart) ??
              (chartData ? (
                <MetricChart
                  series={chartSeries(chartData, split, m.chart.total, platformLabel)}
                  granularity={state.granularity}
                  metric={state.metric}
                  timezone={app.timezone}
                />
              ) : chart.isPending ? (
                <Skeleton
                  role="status"
                  aria-label={messages.common.loading}
                  className="h-55 w-full md:h-90"
                />
              ) : null)}
          </Card>

          <Card className="gap-4 px-4 md:px-6">
            <h2 className="text-heading-3">DAU / WAU / MAU</h2>
            {cardError(rolling) ??
              (rollingData ? (
                <RollingActiveChart data={rollingData} />
              ) : rolling.isPending ? (
                <Skeleton
                  role="status"
                  aria-label={messages.common.loading}
                  className="h-55 w-full md:h-70"
                />
              ) : null)}
          </Card>

          <Card className="gap-4 px-4 md:px-6">
            <h2 className="text-heading-3">{m.breakdown.title}</h2>
            <Suspense fallback={<TableSkeleton rows={6} />}>
              <BreakdownTable
                dimension={state.dim}
                onDimensionChange={(dim) => void update({ dim })}
                metric={state.bmetric}
                onMetricChange={(bmetric) => void update({ bmetric })}
                data={breakdownData}
                onRowClick={addFilter}
                loading={breakdown.isPending}
                error={cardError(breakdown)}
              />
            </Suspense>
          </Card>
        </>
      )}
    </div>
  );
}
