"use client";

import { Line, LineChart } from "recharts";

type SparklineProps = {
  values: number[];
  width?: number;
  height?: number;
  /** sr-only описание тренда: «Тренд за 30 дней: рост». */
  trendLabel: string;
};

type DotProps = { cx?: number; cy?: number; index?: number };

// Мини-график без осей (14-ux-ui-spec.md §16.6): последняя точка отмечена кружком 3 px.
// График декоративный, смысл передаёт trendLabel.
export function Sparkline({ values, width = 96, height = 28, trendLabel }: SparklineProps) {
  const data = values.map((value, index) => ({ index, value }));
  const last = data.length - 1;

  return (
    <span className="inline-flex align-middle">
      <span data-sparkline aria-hidden="true" className="block" style={{ width, height }}>
        <LineChart
          width={width}
          height={height}
          data={data}
          margin={{ top: 4, right: 4, bottom: 4, left: 4 }}
          // Без слоя доступности Recharts: иначе <svg tabindex="0"> внутри aria-hidden ловит фокус.
          accessibilityLayer={false}
        >
          <Line
            type="monotone"
            dataKey="value"
            stroke="var(--chart-1)"
            strokeWidth={1.5}
            isAnimationActive={false}
            dot={({ cx, cy, index }: DotProps) => (
              <circle
                key={index}
                cx={cx}
                cy={cy}
                r={index === last ? 3 : 0}
                fill="var(--chart-1)"
                stroke="none"
              />
            )}
            activeDot={false}
          />
        </LineChart>
      </span>
      <span className="sr-only">{trendLabel}</span>
    </span>
  );
}
