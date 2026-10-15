"use client";

import { ArrowDown, ArrowUp } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { PlatformIcons } from "@/components/common/platform-icons";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { PortfolioRow } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { addDays } from "@/lib/period";
import { cn } from "@/lib/utils";
import type { Messages } from "@/messages";
import { Delta } from "./delta";
import { PlatformShareBar } from "./platform-share-bar";
import { Sparkline } from "./sparkline";

export type PortfolioSortKey =
  | "name"
  | "active_today"
  | "active_date"
  | "active_delta_pct"
  | "new_date"
  | "sessions_date"
  | "retention_d1"
  | "retention_d7";
export type PortfolioSort = { key: PortfolioSortKey; desc: boolean };

export const PORTFOLIO_SORT_KEYS: readonly PortfolioSortKey[] = [
  "name",
  "active_today",
  "active_date",
  "active_delta_pct",
  "new_date",
  "sessions_date",
  "retention_d1",
  "retention_d7",
];
export const DEFAULT_PORTFOLIO_SORT: PortfolioSort = { key: "active_date", desc: true };

type PortfolioTableProps = {
  rows: PortfolioRow[];
  /** Дата отчёта YYYY-MM-DD — для заголовков колонок. */
  date: string;
  sort: PortfolioSort;
  onSortChange: (sort: PortfolioSort) => void;
  /** Клик мышью по строке; с клавиатуры приложение открывается ссылкой в первой ячейке. */
  onOpenApp?: (appId: string) => void;
};

const nameCollator = new Intl.Collator("ru", { sensitivity: "base", numeric: true });

function metric(row: PortfolioRow, key: Exclude<PortfolioSortKey, "name">): number | null {
  return row[key];
}

// sortPortfolioRows: «—» (null) всегда в конце независимо от направления, при равенстве — по названию.
export function sortPortfolioRows(rows: PortfolioRow[], sort: PortfolioSort): PortfolioRow[] {
  const dir = sort.desc ? -1 : 1;
  return rows.toSorted((a, b) => {
    const byName = nameCollator.compare(a.app.name, b.app.name);
    if (sort.key === "name") return dir * byName;
    const x = metric(a, sort.key);
    const y = metric(b, sort.key);
    if (x === null || y === null) {
      if (x === y) return byName;
      return x === null ? 1 : -1;
    }
    return x === y ? byName : dir * (x - y);
  });
}

// Тренд 30 дней для скринридеров: средняя первой недели против последней, порог ±5 %.
export function trendLabel(
  values: number[],
  trend: Messages["portfolio"]["table"]["trend"],
): string {
  const avg = (xs: number[]) => (xs.length ? xs.reduce((s, x) => s + x, 0) / xs.length : 0);
  const first = avg(values.slice(0, 7));
  const last = avg(values.slice(-7));
  const change = first === 0 ? (last === 0 ? 0 : 1) : (last - first) / first;
  return change > 0.05 ? trend.up : change < -0.05 ? trend.down : trend.flat;
}

type Column = {
  key: PortfolioSortKey | "platforms" | "sparkline";
  label: string;
  hint?: string;
};

const isSortable = (column: Column): column is Column & { key: PortfolioSortKey } =>
  column.key !== "platforms" && column.key !== "sparkline";

function SortButton({
  column,
  sort,
  onSortChange,
}: {
  column: Column & { key: PortfolioSortKey };
  sort: PortfolioSort;
  onSortChange: (sort: PortfolioSort) => void;
}) {
  const active = sort.key === column.key;
  const Arrow = sort.desc ? ArrowDown : ArrowUp;
  // Новая колонка: числа — по убыванию, название — по алфавиту; повторный клик меняет направление.
  const next = (): PortfolioSort =>
    active
      ? { key: column.key, desc: !sort.desc }
      : { key: column.key, desc: column.key !== "name" };

  const button = (
    <button
      type="button"
      onClick={() => onSortChange(next())}
      className={cn(
        "inline-flex items-center gap-1 rounded-sm text-label-sm uppercase tracking-wide outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
        active ? "text-foreground" : "text-fg-muted",
      )}
    >
      {column.label}
      {active ? <Arrow aria-hidden="true" className="size-3.5" /> : null}
    </button>
  );
  if (!column.hint) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>{column.hint}</TooltipContent>
    </Tooltip>
  );
}

// PortfolioTable (14-ux-ui-spec.md S-03, §16.4): все приложения за дату, сортировка на клиенте.
export function PortfolioTable({ rows, date, sort, onSortChange, onOpenApp }: PortfolioTableProps) {
  const m = useMessages().portfolio.table;
  const f = useFormat();
  const columns: Column[] = [
    { key: "name", label: m.columns.name },
    { key: "platforms", label: m.columns.platforms },
    { key: "active_today", label: m.columns.today, hint: m.columns.todayHint },
    { key: "active_date", label: fmt(m.columns.dau, { date: f.formatDateShort(date) }) },
    {
      key: "active_delta_pct",
      label: fmt(m.columns.delta, { date: f.formatWeekdayDate(addDays(date, -7)) }),
    },
    { key: "new_date", label: m.columns.new },
    { key: "sessions_date", label: m.columns.sessions },
    { key: "retention_d1", label: "D1" },
    { key: "retention_d7", label: "D7" },
    { key: "sparkline", label: m.columns.sparkline },
  ];
  const sorted = sortPortfolioRows(rows, sort);

  return (
    <TooltipProvider>
      <Table>
        <TableCaption className="sr-only">
          {fmt(m.caption, { date: f.formatDate(date) })}
        </TableCaption>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {columns.map((column) => (
              <TableHead
                key={column.key}
                scope="col"
                aria-sort={
                  !isSortable(column)
                    ? undefined
                    : sort.key === column.key
                      ? sort.desc
                        ? "descending"
                        : "ascending"
                      : "none"
                }
                className={cn("px-3", column.key !== "name" && "text-right")}
              >
                {!isSortable(column) ? (
                  <span className="text-label-sm uppercase tracking-wide text-fg-muted">
                    {column.label}
                  </span>
                ) : (
                  <SortButton column={column} sort={sort} onSortChange={onSortChange} />
                )}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((row) => (
            <TableRow
              key={row.app.id}
              className="h-13 cursor-pointer"
              onClick={(event) => {
                if ((event.target as HTMLElement).closest("a")) return;
                onOpenApp?.(row.app.id);
              }}
            >
              <TableCell className="px-3">
                <Link
                  href={`/apps/${row.app.id}` as Route}
                  className="inline-flex items-center gap-2 rounded-sm font-medium text-foreground outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <PlatformIcons platforms={row.app.platforms} className="text-fg-muted" />
                  {row.app.name}
                </Link>
              </TableCell>
              <TableCell className="px-3 text-right">
                <PlatformShareBar items={row.platforms} total={row.active_date} />
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums text-fg-muted">
                {f.formatInt(row.active_today)}
              </TableCell>
              <TableCell className="px-3 text-right font-medium tabular-nums">
                {f.formatInt(row.active_date)}
              </TableCell>
              <TableCell className="px-3 text-right">
                <Delta value={row.active_delta_pct} />
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums">
                {f.formatInt(row.new_date)}
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums">
                {f.formatInt(row.sessions_date)}
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums">
                {f.formatPercent(row.retention_d1)}
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums">
                {f.formatPercent(row.retention_d7)}
              </TableCell>
              <TableCell className="px-3 text-right">
                <Sparkline
                  values={row.sparkline.map((p) => p.active)}
                  trendLabel={trendLabel(
                    row.sparkline.map((p) => p.active),
                    m.trend,
                  )}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TooltipProvider>
  );
}
