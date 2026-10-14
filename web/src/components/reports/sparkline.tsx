type SparklineProps = {
  values: number[];
  width?: number;
  height?: number;
  /** sr-only описание тренда: «Тренд за 30 дней: рост». */
  trendLabel: string;
};

const PAD = 4;

// Мини-график без осей (14-ux-ui-spec.md §16.6): последняя точка отмечена кружком 3 px.
// График декоративный, смысл передаёт trendLabel. Рисуется обычным SVG, а не Recharts:
// sparkline есть в каждой строке портфеля, и Recharts в initial JS не укладывается в бюджет
// 250 KB gz (NFR-006, OQ-061).
export function Sparkline({ values, width = 96, height = 28, trendLabel }: SparklineProps) {
  const min = Math.min(...values);
  const max = Math.max(...values);
  const stepX = values.length > 1 ? (width - 2 * PAD) / (values.length - 1) : 0;
  const plotHeight = height - 2 * PAD;
  const points = values.map((value, i) => ({
    // Одна точка — у правого края, как последняя точка ряда.
    x: values.length > 1 ? PAD + i * stepX : width - PAD,
    // Ровный ряд — линия посередине; иначе максимум у верхнего края, минимум — у нижнего.
    y: max === min ? height / 2 : PAD + ((max - value) / (max - min)) * plotHeight,
  }));
  const round = (n: number) => Math.round(n * 100) / 100;
  const d = points.map((p, i) => `${i === 0 ? "M" : "L"}${round(p.x)} ${round(p.y)}`).join(" ");
  const last = points.at(-1);

  return (
    <span className="inline-flex align-middle">
      <span data-sparkline aria-hidden="true" className="block" style={{ width, height }}>
        <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden="true">
          {last ? (
            <>
              <path
                d={d}
                fill="none"
                stroke="var(--chart-1)"
                strokeWidth={1.5}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
              <circle cx={round(last.x)} cy={round(last.y)} r={3} fill="var(--chart-1)" />
            </>
          ) : null}
        </svg>
      </span>
      <span className="sr-only">{trendLabel}</span>
    </span>
  );
}
