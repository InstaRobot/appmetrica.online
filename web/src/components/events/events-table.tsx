"use client";

import type { Route } from "next";
import Link from "next/link";
import { Delta } from "@/components/reports/delta";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { EventStat, Unit } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type EventsTableProps = {
  appId: string;
  events: readonly EventStat[];
  unit: Unit;
  /** Параметры отчёта текущего URL (`?period=…`): переходят на страницу события как есть. */
  query: string;
};

// Изменение числа событий к прошлому периоду в процентах; без событий в прошлом сравнивать не с чем.
export function eventsDeltaPct(stat: EventStat): number | null {
  const prev = stat.prev_events;
  return prev === undefined || prev === 0 ? null : ((stat.events - prev) / prev) * 100;
}

// События по убыванию числа, при равенстве — по имени.
export function sortEvents(events: readonly EventStat[]): EventStat[] {
  return events.toSorted((a, b) => b.events - a.events || (a.name < b.name ? -1 : 1));
}

const NUM = "tabular-nums md:text-right";

// Список событий приложения в стиле breakdown-table: имя (моноширинное, как отправлено), события, люди,
// изменение к прошлому периоду и событий на человека. На телефоне строки — карточки.
export function EventsTable({ appId, events, unit, query }: EventsTableProps) {
  const m = useMessages().events.table;
  const f = useFormat();

  return (
    <Table aria-label={m.label} stackOnMobile>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead scope="col" className="px-3">
            {m.name}
          </TableHead>
          <TableHead scope="col" className="px-3 text-right">
            {m.events}
          </TableHead>
          <TableHead scope="col" className="px-3 text-right">
            {unit === "installs" ? m.installs : m.people}
          </TableHead>
          <TableHead scope="col" title={m.deltaHint} className="px-3 text-right">
            {m.delta}
          </TableHead>
          <TableHead scope="col" title={m.perPersonHint} className="px-3 text-right">
            {m.perPerson}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sortEvents(events).map((stat) => (
          <TableRow key={stat.name}>
            <TableCell className="px-3">
              <Link
                href={`/apps/${appId}/events/${encodeURIComponent(stat.name)}${query}` as Route}
                title={stat.name}
                className={cn(
                  "block max-w-full truncate rounded-sm font-mono text-fg-link hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                )}
              >
                {stat.name}
              </Link>
            </TableCell>
            <TableCell label={m.events} className={cn("px-3", NUM)}>
              {f.formatInt(stat.events)}
            </TableCell>
            <TableCell
              label={unit === "installs" ? m.installs : m.people}
              className={cn("px-3", NUM)}
            >
              {f.formatInt(stat.people)}
            </TableCell>
            <TableCell label={m.delta} className="px-3 md:text-right">
              <Delta value={eventsDeltaPct(stat)} />
            </TableCell>
            <TableCell label={m.perPerson} className={cn("px-3", NUM)}>
              {stat.people === 0 ? f.formatDecimal(0) : f.formatDecimal(stat.events / stat.people)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
