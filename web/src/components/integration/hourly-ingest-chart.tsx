"use client";

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import type { IntegrationHealth } from "@/lib/api/gen/model";
import { formatInt, formatTime } from "@/lib/format";

type HourlyIngestChartProps = {
  hourly: IntegrationHealth["hourly"];
  timezone: string;
  height?: number;
};

const config = {
  accepted: { label: "Принято", color: "var(--chart-1)" },
  rejected: { label: "Отклонено", color: "var(--color-danger-500)" },
} satisfies ChartConfig;

// Принятые и отклонённые события по часам за 24 ч (14-ux-ui-spec.md S-10), stacked BarChart.
export function HourlyIngestChart({ hourly, timezone, height = 220 }: HourlyIngestChartProps) {
  const first = hourly[0];
  const last = hourly.at(-1);

  if (!first || !last) {
    return <p className="text-body-sm text-fg-muted">Нет данных за 24 часа</p>;
  }

  const accepted = hourly.reduce((sum, h) => sum + h.accepted, 0);
  const rejected = hourly.reduce((sum, h) => sum + h.rejected, 0);
  const description = `События по часам, ${formatTime(first.hour, timezone)}–${formatTime(last.hour, timezone)}: принято ${formatInt(accepted)}, отклонено ${formatInt(rejected)}`;
  const rows = hourly.map((h) => ({ ...h, label: formatTime(h.hour, timezone) }));

  return (
    <div role="img" aria-label={description} style={{ height }}>
      <ChartContainer config={config} className="aspect-auto h-full w-full">
        <BarChart
          data={rows}
          margin={{ top: 8, right: 8, bottom: 0, left: 8 }}
          accessibilityLayer={false}
        >
          <CartesianGrid vertical={false} />
          <XAxis dataKey="label" tickLine={false} axisLine={false} interval="preserveStartEnd" />
          <YAxis
            width="auto"
            tickLine={false}
            axisLine={false}
            allowDecimals={false}
            tickFormatter={(v: number) => formatInt(v)}
          />
          <ChartTooltip content={<ChartTooltipContent />} />
          <Bar
            dataKey="accepted"
            stackId="ingest"
            fill="var(--color-accepted)"
            isAnimationActive={false}
          />
          <Bar
            dataKey="rejected"
            stackId="ingest"
            fill="var(--color-rejected)"
            radius={[2, 2, 0, 0]}
            isAnimationActive={false}
          />
        </BarChart>
      </ChartContainer>
    </div>
  );
}
