"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { ArrowLeft, SearchX } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import { useState } from "react";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { reportQuery } from "@/components/layout/app-tabs";
import { Delta } from "@/components/reports/delta";
import { GranularitySelect } from "@/components/reports/granularity-select";
import { MetricChart } from "@/components/reports/lazy-charts";
import { STRETCH_GROUP, TOUCH_TOGGLE } from "@/components/reports/responsive";
import { EventSkeleton } from "@/components/skeletons/event-skeleton";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetEventBreakdown, useGetEventReport } from "@/lib/api/gen/endpoints/events/events";
import type { App, EventReport } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { allowedGranularities, todayIn } from "@/lib/period";
import { reportParams, useEventsState } from "@/lib/url-state";
import { cn } from "@/lib/utils";
import { EventBreakdownTable } from "./event-breakdown-table";
import { EventsToolbar } from "./events-toolbar";
import { PropertySelect } from "./property-select";

type ChartSeries = "events" | "people";

// Страница события (FR-102). Приложение (TZ) берётся из кэша AppShell.
export function EventView() {
  const { appId } = useParams<{ appId: string }>();
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <EventPage app={app.data.data} /> : <EventSkeleton />;
}

// Изменение к предыдущему периоду в процентах; без значения в прошлом сравнивать не с чем.
function deltaPct(value: number, prev: number | undefined): number | null {
  return prev === undefined || prev === 0 ? null : ((value - prev) / prev) * 100;
}

function EventKpis({
  summary,
  installs,
}: {
  summary: EventReport["summary"] | undefined;
  installs: boolean;
}) {
  const m = useMessages().events.event;
  const f = useFormat();
  const cards = [
    {
      key: "events",
      label: m.kpi.events,
      value: summary ? f.formatInt(summary.events) : null,
      delta: summary ? deltaPct(summary.events, summary.prev_events) : null,
    },
    {
      key: "people",
      label: installs ? m.kpi.installs : m.kpi.people,
      value: summary ? f.formatInt(summary.people) : null,
      delta: summary ? deltaPct(summary.people, summary.prev_people) : null,
    },
    {
      key: "perPerson",
      label: m.kpi.perPerson,
      title: m.kpi.perPersonHint,
      value: summary ? f.formatDecimal(summary.events_per_person) : null,
      delta: summary ? deltaPct(summary.events_per_person, summary.prev_events_per_person) : null,
    },
  ];
  return (
    <section aria-label={m.totals} className="grid grid-cols-1 gap-3 sm:grid-cols-3 md:gap-4">
      {cards.map((card) => (
        <div
          key={card.key}
          title={card.title}
          className="flex min-h-27 flex-col items-start gap-1.5 rounded-xl border border-border bg-card p-4 shadow-xs md:p-5"
        >
          <span className="text-label-md text-fg-muted">{card.label}</span>
          {card.value === null ? (
            <Skeleton className="h-7 w-30" />
          ) : (
            <>
              <span className="text-heading-2 tabular-nums md:text-heading-1">{card.value}</span>
              <Delta value={card.delta} />
            </>
          )}
        </div>
      ))}
    </section>
  );
}

function EventPage({ app }: { app: App }) {
  const { eventName } = useParams<{ eventName: string }>();
  const messages = useMessages();
  const m = messages.events.event;
  const search = useSearchParams();
  const today = todayIn(app.timezone);
  const { state, update } = useEventsState(app.timezone, today);
  const params = reportParams(state);
  const query = reportQuery(search);
  const listHref = `/apps/${app.id}/events${query}`;
  const [series, setSeries] = useState<ChartSeries>("events");

  const report = useGetEventReport(
    app.id,
    eventName,
    { ...params, granularity: state.granularity, compare: "previous_period" },
    { query: { placeholderData: keepPreviousData } },
  );
  const data = report.data?.status === 200 ? report.data.data : undefined;
  // Свойство по умолчанию — первое из property_keys; в URL оно попадает только после выбора пользователя.
  const property = state.property ?? data?.property_keys[0]?.key;
  const breakdown = useGetEventBreakdown(
    app.id,
    eventName,
    { ...params, property: property ?? "", limit: state.limit },
    {
      query: {
        enabled: data !== undefined && property !== undefined,
        placeholderData: keepPreviousData,
      },
    },
  );
  const breakdownData = breakdown.data?.status === 200 ? breakdown.data.data : undefined;

  // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
  const error = report.isError ? (report.error as unknown as ApiError | null) : null;
  const rangeError =
    error?.code === "range_too_large" ? messages.errors.codes.range_too_large : undefined;
  const notFound = error?.code === "event_not_found";
  const breakdownError = breakdown.isError ? (breakdown.error as unknown as ApiError | null) : null;
  const installs = state.unit === "installs";
  const peopleLabel = installs ? m.chart.installs : m.chart.people;
  const seriesLabel = series === "events" ? m.chart.events : peopleLabel;

  let content: React.ReactNode;
  if (notFound) {
    content = (
      <Card className="gap-0 px-0 py-0">
        <EmptyState
          icon={SearchX}
          title={m.notFound.title}
          description={m.notFound.description}
          action={{ label: m.notFound.action, href: listHref }}
        />
      </Card>
    );
  } else if (error && !rangeError) {
    content = <ErrorState error={error} onRetry={() => void report.refetch()} />;
  } else {
    content = (
      <>
        <EventKpis summary={data?.summary} installs={installs} />
        <Card className="gap-4 px-4 md:px-6">
          <div className="w-full overflow-x-auto md:w-auto md:overflow-visible">
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              spacing={0}
              aria-label={m.chart.metric}
              className={STRETCH_GROUP}
              value={series}
              onValueChange={(next) => {
                if (next === "events" || next === "people") setSeries(next);
              }}
            >
              <ToggleGroupItem
                value="events"
                className={cn("flex-auto md:flex-none", TOUCH_TOGGLE, "px-3")}
              >
                {m.chart.events}
              </ToggleGroupItem>
              <ToggleGroupItem
                value="people"
                className={cn("flex-auto md:flex-none", TOUCH_TOGGLE, "px-3")}
              >
                {peopleLabel}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>
          {data ? (
            <MetricChart
              series={[
                {
                  key: "total",
                  label: seriesLabel,
                  points: data.timeseries.map((p) => ({ ts: p.bucket, value: p[series] })),
                },
              ]}
              granularity={state.granularity}
              metric="sessions"
              timezone={app.timezone}
              labels={{ title: seriesLabel, series: seriesLabel }}
            />
          ) : report.isPending ? (
            <Skeleton
              role="status"
              aria-label={messages.common.loading}
              className="h-55 w-full md:h-90"
            />
          ) : null}
        </Card>
        <Card className="gap-4 px-4 md:px-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-heading-3">{m.breakdown.title}</h2>
            {data && property !== undefined ? (
              <PropertySelect
                keys={data.property_keys}
                value={property}
                onChange={(next) => void update({ property: next })}
              />
            ) : null}
          </div>
          {data && property === undefined ? (
            <p className="py-6 text-center text-body-md text-fg-muted">
              {m.breakdown.noProperties}
            </p>
          ) : (
            <EventBreakdownTable
              eventName={eventName}
              data={breakdownData}
              unit={state.unit}
              limit={state.limit}
              onLimitChange={(limit) => void update({ limit })}
              loading={data === undefined || breakdown.isPending}
              error={
                breakdownError && breakdownError.code !== "range_too_large" ? (
                  <ErrorState error={breakdownError} onRetry={() => void breakdown.refetch()} />
                ) : undefined
              }
            />
          )}
        </Card>
      </>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <Link
          href={listHref as Route}
          className="inline-flex items-center gap-1 self-start rounded-sm text-body-sm text-fg-link hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <ArrowLeft aria-hidden="true" className="size-3.5" />
          {m.back}
        </Link>
        <h1 title={eventName} className="truncate font-mono text-heading-2">
          {eventName}
        </h1>
      </div>
      <EventsToolbar app={app} state={state} update={update} rangeError={rangeError}>
        <GranularitySelect
          value={state.granularity}
          allowed={allowedGranularities(state.period)}
          onChange={(g) => void update({ g })}
        />
      </EventsToolbar>
      {content}
    </div>
  );
}
