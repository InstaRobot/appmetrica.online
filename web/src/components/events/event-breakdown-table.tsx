"use client";

import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { EventBreakdown, Unit } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";

/** Допустимые размеры разбивки по возрастанию: «Показать больше» переходит к следующему. */
export const BREAKDOWN_LIMITS = [20, 50, 100] as const;

// Доли в десятых долях процента (сумма — ровно 1000) по методу наибольшего остатка: каждая строка
// округляется вниз, недостающие десятые получают строки с наибольшими остатками (при равенстве — верхние).
// Так значения, «остальные» и «нет свойства» всегда дают 100%, а не 99,9% или 100,1%.
export function sharePermille(counts: readonly number[]): number[] {
  const total = counts.reduce((s, n) => s + n, 0);
  if (total <= 0) return counts.map(() => 0);
  const exact = counts.map((n) => (n * 1000) / total);
  const shares = exact.map(Math.floor);
  let left = 1000 - shares.reduce((s, n) => s + n, 0);
  const order = exact
    .map((x, i) => ({ i, rest: x - Math.floor(x) }))
    .sort((a, b) => b.rest - a.rest || a.i - b.i);
  for (const { i } of order) {
    if (left <= 0) break;
    shares[i] = (shares[i] ?? 0) + 1;
    left -= 1;
  }
  return shares;
}

type Row = {
  key: string;
  /** Значение свойства как отправлено; у служебных строк — подпись из словаря. */
  label: string;
  value: boolean;
  events: number;
  people: number;
};

type EventBreakdownTableProps = {
  eventName: string;
  data: EventBreakdown | undefined;
  unit: Unit;
  limit: number;
  onLimitChange: (limit: number) => void;
  loading?: boolean;
  /** ErrorState вместо таблицы. */
  error?: React.ReactNode;
};

const NUM = "tabular-nums md:text-right";

// Разбивка события по свойству: значения (как отправлены, не переводятся), «остальные» за пределом limit и
// «нет свойства» — события без ключа. Доли считаются от всех событий периода.
export function EventBreakdownTable({
  eventName,
  data,
  unit,
  limit,
  onLimitChange,
  loading = false,
  error,
}: EventBreakdownTableProps) {
  const m = useMessages().events.event.breakdown;
  const f = useFormat();
  const peopleLabel = unit === "installs" ? m.installs : m.people;

  if (error) return error;
  if (loading && !data) return <TableSkeleton rows={6} />;
  if (!data) return null;

  // Присланное пустое значение отличается от события без ключа: у него своя приглушённая подпись.
  const rows: Row[] = data.values.map((v) => ({
    key: `v:${v.value}`,
    label: v.value === "" ? m.emptyValue : v.value,
    value: v.value !== "",
    events: v.events,
    people: v.people,
  }));
  if (data.other) {
    rows.push({ key: "other", label: m.other, value: false, ...data.other });
  }
  if (data.missing.events > 0) {
    rows.push({ key: "missing", label: m.missing, value: false, ...data.missing });
  }
  if (rows.length === 0) {
    return <p className="py-6 text-center text-body-md text-fg-muted">{m.empty}</p>;
  }
  const shares = sharePermille(rows.map((r) => r.events));
  const nextLimit = BREAKDOWN_LIMITS.find((l) => l > limit);

  return (
    <div className="flex flex-col gap-3">
      <Table aria-label={fmt(m.table, { event: eventName, property: data.property })} stackOnMobile>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead scope="col" className="px-3">
              {m.value}
            </TableHead>
            <TableHead scope="col" className="px-3 text-right">
              {m.events}
            </TableHead>
            <TableHead scope="col" className="px-3 text-right">
              {peopleLabel}
            </TableHead>
            <TableHead scope="col" className="w-2/5 px-3">
              {m.share}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row, i) => {
            const share = (shares[i] ?? 0) / 1000;
            return (
              <TableRow key={row.key}>
                <TableCell label={m.value} className="px-3">
                  <span
                    title={row.label}
                    className={cn(
                      "block max-w-48 truncate md:max-w-none",
                      row.value ? "font-mono" : "text-fg-muted",
                    )}
                  >
                    {row.label}
                  </span>
                </TableCell>
                <TableCell label={m.events} className={cn("px-3", NUM)}>
                  {f.formatInt(row.events)}
                </TableCell>
                <TableCell label={peopleLabel} className={cn("px-3", NUM)}>
                  {f.formatInt(row.people)}
                </TableCell>
                <TableCell label={m.share} className="px-3">
                  <span className="flex items-center gap-2">
                    <span
                      aria-hidden="true"
                      className="h-2 flex-1 overflow-hidden rounded-full bg-subtle"
                    >
                      <span
                        className="block h-full rounded-full bg-chart-1"
                        style={{ width: `${share * 100}%` }}
                      />
                    </span>
                    <span className="w-14 text-right tabular-nums">{f.formatPercent(share)}</span>
                  </span>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      {data.other && nextLimit !== undefined ? (
        <Button
          variant="outline"
          size="sm"
          className="self-start"
          onClick={() => onLimitChange(nextLimit)}
        >
          {m.more}
        </Button>
      ) : null}
    </div>
  );
}
