"use client";

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { CHART_HEIGHT, chartHeight } from "@/components/reports/responsive";
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import type { RealtimeMinute } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";

type PerMinuteChartProps = { points: RealtimeMinute[]; timezone: string; height?: number };

// Старты сессий по минутам за 30 минут (14-ux-ui-spec.md §16.10). Опрос раз в 5 с перерисовывает
// столбики на месте, без анимации (§12.2).
export function PerMinuteChart({ points, timezone, height = 200 }: PerMinuteChartProps) {
  const m = useMessages().realtime.chart;
  const f = useFormat();
  const first = points[0];
  const last = points.at(-1);

  if (!first || !last) {
    return <p className="text-body-sm text-fg-muted">{m.noData}</p>;
  }

  const config = {
    sessions_started: { label: m.series, color: "var(--chart-1)" },
  } satisfies ChartConfig;
  const total = points.reduce((sum, p) => sum + p.sessions_started, 0);
  const description = fmt(m.description, {
    from: f.formatTime(first.minute, timezone),
    to: f.formatTime(last.minute, timezone),
    total: f.formatInt(total),
    last: f.formatInt(last.sessions_started),
  });
  const rows = points.map((p) => ({ ...p, label: f.formatTime(p.minute, timezone) }));

  return (
    <div role="img" aria-label={description} className={CHART_HEIGHT} style={chartHeight(height)}>
      <ChartContainer config={config} className="aspect-auto h-full w-full">
        <BarChart
          data={rows}
          margin={{ top: 8, right: 8, bottom: 0, left: 8 }}
          accessibilityLayer={false}
        >
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            ticks={[rows[0]?.label ?? "", rows.at(-1)?.label ?? ""]}
            interval="preserveStartEnd"
          />
          <YAxis
            width="auto"
            tickLine={false}
            axisLine={false}
            allowDecimals={false}
            tickFormatter={(v: number) => f.formatInt(v)}
          />
          <ChartTooltip content={<ChartTooltipContent />} />
          <Bar
            dataKey="sessions_started"
            fill="var(--color-sessions_started)"
            radius={[2, 2, 0, 0]}
            isAnimationActive={false}
          />
        </BarChart>
      </ChartContainer>
    </div>
  );
}
