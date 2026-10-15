"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { lazy, Suspense, useId, useState } from "react";
import { ErrorState } from "@/components/feedback/error-state";
import { HourlyIngestChart } from "@/components/reports/lazy-charts";
import { IntegrationSkeleton } from "@/components/skeletons/integration-skeleton";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetIntegrationHealth } from "@/lib/api/gen/endpoints/integration/integration";
import type { App, IntegrationHealth } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { ingestEndpoint } from "@/lib/snippets";
import { cn } from "@/lib/utils";
import { isRejectionRateHigh, RejectionRateWarning } from "./rejection-rate-warning";
import { RejectionReasonsTable } from "./rejection-reasons-table";
import { VersionsHealthTable } from "./versions-health-table";
import { WaitingForFirstEvent } from "./waiting-for-first-event";

// Опрос getIntegrationHealth(hours=24): раз в 3 с до первого события, затем раз в 30 с (S-10).
const HOURS = 24;
const WAITING_POLL_MS = 3000;
const HEALTHY_POLL_MS = 30_000;

// Онбординг (ключи, сниппеты, статус платформ) — отдельный чанк: с секциями по платформам initial JS
// /apps/[appId]/integration выходил за бюджет 250 KB gz.
const LazyIntegrationOnboarding = lazy(() =>
  import("./integration-onboarding").then((m) => ({ default: m.IntegrationOnboarding })),
);

function IntegrationOnboarding(props: { app: App; ingestUrl: string }) {
  const m = useMessages();
  return (
    <Suspense
      fallback={
        <Card role="status" aria-label={m.common.loading} className="gap-3 px-6">
          <Skeleton className="h-5 w-48" />
          <Skeleton className="h-8 w-full" />
        </Card>
      }
    >
      <LazyIntegrationOnboarding {...props} />
    </Suspense>
  );
}

type IntegrationPanelProps = { appId: string };

// S-10 Интеграция (FR-017, FR-7). Приложение (платформы, TZ) берётся из кэша AppShell.
export function IntegrationPanel({ appId }: IntegrationPanelProps) {
  const app = useGetApp(appId);
  return app.data?.status === 200 ? (
    <IntegrationReport app={app.data.data} />
  ) : (
    <IntegrationSkeleton />
  );
}

function IntegrationReport({ app }: { app: App }) {
  const m = useMessages().integration;
  const ingestUrl = ingestEndpoint();
  const health = useGetIntegrationHealth(
    app.id,
    { hours: HOURS },
    {
      query: {
        refetchInterval: (query) => {
          const res = query.state.data;
          return res?.status === 200 && res.data.first_event_at !== null
            ? HEALTHY_POLL_MS
            : WAITING_POLL_MS;
        },
        staleTime: 0,
        placeholderData: keepPreviousData,
      },
    },
  );
  const data = health.data?.status === 200 ? health.data.data : undefined;

  // Режим фиксируется по первому ответу: событие, пришедшее при открытой странице, показывается
  // зелёным WaitingForFirstEvent в онбординге, режим здоровья — со следующего открытия.
  const [onboarding, setOnboarding] = useState<boolean | null>(null);
  if (onboarding === null && data) {
    setOnboarding(data.first_event_at === null);
  }

  if (!data || onboarding === null) {
    if (health.isError) {
      return (
        <Card className="px-6">
          <ErrorState
            error={health.error as unknown as ApiError}
            onRetry={() => void health.refetch()}
          />
        </Card>
      );
    }
    return <IntegrationSkeleton />;
  }

  if (onboarding) {
    return (
      <div className="flex flex-col gap-6">
        <h2 className="text-heading-2">{m.title}</h2>
        <IntegrationOnboarding app={app} ingestUrl={ingestUrl} />
        <WaitingForFirstEvent
          firstEventAt={data.first_event_at}
          appName={app.name}
          audienceHref={`/apps/${app.id}`}
          timezone={app.timezone}
        />
      </div>
    );
  }

  return <IntegrationHealthView app={app} data={data} ingestUrl={ingestUrl} />;
}

function HealthKpi({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: KPI-карточка, а не набор полей формы (fieldset)
    <div
      role="group"
      aria-label={label}
      className="flex flex-col gap-1.5 rounded-2xl border border-border bg-card p-5 shadow-(--shadow-card)"
    >
      <span className="text-label-md text-fg-muted">{label}</span>
      <span className="flex items-center gap-2 text-heading-1 tabular-nums">{children}</span>
    </div>
  );
}

type IntegrationHealthViewProps = { app: App; data: IntegrationHealth; ingestUrl: string };

function IntegrationHealthView({ app, data, ingestUrl }: IntegrationHealthViewProps) {
  const m = useMessages().integration;
  const f = useFormat();
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const high = isRejectionRateHigh(data.rejected_total, data.accepted_total);
  // «Последнее событие» (макет «Dashboard v2»): отдельного поля в IntegrationHealth нет — берём самый
  // свежий last_seen среди версий приложения.
  const lastSeen = data.versions.reduce<string | null>(
    (latest, v) =>
      latest === null || Date.parse(v.last_seen) > Date.parse(latest) ? v.last_seen : latest,
    null,
  );

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <h2 className="text-heading-2">{m.title}</h2>
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          aria-expanded={expanded}
          aria-controls={sectionId}
          onClick={() => setExpanded(!expanded)}
        >
          {m.showKeys}
          <ChevronDown
            aria-hidden="true"
            className={cn("transition-transform", expanded && "rotate-180")}
          />
        </Button>
      </div>
      <div id={sectionId} hidden={!expanded} className="flex flex-col gap-6">
        {expanded ? <IntegrationOnboarding app={app} ingestUrl={ingestUrl} /> : null}
      </div>
      <div className="grid grid-cols-4 gap-4">
        <HealthKpi label={fmt(m.kpi.accepted, { hours: data.hours })}>
          {f.formatInt(data.accepted_total)}
        </HealthKpi>
        <HealthKpi label={fmt(m.kpi.rejected, { hours: data.hours })}>
          {f.formatInt(data.rejected_total)}
        </HealthKpi>
        <HealthKpi label={m.kpi.rejectionRate}>
          <span className={cn(high && "text-warning")}>
            {f.formatRejectionRate(data.rejected_total, data.accepted_total)}
          </span>
          {high ? <RejectionRateWarning /> : null}
        </HealthKpi>
        <HealthKpi label={m.kpi.lastEvent}>
          <span className="text-heading-3">
            {lastSeen ? f.formatDateTime(lastSeen, app.timezone) : "—"}
          </span>
        </HealthKpi>
      </div>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">{m.sections.hourly}</h3>
        <HourlyIngestChart hourly={data.hourly} timezone={app.timezone} />
      </Card>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">{m.sections.reasons}</h3>
        <RejectionReasonsTable reasons={data.reasons} timezone={app.timezone} />
      </Card>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">{m.sections.versions}</h3>
        <VersionsHealthTable versions={data.versions} timezone={app.timezone} />
      </Card>
    </div>
  );
}
