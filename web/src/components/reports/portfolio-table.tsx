"use client";

import { ArrowDown, ArrowUp, ChevronDown } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useId } from "react";
import { PlatformIcons } from "@/components/common/platform-icons";
import { Button } from "@/components/ui/button";
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
import { useFormat, useLocale, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import type { Locale } from "@/lib/i18n/locale";
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

// Сравнение названий по правилам языка интерфейса: в русском кириллица идёт раньше латиницы.
const collators = new Map<Locale, Intl.Collator>();

function nameCollator(locale: Locale): Intl.Collator {
  let collator = collators.get(locale);
  if (!collator) {
    collator = new Intl.Collator(locale, { sensitivity: "base", numeric: true });
    collators.set(locale, collator);
  }
  return collator;
}

function metric(row: PortfolioRow, key: Exclude<PortfolioSortKey, "name">): number | null {
  return row[key];
}

// sortPortfolioRows: «—» (null) всегда в конце независимо от направления, при равенстве — по названию.
export function sortPortfolioRows(
  rows: PortfolioRow[],
  sort: PortfolioSort,
  locale: Locale,
): PortfolioRow[] {
  const dir = sort.desc ? -1 : 1;
  const collator = nameCollator(locale);
  return rows.toSorted((a, b) => {
    const byName = collator.compare(a.app.name, b.app.name);
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

// Новая колонка: числа — по убыванию, название — по алфавиту; та же колонка — смена направления.
function nextSort(sort: PortfolioSort, key: PortfolioSortKey): PortfolioSort {
  return sort.key === key ? { key, desc: !sort.desc } : { key, desc: key !== "name" };
}

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

  // На телефоне шапка только для скринридеров (сортирует SortPicker): невидимая кнопка не ловит фокус.
  const button = (
    <button
      type="button"
      onClick={() => onSortChange(nextSort(sort, column.key))}
      className={cn(
        "inline-flex items-center gap-1 rounded-sm text-label-sm uppercase tracking-wide outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring max-md:invisible",
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

// Сортировка на телефоне, где заголовков колонок не видно: поле — нативный select, направление — кнопка.
function SortPicker({
  columns,
  sort,
  onSortChange,
}: {
  columns: Column[];
  sort: PortfolioSort;
  onSortChange: (sort: PortfolioSort) => void;
}) {
  const m = useMessages().portfolio.table.sort;
  const id = useId();
  return (
    <div className="flex items-end gap-2 border-b p-3 md:hidden">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <label htmlFor={id} className="text-caption text-fg-muted">
          {m.label}
        </label>
        <div className="relative">
          {/* 16 px: iOS не увеличивает страницу при фокусе на поле. */}
          <select
            id={id}
            value={sort.key}
            onChange={(e) => onSortChange(nextSort(sort, e.target.value as PortfolioSortKey))}
            className="h-11 w-full appearance-none rounded-lg border border-input bg-background pr-9 pl-3 text-lg outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
          >
            {columns.filter(isSortable).map((column) => (
              <option key={column.key} value={column.key}>
                {column.label}
              </option>
            ))}
          </select>
          <ChevronDown
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-fg-muted"
          />
        </div>
      </div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-11"
        aria-label={m.descending}
        aria-pressed={sort.desc}
        onClick={() => onSortChange({ key: sort.key, desc: !sort.desc })}
      >
        {sort.desc ? <ArrowDown aria-hidden="true" /> : <ArrowUp aria-hidden="true" />}
      </Button>
    </div>
  );
}

// Порядок ячеек в карточке телефона: DAU с дельтой и «сегодня» первыми, доли платформ — последней строкой.
const CARD_ORDER: Record<Column["key"], string> = {
  name: "",
  active_date: "max-md:order-1",
  active_delta_pct: "max-md:order-2",
  active_today: "max-md:order-3",
  new_date: "max-md:order-4",
  retention_d1: "max-md:order-5",
  retention_d7: "max-md:order-6",
  sessions_date: "max-md:order-7",
  sparkline: "max-md:order-8 max-md:col-span-2",
  platforms: "max-md:order-9 max-md:col-span-full",
};

// PortfolioTable (14-ux-ui-spec.md S-03, §16.4): все приложения за дату, сортировка на клиенте.
// На телефоне строка — карточка (Table stackOnMobile), сортировка — SortPicker над списком.
export function PortfolioTable({ rows, date, sort, onSortChange, onOpenApp }: PortfolioTableProps) {
  const locale = useLocale();
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
    { key: "new_date", label: fmt(m.columns.new, { date: f.formatDateShort(date) }) },
    { key: "sessions_date", label: fmt(m.columns.sessions, { date: f.formatDateShort(date) }) },
    { key: "retention_d1", label: "D1" },
    { key: "retention_d7", label: "D7" },
    { key: "sparkline", label: m.columns.sparkline },
  ];
  const sorted = sortPortfolioRows(rows, sort, locale);
  const label = (key: Column["key"]) => columns.find((c) => c.key === key)?.label ?? "";
  const cell = (key: Column["key"], className?: string) => ({
    label: label(key),
    className: cn("px-3 text-right", CARD_ORDER[key], className),
  });

  return (
    <TooltipProvider>
      {/* Одна обёртка: отступ между дочерними элементами Card не разрывает выбор сортировки и список. */}
      <div>
        <SortPicker columns={columns} sort={sort} onSortChange={onSortChange} />
        <Table stackOnMobile>
          <TableCaption className="sr-only">
            {fmt(m.caption, { date: f.formatDate(date) })}
          </TableCaption>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {columns.map((column) => (
                <TableHead
                  key={column.key}
                  scope="col"
                  // Имя колонки не зависит от кнопки сортировки, которая на телефоне невидима.
                  aria-label={isSortable(column) ? column.label : undefined}
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
          <TableBody className="max-md:p-3">
            {sorted.map((row) => (
              <TableRow
                key={row.app.id}
                className="h-13 cursor-pointer max-md:h-auto max-md:grid-cols-3"
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
                <TableCell {...cell("platforms")}>
                  <PlatformShareBar items={row.platforms} total={row.active_date} />
                </TableCell>
                <TableCell {...cell("active_today", "tabular-nums text-fg-muted")}>
                  {f.formatInt(row.active_today)}
                </TableCell>
                <TableCell {...cell("active_date", "font-medium tabular-nums")}>
                  {f.formatInt(row.active_date)}
                </TableCell>
                <TableCell {...cell("active_delta_pct")}>
                  <Delta value={row.active_delta_pct} />
                </TableCell>
                <TableCell {...cell("new_date", "tabular-nums")}>
                  {f.formatInt(row.new_date)}
                </TableCell>
                <TableCell {...cell("sessions_date", "tabular-nums")}>
                  {f.formatInt(row.sessions_date)}
                </TableCell>
                <TableCell {...cell("retention_d1", "tabular-nums")}>
                  {f.formatPercent(row.retention_d1)}
                </TableCell>
                <TableCell {...cell("retention_d7", "tabular-nums")}>
                  {f.formatPercent(row.retention_d7)}
                </TableCell>
                <TableCell {...cell("sparkline")}>
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
      </div>
    </TooltipProvider>
  );
}
