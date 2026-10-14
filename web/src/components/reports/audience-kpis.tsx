"use client";

import type { AudienceSummary, TimeSeriesPoint } from "@/lib/api/gen/model";
import { formatDuration } from "@/lib/format";
import { METRIC_DEFINITIONS } from "@/lib/metric-definitions";
import type { ChartMetric } from "@/lib/url-state";
import { KpiCard } from "./kpi-card";

type AudienceKpisProps = {
  summary: AudienceSummary | undefined;
  /** Дневные ряды метрик для спарклайнов (getAudienceTimeseries с g=day). */
  sparklines?: Partial<Record<ChartMetric, TimeSeriesPoint[]>>;
  selected: ChartMetric;
  onSelect: (metric: ChartMetric) => void;
  loading?: boolean;
  /** ErrorState вместо карточек, если сводка не загрузилась. */
  error?: React.ReactNode;
};

const METRICS: { key: ChartMetric; format: "number" | "duration" }[] = [
  { key: "active", format: "number" },
  { key: "new", format: "number" },
  { key: "sessions", format: "number" },
  { key: "avg_duration_ms", format: "duration" },
];

// Четыре KPI экрана S-05 (Активные, Новые, Сессии, Ср. длительность) с дельтой к предыдущему периоду.
export function AudienceKpis({
  summary,
  sparklines,
  selected,
  onSelect,
  loading = false,
  error,
}: AudienceKpisProps) {
  return (
    <section aria-label="Ключевые метрики">
      {error ? (
        error
      ) : (
        <div className="grid grid-cols-4 gap-4">
          {METRICS.map(({ key, format }) => (
            <KpiCard
              key={key}
              label={METRIC_DEFINITIONS[key].label}
              definition={METRIC_DEFINITIONS[key].definition}
              value={summary ? summary[key] : null}
              format={format}
              deltaPct={summary?.compare ? summary.compare.delta_pct[key] : undefined}
              secondary={
                key === "avg_duration_ms" && summary?.median_duration_ms != null
                  ? `медиана ${formatDuration(summary.median_duration_ms)}`
                  : undefined
              }
              sparkline={sparklines?.[key]}
              selected={selected === key}
              onSelect={() => onSelect(key)}
              loading={loading}
            />
          ))}
        </div>
      )}
    </section>
  );
}
