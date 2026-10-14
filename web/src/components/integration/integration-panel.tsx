"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";
import { ErrorState } from "@/components/feedback/error-state";
import { HourlyIngestChart } from "@/components/reports/lazy-charts";
import { IntegrationSkeleton } from "@/components/skeletons/integration-skeleton";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetIntegrationHealth } from "@/lib/api/gen/endpoints/integration/integration";
import type { App, IntegrationHealth } from "@/lib/api/gen/model";
import { formatInt, formatRejectionRate } from "@/lib/format";
import { ingestEndpoint } from "@/lib/snippets";
import { cn } from "@/lib/utils";
import { IntegrationOnboarding } from "./integration-onboarding";
import { isRejectionRateHigh, RejectionRateWarning } from "./rejection-rate-warning";
import { RejectionReasonsTable } from "./rejection-reasons-table";
import { VersionsHealthTable } from "./versions-health-table";
import { WaitingForFirstEvent } from "./waiting-for-first-event";

// Опрос getIntegrationHealth(hours=24): раз в 3 с до первого события, затем раз в 30 с (S-10).
const HOURS = 24;
const WAITING_POLL_MS = 3000;
const HEALTHY_POLL_MS = 30_000;

type IntegrationPanelProps = { appId: string };

// S-10 Интеграция (FR-017). Приложение (платформа, TZ) берётся из кэша AppShell.
export function IntegrationPanel({ appId }: IntegrationPanelProps) {
  const app = useGetApp(appId);
  return app.data?.status === 200 ? (
    <IntegrationReport app={app.data.data} />
  ) : (
    <IntegrationSkeleton />
  );
}

function IntegrationReport({ app }: { app: App }) {
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
        <h2 className="text-heading-2">Интеграция</h2>
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
      className="flex flex-col gap-1.5 rounded-xl border border-border bg-card p-5 shadow-xs"
    >
      <span className="text-label-md text-fg-muted">{label}</span>
      <span className="flex items-center gap-2 text-heading-1 tabular-nums">{children}</span>
    </div>
  );
}

type IntegrationHealthViewProps = { app: App; data: IntegrationHealth; ingestUrl: string };

function IntegrationHealthView({ app, data, ingestUrl }: IntegrationHealthViewProps) {
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const high = isRejectionRateHigh(data.rejected_total, data.accepted_total);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <h2 className="text-heading-2">Интеграция</h2>
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          aria-expanded={expanded}
          aria-controls={sectionId}
          onClick={() => setExpanded(!expanded)}
        >
          Показать ключ и сниппеты
          <ChevronDown
            aria-hidden="true"
            className={cn("transition-transform", expanded && "rotate-180")}
          />
        </Button>
      </div>
      <div id={sectionId} hidden={!expanded} className="flex flex-col gap-6">
        {expanded ? <IntegrationOnboarding app={app} ingestUrl={ingestUrl} /> : null}
      </div>
      <div className="grid grid-cols-3 gap-4">
        <HealthKpi label={`Принято за ${data.hours} ч`}>{formatInt(data.accepted_total)}</HealthKpi>
        <HealthKpi label={`Отклонено за ${data.hours} ч`}>
          {formatInt(data.rejected_total)}
        </HealthKpi>
        <HealthKpi label="Доля отказов">
          <span className={cn(high && "text-warning")}>
            {formatRejectionRate(data.rejected_total, data.accepted_total)}
          </span>
          {high ? <RejectionRateWarning /> : null}
        </HealthKpi>
      </div>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">По часам</h3>
        <HourlyIngestChart hourly={data.hourly} timezone={app.timezone} />
      </Card>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">Причины отказов</h3>
        <RejectionReasonsTable reasons={data.reasons} timezone={app.timezone} />
      </Card>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">По версиям приложения</h3>
        <VersionsHealthTable versions={data.versions} timezone={app.timezone} />
      </Card>
    </div>
  );
}
