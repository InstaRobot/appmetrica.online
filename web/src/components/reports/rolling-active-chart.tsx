"use client";

import { useState } from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import type { RollingActive } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { TableToggle } from "./metric-chart";

type RollingActiveChartProps = { data: RollingActive; height?: number };

const SERIES = ["dau", "wau", "mau"] as const;

const config = {
  dau: { label: "DAU", color: "var(--chart-1)" },
  wau: { label: "WAU", color: "var(--chart-2)" },
  mau: { label: "MAU", color: "var(--chart-3)" },
} satisfies ChartConfig;

// DAU / WAU / MAU (14-ux-ui-spec.md §16.6): для каждого дня — активные за день, 7 и 30 дней по этот день.
export function RollingActiveChart({ data, height = 280 }: RollingActiveChartProps) {
  const messages = useMessages().reports;
  const m = messages.rolling;
  const f = useFormat();
  const [showTable, setShowTable] = useState(false);
  const points = data.points;
  const first = points[0];
  const last = points.at(-1);

  if (!first || !last) {
    return <p className="text-body-sm text-fg-muted">{messages.noData}</p>;
  }

  const description = fmt(m.description, {
    range: f.formatDateRange(first.day, last.day),
    dau: f.formatInt(last.dau),
    wau: f.formatInt(last.wau),
    mau: f.formatInt(last.mau),
  });
  const rows = points.map((p) => ({ ...p, label: f.formatDateShort(p.day) }));

  return (
    <figure className="flex flex-col gap-2">
      <div role="img" aria-label={description} style={{ height }}>
        <ChartContainer config={config} className="aspect-auto h-full w-full">
          <LineChart
            data={rows}
            margin={{ top: 8, right: 8, bottom: 0, left: 8 }}
            accessibilityLayer={false}
          >
            <CartesianGrid vertical={false} />
            <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
            <YAxis
              width="auto"
              tickLine={false}
              axisLine={false}
              tickFormatter={(v: number) => f.formatInt(v)}
            />
            <ChartTooltip content={<ChartTooltipContent />} />
            <ChartLegend content={<ChartLegendContent />} />
            {SERIES.map((key) => (
              <Line
                key={key}
                type="monotone"
                dataKey={key}
                stroke={`var(--color-${key})`}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ChartContainer>
      </div>
      <TableToggle shown={showTable} onToggle={() => setShowTable((s) => !s)} />
      {showTable ? (
        <table className="w-full text-body-sm">
          <caption className="sr-only">{description}</caption>
          <thead>
            <tr className="border-b border-border text-left text-fg-muted">
              <th scope="col" className="py-2 font-medium">
                {m.day}
              </th>
              {SERIES.map((key) => (
                <th key={key} scope="col" className="py-2 text-right font-medium">
                  {config[key].label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {points.map((p) => (
              <tr key={p.day} className="border-b border-border last:border-0">
                <th scope="row" className="py-1.5 text-left font-normal">
                  {f.formatDate(p.day)}
                </th>
                {SERIES.map((key) => (
                  <td key={key} className="py-1.5 text-right tabular-nums">
                    {f.formatInt(p[key])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </figure>
  );
}
