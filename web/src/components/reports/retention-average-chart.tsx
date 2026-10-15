"use client";

import { useState } from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { type ChartConfig, ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { TableToggle } from "./metric-chart";

type RetentionAverageChartProps = {
  average: Array<number | null>;
  granularity: "day" | "week";
  height?: number;
};

type Datum = { label: string; value: number | null };

const COPY = {
  day: { prefix: "D", keys: [1, 7, 30] },
  week: { prefix: "W", keys: [1, 4, 12] },
} as const;

function ChartTip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: ReadonlyArray<{ payload?: Datum }>;
}) {
  const f = useFormat();
  const d = payload?.[0]?.payload;
  if (!active || !d) return null;
  return (
    <div className="flex flex-col gap-1 rounded-md border border-border bg-popover px-3 py-2 text-body-sm shadow-md">
      <span className="text-fg-muted">{d.label}</span>
      <span className="font-medium tabular-nums">{f.formatPercent(d.value)}</span>
    </div>
  );
}

// Средняя кривая удержания S-06 (14-ux-ui-spec.md §16.9): ключевые точки D1/D7/D30 (W1/W4/W12),
// линия по всем периодам и таблица точек по кнопке.
export function RetentionAverageChart({
  average,
  granularity,
  height = 220,
}: RetentionAverageChartProps) {
  const messages = useMessages();
  const m = messages.retention.average;
  const f = useFormat();
  const [showTable, setShowTable] = useState(false);
  const copy = COPY[granularity];

  if (average.every((v) => v === null)) {
    return <p className="text-body-sm text-fg-muted">{messages.reports.noData}</p>;
  }

  const config = { value: { label: m.series, color: "var(--chart-1)" } } satisfies ChartConfig;
  const data: Datum[] = average.map((value, n) => ({ label: `${copy.prefix}${n}`, value }));
  const keys = copy.keys.filter((n) => n < average.length);
  const description = fmt(m.description, {
    by: m.by[granularity],
    points: keys
      .map((n) => {
        const v = average[n];
        return `${copy.prefix}${n} ${v === null || v === undefined ? m.noData : f.formatPercent(v)}`;
      })
      .join(", "),
  });

  return (
    <figure className="flex flex-col gap-3">
      <ul aria-label={m.keyPoints} className="flex gap-8">
        {keys.map((n) => (
          <li key={n} className="flex items-baseline gap-2">
            <span className="text-label-sm text-fg-muted">
              {copy.prefix}
              {n}
            </span>
            <span className="text-heading-3 tabular-nums">{f.formatPercent(average[n])}</span>
          </li>
        ))}
      </ul>
      <div role="img" aria-label={description} style={{ height }}>
        <ChartContainer config={config} className="aspect-auto h-full w-full">
          <LineChart
            data={data}
            margin={{ top: 8, right: 8, bottom: 0, left: 8 }}
            accessibilityLayer={false}
          >
            <CartesianGrid vertical={false} />
            <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={16} />
            <YAxis
              width="auto"
              domain={[0, 1]}
              tickLine={false}
              axisLine={false}
              tickFormatter={(v: number) => f.formatPercent(v)}
            />
            <ChartTooltip content={<ChartTip />} />
            <Line
              type="monotone"
              dataKey="value"
              stroke="var(--color-value)"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ChartContainer>
      </div>
      <TableToggle shown={showTable} onToggle={() => setShowTable((s) => !s)} />
      {showTable ? (
        <table className="w-full max-w-sm text-body-sm">
          <caption className="sr-only">{description}</caption>
          <thead>
            <tr className="border-b border-border text-left text-fg-muted">
              <th scope="col" className="py-2 font-medium">
                {m.column[granularity]}
              </th>
              <th scope="col" className="py-2 text-right font-medium">
                {m.series}
              </th>
            </tr>
          </thead>
          <tbody>
            {data.map((d) => (
              <tr key={d.label} className="border-b border-border last:border-0">
                <th scope="row" className="py-1.5 text-left font-normal">
                  {d.label}
                </th>
                <td className="py-1.5 text-right tabular-nums">{f.formatPercent(d.value)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </figure>
  );
}
