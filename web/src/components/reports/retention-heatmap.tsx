"use client";

import { useState } from "react";
import type { RetentionCohort, RetentionMatrix, Unit } from "@/lib/api/gen/model";
import { formatDate, formatInt, formatPercent, formatWeekRange } from "@/lib/format";
import { RETENTION_STEP_CLASSES, retentionStep } from "@/lib/retention-color";
import { cn } from "@/lib/utils";

type RetentionHeatmapProps = {
  matrix: RetentionMatrix;
  /** 'day' → D0..Dn, 'week' → W0..Wn */
  granularity: "day" | "week";
  /** Единица подсчёта для подсказок: люди или установки. */
  unit?: Unit;
};

const COPY = {
  day: {
    prefix: "D",
    period: "день",
    caption: "Retention по дням: доля когорты, вернувшейся в день N",
  },
  week: {
    prefix: "W",
    period: "неделя",
    caption: "Retention по неделям: доля когорты, вернувшейся в неделю N",
  },
} as const;

const CELL = "h-8 w-14 min-w-14 px-1 text-center";
// Закреплённые колонки «Когорта» (132 px) и «Размер»; шапка и строка «Среднее» закреплены сверху.
const COHORT_COL = "sticky left-0 w-[132px] min-w-[132px] px-3 text-left";
const SIZE_COL = "sticky left-[132px] w-20 min-w-20 px-3 text-right";

// Матрица когорт S-06 (14-ux-ui-spec.md S-06, §16.9): настоящая таблица, 6-шаговая шкала цвета,
// D0 (W0) не окрашивается, ненаступившие периоды пустые. Клик по шапке периода выделяет колонку.
export function RetentionHeatmap({ matrix, granularity, unit = "persons" }: RetentionHeatmapProps) {
  const [selected, setSelected] = useState<number | null>(null);
  const copy = COPY[granularity];
  const columns = Array.from({ length: matrix.periods + 1 }, (_, n) => n);
  const who = unit === "installs" ? "устр." : "чел.";
  const label = (start: string) =>
    granularity === "week" ? formatWeekRange(start) : formatDate(start);

  const filled = matrix.cohorts.filter((c) => c.size > 0);
  const meanSize =
    filled.length > 0 ? filled.reduce((sum, c) => sum + c.size, 0) / filled.length : null;

  const valueCell = (value: number | null | undefined, n: number, cohort?: RetentionCohort) => {
    const mark = selected === n ? { "data-selected": "" } : {};
    const outline = selected === n && "outline-2 -outline-offset-2 outline-ring";
    // Ячейки строки «Среднее» (без когорты) закреплены сверху под шапкой.
    const base = cohort ? CELL : cn(CELL, "sticky top-8 z-20 bg-subtle");
    if (value === null || value === undefined) {
      return <td key={n} aria-label="нет данных" className={cn(base, outline)} {...mark} />;
    }
    const text = formatPercent(value);
    if (!cohort) {
      return (
        <td key={n} className={cn(base, outline)} {...mark}>
          {text}
        </td>
      );
    }
    const title = `Когорта ${label(cohort.cohort_start)} (${formatInt(cohort.size)} ${who}) · ${copy.period} ${n}: ${formatInt(Math.round(cohort.size * value))} ${who} (${text})`;
    if (n === 0) {
      return (
        <td
          key={n}
          title={title}
          className={cn(CELL, "bg-subtle text-fg-muted", outline)}
          {...mark}
        >
          {text}
        </td>
      );
    }
    const step = retentionStep(value);
    return (
      <td
        key={n}
        title={title}
        data-step={step}
        className={cn(CELL, RETENTION_STEP_CLASSES[step], outline)}
        {...mark}
      >
        {text}
      </td>
    );
  };

  return (
    <section
      // biome-ignore lint/a11y/noNoninteractiveTabindex: прокручиваемая область должна быть доступна с клавиатуры
      tabIndex={0}
      aria-label="Матрица когорт, прокручивается по горизонтали"
      className="max-h-[640px] overflow-auto rounded-md border border-border outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <table className="border-separate border-spacing-px text-body-sm tabular-nums">
        <caption className="sr-only">{copy.caption}</caption>
        <thead>
          <tr className="text-label-sm text-fg-muted uppercase">
            <th scope="col" className={cn(COHORT_COL, "top-0 z-40 h-8 bg-subtle font-medium")}>
              Когорта
            </th>
            <th scope="col" className={cn(SIZE_COL, "top-0 z-40 bg-subtle font-medium")}>
              Размер
            </th>
            {columns.map((n) => (
              <th key={n} scope="col" className="sticky top-0 z-30 w-14 min-w-14 bg-subtle p-0">
                <button
                  type="button"
                  aria-pressed={selected === n}
                  onClick={() => setSelected((s) => (s === n ? null : n))}
                  className={cn(
                    "h-8 w-full font-medium uppercase outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                    selected === n && "text-foreground outline-2 -outline-offset-2 outline-ring",
                  )}
                >
                  {copy.prefix}
                  {n}
                </button>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          <tr data-average className="font-semibold">
            <th scope="row" className={cn(COHORT_COL, "top-8 z-[25] bg-subtle font-semibold")}>
              Среднее
            </th>
            <td className={cn(SIZE_COL, "top-8 z-[25] bg-subtle")}>{formatInt(meanSize)}</td>
            {columns.map((n) => valueCell(matrix.average[n], n))}
          </tr>
          {matrix.cohorts.map((cohort) => (
            <tr key={cohort.cohort_start}>
              <th
                scope="row"
                className={cn(COHORT_COL, "z-10 bg-card font-normal whitespace-nowrap")}
              >
                {label(cohort.cohort_start)}
              </th>
              <td className={cn(SIZE_COL, "z-10 bg-card")}>{formatInt(cohort.size)}</td>
              {columns.map((n) => valueCell(cohort.values[n], n, cohort))}
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
