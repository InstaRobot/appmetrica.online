"use client";

import { type ComponentProps, lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";

// Графики Recharts (с d3, redux и immer ≈ 150 KB gz) загружаются отдельным чанком после первой
// загрузки страницы: в initial JS они не укладываются в бюджет 250 KB gz (NFR-006, T-044).
// Данные графиков всё равно приходят запросом с клиента, поэтому чанк грузится параллельно с ними.
// Заглушка той же высоты, что и график (плюс «Показать таблицей» у графиков с таблицей), не сдвигает
// страницу, когда график появляется.

const TABLE_TOGGLE_HEIGHT = 40;

function ChartSkeleton({ height }: { height: number }) {
  return <Skeleton role="status" aria-label="Загрузка графика…" style={{ height }} />;
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
    <Suspense fallback={<ChartSkeleton height={(props.height ?? 360) + TABLE_TOGGLE_HEIGHT} />}>
      <LazyMetricChart {...props} />
    </Suspense>
  );
}

export function RollingActiveChart(props: ComponentProps<typeof LazyRollingActiveChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={(props.height ?? 280) + TABLE_TOGGLE_HEIGHT} />}>
      <LazyRollingActiveChart {...props} />
    </Suspense>
  );
}

export function RetentionAverageChart(props: ComponentProps<typeof LazyRetentionAverageChart>) {
  return (
    <Suspense fallback={<ChartSkeleton height={(props.height ?? 220) + TABLE_TOGGLE_HEIGHT} />}>
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
