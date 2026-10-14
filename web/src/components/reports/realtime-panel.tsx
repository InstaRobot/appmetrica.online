"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { Pause, Play, Radio, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { EventFeed } from "@/components/realtime/event-feed";
import { PerMinuteChart } from "@/components/realtime/per-minute-chart";
import { RealtimeCounter } from "@/components/realtime/realtime-counter";
import { RealtimeSkeleton } from "@/components/skeletons/realtime-skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetRealtime } from "@/lib/api/gen/endpoints/realtime/realtime";
import type { App } from "@/lib/api/gen/model";
import { formatClock } from "@/lib/format";
import { useRealtimeState } from "@/lib/url-state";
import { EnvironmentSwitch } from "./environment-switch";

// Опрос getRealtime раз в 5 с (14-ux-ui-spec.md S-07); после 3 неудачных опросов подряд — предупреждение.
const POLL_MS = 5000;
const STALE_AFTER_FAILURES = 3;

type RealtimePanelProps = { appId: string };

// S-07 Realtime (FR-015). Приложение (TZ) берётся из кэша AppShell.
export function RealtimePanel({ appId }: RealtimePanelProps) {
  const app = useGetApp(appId);
  return app.data?.status === 200 ? <RealtimeReport app={app.data.data} /> : <RealtimeSkeleton />;
}

function RealtimeReport({ app }: { app: App }) {
  const { env, setEnv } = useRealtimeState();
  const [paused, setPaused] = useState(false);
  // На паузе опрос, фокус окна и восстановление сети запросов не делают; на скрытой вкладке
  // TanStack Query сам не опрашивает (refetchIntervalInBackground: false).
  const realtime = useGetRealtime(
    app.id,
    { environment: env },
    {
      query: {
        refetchInterval: paused ? false : POLL_MS,
        refetchIntervalInBackground: false,
        refetchOnWindowFocus: !paused,
        refetchOnReconnect: !paused,
        staleTime: 0,
        // Каждый опрос — одна попытка: подряд идущие ошибки считаются по опросам.
        retry: false,
        placeholderData: keepPreviousData,
      },
    },
  );

  // Ошибки опроса после последнего успешного ответа: база сдвигается при каждом успехе.
  const [base, setBase] = useState({ dataAt: realtime.dataUpdatedAt, errors: 0 });
  if (base.dataAt !== realtime.dataUpdatedAt) {
    setBase({ dataAt: realtime.dataUpdatedAt, errors: realtime.errorUpdateCount });
  }
  const failures = realtime.errorUpdateCount - base.errors;

  const data = realtime.data?.status === 200 ? realtime.data.data : undefined;
  const stale = data !== undefined && realtime.isError && failures >= STALE_AFTER_FAILURES;

  const togglePause = () => {
    // «Продолжить» сразу запрашивает свежие данные, дальше — обычный интервал.
    if (paused) void realtime.refetch();
    setPaused(!paused);
  };

  let content: React.ReactNode;
  if (data) {
    content = (
      <>
        <div className="grid grid-cols-[320px_1fr] gap-4">
          <RealtimeCounter
            value={data.active_now}
            updatedAt={data.generated_at}
            timezone={app.timezone}
            paused={paused}
            stale={stale}
          />
          <Card className="gap-4 px-6">
            <h2 className="text-heading-3">Старты сессий по минутам</h2>
            <PerMinuteChart points={data.per_minute} timezone={app.timezone} />
          </Card>
        </div>
        {stale ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden />
            <AlertDescription>
              Не удаётся обновить данные. Показаны данные на{" "}
              {formatClock(data.generated_at, app.timezone)}.
            </AlertDescription>
          </Alert>
        ) : null}
        <Card className="gap-4 px-6">
          <h2 className="text-heading-3">Последние события</h2>
          {data.recent_events.length === 0 ? (
            <EmptyState
              icon={Radio}
              title="За последние 30 минут событий нет"
              description="Отправьте тестовое событие из сниппета на вкладке «Интеграция» — оно появится здесь в течение 10 секунд."
            />
          ) : (
            <EventFeed events={data.recent_events} timezone={app.timezone} appId={app.id} />
          )}
        </Card>
      </>
    );
  } else if (realtime.isError) {
    // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
    content = (
      <Card>
        <ErrorState
          error={realtime.error as unknown as ApiError}
          onRetry={() => void realtime.refetch()}
        />
      </Card>
    );
  } else {
    content = <RealtimeSkeleton />;
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <Button variant="outline" size="sm" aria-pressed={paused} onClick={togglePause}>
          {paused ? <Play aria-hidden /> : <Pause aria-hidden />}
          {paused ? "Продолжить" : "Пауза"}
        </Button>
        <div className="ml-auto">
          <EnvironmentSwitch value={env} onChange={(next) => void setEnv(next)} />
        </div>
      </div>
      {content}
    </div>
  );
}
