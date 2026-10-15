"use client";

import { useState } from "react";
import { Area, AreaChart, CartesianGrid, Line, Tooltip, XAxis, YAxis } from "recharts";
import { platformChartVar } from "@/components/common/platform-icon";
import { Button } from "@/components/ui/button";
import { type ChartConfig, ChartContainer } from "@/components/ui/chart";
import type { TimeSeriesPoint } from "@/lib/api/gen/model";
import type { Formatters } from "@/lib/format";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { useMetricDefinitions } from "@/lib/metric-definitions";
import { type Granularity, todayIn } from "@/lib/period";
import type { ChartMetric } from "@/lib/url-state";
import { CHART_HEIGHT, chartHeight } from "./responsive";

export type MetricChartSeries = {
  /** "total" — итог, иначе значение events.platform. */
  key: string;
  /** «Итог» или название платформы. */
  label: string;
  points: TimeSeriesPoint[];
  /** Ряд предыдущего периода — только у итога. */
  compare?: TimeSeriesPoint[];
};

type MetricChartProps = {
  /** Итог — одна серия "total"; режим «По платформам» — серия на платформу. */
  series: MetricChartSeries[];
  granularity: Granularity;
  metric: ChartMetric;
  /** TZ приложения — для подписей часов и дней. */
  timezone: string;
  height?: number;
};

// Значения серий лежат под ключами s0…sN: значение events.platform может совпасть с полями точки.
type Datum = { ts: string; label: string; compare: number | null } & Record<
  `s${number}`,
  number | null
>;

type SeriesLine = { dataKey: `s${number}`; label: string; color: string };

export const formatMetric = (
  f: Formatters,
  metric: ChartMetric,
  value: number | null | undefined,
): string => (metric === "avg_duration_ms" ? f.formatDuration(value) : f.formatInt(value));

// Календарный день начала корзины в TZ приложения.
const dayOf = (ts: string, timezone: string) => todayIn(timezone, new Date(ts));

export function seriesRange(f: Formatters, timestamps: string[], timezone: string): string {
  const first = timestamps[0];
  const last = timestamps.at(-1);
  return first && last ? f.formatDateRange(dayOf(first, timezone), dayOf(last, timezone)) : "";
}

type TableToggleProps = { shown: boolean; onToggle: () => void };

// «Показать таблицей» (14-ux-ui-spec.md S-05, Accessibility): точки графика таблицей для скринридеров.
export function TableToggle({ shown, onToggle }: TableToggleProps) {
  const m = useMessages().reports.chart;
  return (
    <Button
      variant="link"
      size="sm"
      className="self-start px-0"
      aria-expanded={shown}
      onClick={onToggle}
    >
      {shown ? m.hideTable : m.showTable}
    </Button>
  );
}

type TipProps = {
  active?: boolean;
  payload?: ReadonlyArray<{ payload?: Datum }>;
  metric: ChartMetric;
  compare: boolean;
  /** Линии режима «По платформам»; в итоге — undefined. */
  lines?: SeriesLine[];
};

function ChartTip({ active, payload, metric, compare, lines }: TipProps) {
  const m = useMessages().reports.chart;
  const f = useFormat();
  const d = payload?.[0]?.payload;
  if (!active || !d) return null;
  if (lines) {
    return (
      <div className="flex flex-col gap-1 rounded-md border border-border bg-popover px-3 py-2 text-body-sm shadow-md">
        <span className="text-fg-muted">{d.label}</span>
        {lines.map((l) => (
          <span key={l.dataKey} className="flex items-center gap-2">
            <Swatch color={l.color} />
            {l.label}
            <span className="ml-auto pl-3 font-medium tabular-nums">
              {formatMetric(f, metric, d[l.dataKey])}
            </span>
          </span>
        ))}
      </div>
    );
  }
  const value = d.s0 ?? null;
  const delta =
    compare && value !== null && d.compare !== null && d.compare !== 0
      ? ((value - d.compare) / d.compare) * 100
      : null;
  return (
    <div className="flex flex-col gap-1 rounded-md border border-border bg-popover px-3 py-2 text-body-sm shadow-md">
      <span className="text-fg-muted">{d.label}</span>
      <span className="font-medium tabular-nums">{formatMetric(f, metric, value)}</span>
      {compare ? (
        <span className="text-fg-muted tabular-nums">
          {fmt(m.previousValue, { value: formatMetric(f, metric, d.compare) })}
          {delta !== null ? ` (${f.formatDelta(delta)})` : ""}
        </span>
      ) : null}
    </div>
  );
}

function Swatch({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="size-2.5 shrink-0 rounded-full"
      style={{ background: color }}
    />
  );
}

// Точки серий по корзинам: ряды API покрывают одну сетку периода, корзина отсутствующей точки — null.
function chartData(
  f: Formatters,
  series: MetricChartSeries[],
  granularity: Granularity,
  timezone: string,
) {
  const rows = new Map<string, Datum>();
  const row = (ts: string): Datum => {
    const existing = rows.get(ts);
    if (existing) return existing;
    const d: Datum = { ts, label: f.formatBucket(ts, granularity, timezone), compare: null };
    series.forEach((_, i) => {
      d[`s${i}`] = null;
    });
    rows.set(ts, d);
    return d;
  };
  series.forEach((s, i) => {
    for (const p of s.points) row(p.ts)[`s${i}`] = p.value;
  });
  // Ряд предыдущего периода сопоставляется с текущим по номеру корзины.
  const compare = series[0]?.compare;
  if (compare) {
    series[0]?.points.forEach((p, i) => {
      row(p.ts).compare = compare[i]?.value ?? null;
    });
  }
  return [...rows.values()];
}

// Максимум серии для описания графика; нулевые и пустые корзины не в счёт.
function seriesMax(data: Datum[], key: `s${number}`): Datum | undefined {
  let max: Datum | undefined;
  for (const d of data) {
    const v = d[key] ?? null;
    if (v !== null && v > 0 && (!max || v > (max[key] ?? 0))) max = d;
  }
  return max;
}

// График метрики S-05 (14-ux-ui-spec.md §16.6): область текущего периода и пунктир предыдущего, в режиме
// «По платформам» — линия на платформу с легендой. Для скринридеров — role="img"
// с кратким описанием и таблица точек по кнопке.
export function MetricChart({
  series,
  granularity,
  metric,
  timezone,
  height = 360,
}: MetricChartProps) {
  const m = useMessages().reports.chart;
  const metrics = useMetricDefinitions();
  const f = useFormat();
  const [showTable, setShowTable] = useState(false);
  const split = series.some((s) => s.key !== "total");
  const compare = !split && series[0]?.compare !== undefined;
  const data = chartData(f, series, granularity, timezone);
  const lines: SeriesLine[] = series.map((s, i) => ({
    dataKey: `s${i}`,
    label: split ? s.label : metrics[metric].label,
    color: split ? platformChartVar(s.key) : "var(--chart-1)",
  }));

  const maxes = lines.map((l) => seriesMax(data, l.dataKey));
  const max = (i: number) => {
    const d = maxes[i];
    return d
      ? fmt(m.peak, { value: formatMetric(f, metric, d[`s${i}`]), bucket: d.label })
      : m.noEventsShort;
  };
  const range = seriesRange(
    f,
    data.map((d) => d.ts),
    timezone,
  );
  const hasEvents = maxes.some(Boolean);
  const summary =
    split && hasEvents ? lines.map((l, i) => `${l.label}: ${max(i)}`).join(", ") : max(0);
  const description = [
    fmt(split ? m.headingSplit : m.heading, { title: m.titles[metric], by: m.by[granularity] }),
    range,
    summary,
  ]
    .filter(Boolean)
    .join(", ");

  const config: ChartConfig = {
    compare: { label: m.previousPeriod, color: "var(--chart-compare)" },
  };
  for (const l of lines) config[l.dataKey] = { label: l.label, color: l.color };

  return (
    <figure className="flex flex-col gap-2">
      {split ? (
        <ul
          aria-label={m.legend}
          className="flex flex-wrap gap-x-4 gap-y-1 text-body-sm text-fg-muted"
        >
          {lines.map((l) => (
            <li key={l.dataKey} className="flex items-center gap-1.5">
              <Swatch color={l.color} />
              {l.label}
            </li>
          ))}
        </ul>
      ) : null}
      <div role="img" aria-label={description} className={CHART_HEIGHT} style={chartHeight(height)}>
        <ChartContainer config={config} className="aspect-auto h-full w-full">
          <AreaChart
            data={data}
            margin={{ top: 8, right: 8, bottom: 0, left: 8 }}
            accessibilityLayer={false}
          >
            <CartesianGrid vertical={false} />
            <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
            <YAxis
              width="auto"
              tickLine={false}
              axisLine={false}
              tickFormatter={(v: number) => formatMetric(f, metric, v)}
            />
            <Tooltip
              content={<ChartTip metric={metric} compare={compare} {...(split ? { lines } : {})} />}
            />
            {split
              ? lines.map((l) => (
                  <Line
                    key={l.dataKey}
                    type="monotone"
                    dataKey={l.dataKey}
                    name={l.label}
                    stroke={`var(--color-${l.dataKey})`}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                  />
                ))
              : lines.map((l) => (
                  <Area
                    key={l.dataKey}
                    type="monotone"
                    dataKey={l.dataKey}
                    stroke={`var(--color-${l.dataKey})`}
                    fill={`var(--color-${l.dataKey})`}
                    fillOpacity={0.12}
                    strokeWidth={2}
                    isAnimationActive={false}
                  />
                ))}
            {compare ? (
              <Line
                type="monotone"
                dataKey="compare"
                stroke="var(--color-compare)"
                strokeDasharray="4 4"
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
            ) : null}
          </AreaChart>
        </ChartContainer>
      </div>
      {hasEvents ? null : (
        <figcaption className="text-body-sm text-fg-muted">{m.noEvents}</figcaption>
      )}
      <TableToggle shown={showTable} onToggle={() => setShowTable((s) => !s)} />
      {showTable ? (
        <table className="w-full text-body-sm">
          <caption className="sr-only">{description}</caption>
          <thead>
            <tr className="border-b border-border text-left text-fg-muted">
              <th scope="col" className="py-2 font-medium">
                {m.period}
              </th>
              {lines.map((l) => (
                <th key={l.dataKey} scope="col" className="py-2 text-right font-medium">
                  {l.label}
                </th>
              ))}
              {compare ? (
                <th scope="col" className="py-2 text-right font-medium">
                  {m.previousPeriod}
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {data.map((d) => (
              <tr key={d.ts} className="border-b border-border last:border-0">
                <th scope="row" className="py-1.5 text-left font-normal">
                  {d.label}
                </th>
                {lines.map((l) => (
                  <td key={l.dataKey} className="py-1.5 text-right tabular-nums">
                    {formatMetric(f, metric, d[l.dataKey])}
                  </td>
                ))}
                {compare ? (
                  <td className="py-1.5 text-right tabular-nums">
                    {formatMetric(f, metric, d.compare)}
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </figure>
  );
}
