"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { ListChecks } from "lucide-react";
import { useParams, useSearchParams } from "next/navigation";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { reportQuery } from "@/components/layout/app-tabs";
import { EventsSkeleton } from "@/components/skeletons/events-skeleton";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Card } from "@/components/ui/card";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useListEvents } from "@/lib/api/gen/endpoints/events/events";
import type { App } from "@/lib/api/gen/model";
import { eventsCurl } from "@/lib/copy-as-curl";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { publicHref } from "@/lib/i18n/routes";
import { todayIn } from "@/lib/period";
import { toast } from "@/lib/toast";
import { reportParams, useEventsState } from "@/lib/url-state";
import { EventsTable } from "./events-table";
import { EventsToolbar } from "./events-toolbar";

// Вкладка «События» (FR-102). Приложение (TZ) берётся из кэша AppShell.
export function EventsView() {
  const { appId } = useParams<{ appId: string }>();
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <EventsReport app={app.data.data} /> : <EventsSkeleton />;
}

function EventsReport({ app }: { app: App }) {
  const messages = useMessages();
  const m = messages.events;
  const locale = useLocale();
  const search = useSearchParams();
  const today = todayIn(app.timezone);
  const { state, update } = useEventsState(app.timezone, today);
  const params = reportParams(state);
  const list = useListEvents(
    app.id,
    { ...params, compare: "previous_period" },
    { query: { placeholderData: keepPreviousData } },
  );

  // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
  const error = list.isError ? (list.error as unknown as ApiError | null) : null;
  const rangeError =
    error?.code === "range_too_large" ? messages.errors.codes.range_too_large : undefined;
  const data = list.data?.status === 200 ? list.data.data : undefined;

  const copyRequest = async () => {
    await navigator.clipboard.writeText(eventsCurl(app.id, state));
    toast.success(messages.reports.audience.requestCopied, { duration: 1500 });
  };

  let content: React.ReactNode = null;
  if (error && !rangeError) {
    content = <ErrorState error={error} onRetry={() => void list.refetch()} />;
  } else if (data && data.events.length === 0) {
    content = (
      <EmptyState
        icon={ListChecks}
        title={m.empty.title}
        description={m.empty.description}
        action={{
          label: m.empty.action,
          href: publicHref(locale, "/docs/concepts/events"),
        }}
      />
    );
  } else if (data) {
    content = (
      <div className="flex flex-col gap-3 px-4 py-2 md:px-6">
        <EventsTable
          appId={app.id}
          events={data.events}
          unit={state.unit}
          query={reportQuery(search)}
        />
        {data.truncated ? <p className="text-caption text-fg-muted">{m.truncated}</p> : null}
      </div>
    );
  } else if (list.isPending) {
    content = <TableSkeleton rows={6} />;
  }

  return (
    <div className="flex flex-col gap-6">
      <EventsToolbar
        app={app}
        state={state}
        update={update}
        rangeError={rangeError}
        onCopy={() => void copyRequest()}
      />
      <Card className="gap-0 px-0 py-0">{content}</Card>
    </div>
  );
}
