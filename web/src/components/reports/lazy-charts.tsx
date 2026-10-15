"use client";

import { type ComponentProps, type CSSProperties, lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { useMessages } from "@/lib/i18n/client";
import { chartHeight } from "./responsive";

// Графики Recharts (с d3, redux и immer ≈ 150 KB gz) загружаются отдельным чанком после первой
// загрузки страницы: в initial JS они не укладываются в бюджет 250 KB gz (NFR-006, T-044).
// Данные графиков всё равно приходят запросом с клиента, поэтому чанк грузится параллельно с ними.
// Заглушка той же высоты, что и график (плюс «Показать таблицей» у графиков с таблицей), не сдвигает
// страницу, когда график появляется. На телефоне график ниже (CHART_HEIGHT), заглушка — тоже.

const TABLE_TOGGLE_HEIGHT = 40;

function ChartSkeleton({ height, extra = 0 }: { height: number; extra?: number }) {
  const m = useMessages().reports.chart;
  const style = { ...chartHeight(height), "--chart-extra": `${extra}px` } as CSSProperties;
  return (
    <Skeleton
      role="status"
      aria-label={m.loading}
      className="h-[calc(min(var(--chart-h),220px)+var(--chart-extra))] md:h-[calc(var(--chart-h)+var(--chart-extra))]"
      style={style}
    />
  );
}

const LazyMetricChart = lazy(() =>
  import("./metric-chart").then((m) => ({ default: m.MetricChart })),
);
const LazyRollingActiveChart = lazy(() =>
  import("./rolling-active-chart").then((m) => ({ default: m.RollingActiveChart })),
);
const LazyRetentionAverageChart = lazy(() =>
  import("./retention-average-chart").then((m) => ({ default: m.RetentionAverageChart })),
);
const LazyPerMinuteChart = lazy(() =>
  import("@/components/realtime/per-minute-chart").then((m) => ({ default: m.PerMinuteChart })),
);
const LazyHourlyIngestChart = lazy(() =>
  import("@/components/integration/hourly-ingest-chart").then((m) => ({
    default: m.HourlyIngestChart,
  })),
);

export function MetricChart(props: ComponentProps<typeof LazyMetricChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={props.height ?? 360} extra={TABLE_TOGGLE_HEIGHT} />}>
      <LazyMetricChart {...props} />
    </Suspense>
  );
}

export function RollingActiveChart(props: ComponentProps<typeof LazyRollingActiveChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={props.height ?? 280} extra={TABLE_TOGGLE_HEIGHT} />}>
      <LazyRollingActiveChart {...props} />
    </Suspense>
  );
}

export function RetentionAverageChart(props: ComponentProps<typeof LazyRetentionAverageChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={props.height ?? 220} extra={TABLE_TOGGLE_HEIGHT} />}>
      <LazyRetentionAverageChart {...props} />
    </Suspense>
  );
}

export function PerMinuteChart(props: ComponentProps<typeof LazyPerMinuteChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={props.height ?? 200} />}>
      <LazyPerMinuteChart {...props} />
    </Suspense>
  );
}

export function HourlyIngestChart(props: ComponentProps<typeof LazyHourlyIngestChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={props.height ?? 220} />}>
      <LazyHourlyIngestChart {...props} />
    </Suspense>
  );
}
