"use client";

import { useState } from "react";
import type { RetentionCohort, RetentionMatrix, Unit } from "@/lib/api/gen/model";
import { useFormat, useLocale, useMessages } from "@/lib/i18n/client";
import { fmt, plural } from "@/lib/i18n/format-message";
import { RETENTION_STEP_CLASSES, retentionStep } from "@/lib/retention-color";
import { cn } from "@/lib/utils";

type RetentionHeatmapProps = {
  matrix: RetentionMatrix;
  /** 'day' → D0..Dn, 'week' → W0..Wn */
  granularity: "day" | "week";
  /** Единица подсчёта для подсказок: люди или установки. */
  unit?: Unit;
};

const PREFIX = { day: "D", week: "W" } as const;

const CELL = "h-8 w-14 min-w-14 px-1 text-center";
// Закреплённые колонки «Когорта» (132 px) и «Размер»; шапка и строка «Среднее» закреплены сверху.
// На телефоне по горизонтали закреплена только «Когорта»: «Размер» уезжает под неё вместе с цифрами,
// поэтому его z-index там ниже, чем у «Когорты».
const COHORT_COL = "sticky left-0 w-[132px] min-w-[132px] px-3 text-left";
const SIZE_COL = "sticky md:left-[132px] w-20 min-w-20 px-3 text-right";

// Матрица когорт S-06 (14-ux-ui-spec.md S-06, §16.9): настоящая таблица, 6-шаговая шкала цвета,
// D0 (W0) не окрашивается, ненаступившие периоды пустые. Клик по шапке периода выделяет колонку.
export function RetentionHeatmap({ matrix, granularity, unit = "persons" }: RetentionHeatmapProps) {
  const m = useMessages().retention.heatmap;
  const locale = useLocale();
  const f = useFormat();
  const [selected, setSelected] = useState<number | null>(null);
  const prefix = PREFIX[granularity];
  const columns = Array.from({ length: matrix.periods + 1 }, (_, n) => n);
  const who = (count: number) => plural(locale, unit === "installs" ? m.installs : m.people, count);
  const label = (start: string) =>
    granularity === "week" ? f.formatWeekRange(start) : f.formatDate(start);

  const filled = matrix.cohorts.filter((c) => c.size > 0);
  const meanSize =
    filled.length > 0 ? filled.reduce((sum, c) => sum + c.size, 0) / filled.length : null;

  const valueCell = (value: number | null | undefined, n: number, cohort?: RetentionCohort) => {
    const mark = selected === n ? { "data-selected": "" } : {};
    const outline = selected === n && "outline-2 -outline-offset-2 outline-ring";
    // Ячейки строки «Среднее» (без когорты) закреплены сверху под шапкой.
    const base = cohort ? CELL : cn(CELL, "sticky top-8 z-20 bg-subtle");
    if (value === null || value === undefined) {
      return <td key={n} aria-label={m.noData} className={cn(base, outline)} {...mark} />;
    }
    const text = f.formatPercent(value);
    if (!cohort) {
      return (
        <td key={n} className={cn(base, outline)} {...mark}>
          {text}
        </td>
      );
    }
    const title = fmt(m.cellTitle, {
      cohort: label(cohort.cohort_start),
      size: who(cohort.size),
      period: m.period[granularity],
      n,
      returned: who(Math.round(cohort.size * value)),
      share: text,
    });
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
      aria-label={m.region}
      className="max-h-[640px] overflow-auto rounded-md border border-border outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <table className="border-separate border-spacing-px text-body-sm tabular-nums">
        <caption className="sr-only">{m.caption[granularity]}</caption>
        <thead>
          <tr className="text-label-sm text-fg-muted uppercase">
            <th scope="col" className={cn(COHORT_COL, "top-0 z-40 h-8 bg-subtle font-medium")}>
              {m.cohort}
            </th>
            <th scope="col" className={cn(SIZE_COL, "top-0 z-30 bg-subtle font-medium md:z-40")}>
              {m.size}
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
                  {prefix}
                  {n}
                </button>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          <tr data-average className="font-semibold">
            <th scope="row" className={cn(COHORT_COL, "top-8 z-[25] bg-subtle font-semibold")}>
              {m.average}
            </th>
            <td className={cn(SIZE_COL, "top-8 z-20 bg-subtle md:z-[25]")}>
              {f.formatInt(meanSize)}
            </td>
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
              <td className={cn(SIZE_COL, "bg-card md:z-10")}>{f.formatInt(cohort.size)}</td>
              {columns.map((n) => valueCell(cohort.values[n], n, cohort))}
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
