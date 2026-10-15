"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { CalendarDays, ChevronLeft, ChevronRight, Info, LayoutGrid, Plus } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { createParser, parseAsStringLiteral, useQueryStates } from "nuqs";
import { useId, useState } from "react";
import { LazyCalendar } from "@/components/common/lazy-calendar";
import { usePlatformLabel } from "@/components/common/platform-icon";
import { PlatformIcons } from "@/components/common/platform-icons";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { EnvironmentSwitch } from "@/components/reports/environment-switch";
import {
  DEFAULT_PORTFOLIO_SORT,
  PORTFOLIO_SORT_KEYS,
  type PortfolioSort,
  type PortfolioSortKey,
  PortfolioTable,
} from "@/components/reports/portfolio-table";
import { UnitBadge } from "@/components/reports/unit-badge";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetSettings } from "@/lib/api/gen/endpoints/admin-settings/admin-settings";
import { useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetPortfolio } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import { type App, Environment, Unit } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { addDays, isIsoDate, yesterdayIn } from "@/lib/period";

// URL-состояние S-03: ?date=YYYY-MM-DD (по умолчанию — вчера по settings.owner_timezone, решает сервер),
// ?env=, скрытый ?unit=installs и ?sort=-active_date (минус — по убыванию).
const dateParser = createParser({
  parse: (value: string) => (isIsoDate(value) ? value : null),
  serialize: (value: string) => value,
});

const sortParser = createParser<PortfolioSort>({
  parse: (value) => {
    const desc = value.startsWith("-");
    const key = (desc ? value.slice(1) : value) as PortfolioSortKey;
    return PORTFOLIO_SORT_KEYS.includes(key) ? { key, desc } : null;
  },
  serialize: (sort) => `${sort.desc ? "-" : ""}${sort.key}`,
  eq: (a, b) => a.key === b.key && a.desc === b.desc,
});

const searchParams = {
  date: dateParser,
  env: parseAsStringLiteral(Object.values(Environment)).withDefault(Environment.production),
  unit: parseAsStringLiteral(Object.values(Unit)).withDefault(Unit.persons),
  sort: sortParser.withDefault(DEFAULT_PORTFOLIO_SORT),
};

// Местная полночь для react-day-picker ↔ YYYY-MM-DD: календарь работает с датами без зоны.
function toPickerDate(date: string): Date {
  const [y, m, d] = date.split("-").map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

function fromPickerDate(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

type DateStepperProps = { value: string; max: string; onChange: (date: string) => void };

// ◀ дата ▶ (S-03): ±1 день, вперёд не дальше вчера; клик по дате — календарь.
function DateStepper({ value, max, onChange }: DateStepperProps) {
  const m = useMessages().portfolio.date;
  const [open, setOpen] = useState(false);
  const label = useFormat().formatDate(value);
  return (
    <div className="flex items-center gap-1">
      <Button
        variant="outline"
        size="icon"
        aria-label={m.previous}
        onClick={() => onChange(addDays(value, -1))}
      >
        <ChevronLeft aria-hidden="true" />
      </Button>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button variant="outline" aria-label={fmt(m.pick, { date: label })} className="min-w-36">
            <CalendarDays aria-hidden="true" />
            {label}
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-auto p-0">
          <LazyCalendar
            mode="single"
            selected={toPickerDate(value)}
            defaultMonth={toPickerDate(value)}
            disabled={{ after: toPickerDate(max) }}
            onSelect={(day) => {
              if (!day) return;
              onChange(fromPickerDate(day));
              setOpen(false);
            }}
          />
        </PopoverContent>
      </Popover>
      <Button
        variant="outline"
        size="icon"
        aria-label={m.next}
        disabled={value >= max}
        onClick={() => onChange(addDays(value, 1))}
      >
        <ChevronRight aria-hidden="true" />
      </Button>
    </div>
  );
}

function ArchivedApps({ apps, timeZone }: { apps: App[]; timeZone: string }) {
  const platformLabel = usePlatformLabel();
  const m = useMessages().portfolio.archive;
  const f = useFormat();
  const headingId = useId();
  return (
    <section className="mt-6 flex flex-col gap-3">
      <h2 id={headingId} className="text-heading-3">
        {m.title}
      </h2>
      <Card className="py-0">
        <Table aria-labelledby={headingId}>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead scope="col" className="px-3">
                {m.name}
              </TableHead>
              <TableHead scope="col" className="px-3">
                {m.platforms}
              </TableHead>
              <TableHead scope="col" className="px-3">
                {m.archivedAt}
              </TableHead>
              <TableHead scope="col" className="px-3">
                <span className="sr-only">{m.actions}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {apps.map((app) => (
              <TableRow key={app.id}>
                <TableCell className="px-3">
                  <span className="inline-flex items-center gap-2">
                    <PlatformIcons platforms={app.platforms} className="text-fg-muted" />
                    {app.name}
                  </span>
                </TableCell>
                <TableCell className="px-3">
                  {app.platforms.map(platformLabel).join(", ")}
                </TableCell>
                <TableCell className="px-3">
                  {app.archived_at ? f.formatDate(app.archived_at, timeZone) : "—"}
                </TableCell>
                <TableCell className="px-3 text-right">
                  <Link href={`/apps/${app.id}` as Route} className="text-fg-link hover:underline">
                    {m.open}
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </section>
  );
}

// S-03 Портфель (FR-012): таблица всех активных приложений за дату и четыре состояния экрана.
export function PortfolioView() {
  const m = useMessages().portfolio;
  const f = useFormat();
  const router = useRouter();
  const [{ date, env, unit, sort }, setState] = useQueryStates(searchParams);
  const [showArchived, setShowArchived] = useState(false);
  const archiveSwitchId = useId();

  const settings = useGetSettings();
  const ownerTimezone =
    settings.data?.status === 200
      ? settings.data.data.owner_timezone
      : Intl.DateTimeFormat().resolvedOptions().timeZone;

  const portfolio = useGetPortfolio(
    { environment: env, unit, ...(date ? { date } : {}) },
    { query: { placeholderData: keepPreviousData } },
  );
  const report = portfolio.data?.status === 200 ? portfolio.data.data : undefined;

  const apps = useListApps({ include_archived: true });
  const archived =
    apps.data?.status === 200
      ? apps.data.data.items.filter((app) => app.status === "archived")
      : [];

  const maxDate = yesterdayIn(ownerTimezone);
  const current = date ?? report?.date ?? maxDate;
  const noEvents =
    report !== undefined &&
    report.rows.length > 0 &&
    report.rows.every((r) => r.active_date === 0 && r.new_date === 0 && r.sessions_date === 0);

  return (
    <>
      <PageHeader
        title={m.title}
        adornment={<UnitBadge unit={unit} />}
        actions={
          <>
            <EnvironmentSwitch value={env} onChange={(next) => void setState({ env: next })} />
            <DateStepper
              value={current}
              max={maxDate}
              onChange={(next) => void setState({ date: next })}
            />
            <Button asChild>
              <Link href="/apps/new">
                <Plus aria-hidden="true" />
                {m.addApp}
              </Link>
            </Button>
          </>
        }
      />

      {noEvents ? (
        <Alert className="mb-4">
          <Info aria-hidden="true" />
          <AlertDescription>
            {fmt(m.noEvents, { date: f.formatDate(report.date) })}
          </AlertDescription>
        </Alert>
      ) : null}

      {report && report.rows.length === 0 ? (
        <Card>
          <EmptyState
            icon={LayoutGrid}
            title={m.empty.title}
            description={m.empty.description}
            action={{ label: m.addApp, href: "/apps/new" }}
          />
        </Card>
      ) : (
        <Card className="py-0">
          {portfolio.isError && !report ? (
            <div className="p-4">
              <ErrorState
                // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
                error={portfolio.error as unknown as ApiError}
                onRetry={() => void portfolio.refetch()}
              />
            </div>
          ) : report ? (
            <PortfolioTable
              rows={report.rows}
              date={report.date}
              sort={sort}
              onSortChange={(next) => void setState({ sort: next })}
              onOpenApp={(id) => router.push(`/apps/${id}` as Route)}
            />
          ) : (
            <TableSkeleton rows={4} rowHeight="portfolio" />
          )}
        </Card>
      )}

      {archived.length > 0 ? (
        <div className="mt-4 flex items-center gap-2">
          <Switch id={archiveSwitchId} checked={showArchived} onCheckedChange={setShowArchived} />
          <Label htmlFor={archiveSwitchId}>{fmt(m.showArchived, { count: archived.length })}</Label>
        </div>
      ) : null}
      {showArchived && archived.length > 0 ? (
        <ArchivedApps apps={archived} timeZone={ownerTimezone} />
      ) : null}
    </>
  );
}
