"use client";

import { useQueries } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useState } from "react";
import { CopyField } from "@/components/common/copy-field";
import { ErrorState } from "@/components/feedback/error-state";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ApiError } from "@/lib/api/fetcher";
import { useListWriteKeys } from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import { getGetAudienceBreakdownQueryOptions } from "@/lib/api/gen/endpoints/audience/audience";
import { type App, Environment } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";
import { todayIn, yesterdayIn } from "@/lib/period";
import { activeWriteKeyFor, PlatformIntegrationSection } from "./platform-integration-section";

// Статус платформ: опрос раз в 3 с, пока есть платформа без событий, затем раз в 30 с (как здоровье).
const WAITING_POLL_MS = 3000;
const RECEIVED_POLL_MS = 30_000;
const ENVIRONMENTS = [Environment.production, Environment.development] as const;

type IntegrationOnboardingProps = { app: App; ingestUrl: string };

// Онбординг S-10 по платформам (FR-7): общий эндпоинт, секция на каждую явную
// платформу приложения и ссылка «Добавить платформу» в настройки.
export function IntegrationOnboarding({ app, ingestUrl }: IntegrationOnboardingProps) {
  const m = useMessages();
  const keys = useListWriteKeys(app.id);
  const list = keys.data?.status === 200 ? keys.data.data.items : undefined;
  const observed = useObservedPlatforms(app);

  if (!list) {
    if (keys.isError) {
      // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
      return (
        <Card className="px-4 md:px-6">
          <ErrorState
            error={keys.error as unknown as ApiError}
            onRetry={() => void keys.refetch()}
          />
        </Card>
      );
    }
    return (
      <Card role="status" aria-label={m.common.loading} className="gap-3 px-4 md:px-6">
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-8 w-full" />
      </Card>
    );
  }

  return (
    <>
      <Card className="gap-3 px-4 md:px-6">
        <h3 className="text-heading-3">{m.integration.endpoint.title}</h3>
        <p className="text-body-sm text-fg-muted">{m.integration.endpoint.hint}</p>
        <CopyField value={ingestUrl} label={m.integration.endpoint.field} />
      </Card>
      {app.platforms.map((platform) => (
        <PlatformIntegrationSection
          key={platform}
          appId={app.id}
          platform={platform}
          writeKey={activeWriteKeyFor(list, platform)}
          received={observed.has(platform)}
          ingestUrl={ingestUrl}
        />
      ))}
      <Link
        href={`/apps/${app.id}/settings#platforms` as Route}
        className="flex min-h-11 items-center gap-1.5 self-start text-body-sm text-fg-link hover:underline md:min-h-0"
      >
        <Plus aria-hidden="true" className="size-4" />
        {m.integration.addPlatform}
      </Link>
    </>
  );
}

// useObservedPlatforms — платформы с событиями за 48 ч (вчера и сегодня по TZ приложения): breakdown
// по платформе в обоих окружениях, потому что сниппеты онбординга шлют environment=development.
function useObservedPlatforms(app: App): Set<string> {
  const [allReceived, setAllReceived] = useState(false);
  const from = yesterdayIn(app.timezone);
  const to = todayIn(app.timezone);
  const results = useQueries({
    queries: ENVIRONMENTS.map((environment) =>
      getGetAudienceBreakdownQueryOptions(
        app.id,
        { from, to, dimension: "platform", environment, limit: 50 },
        {
          query: {
            refetchInterval: allReceived ? RECEIVED_POLL_MS : WAITING_POLL_MS,
            staleTime: 0,
          },
        },
      ),
    ),
  });

  const observed = new Set<string>();
  for (const result of results) {
    if (result.data?.status !== 200) continue;
    for (const row of result.data.data.rows) {
      if (row.count > 0) observed.add(row.value);
    }
  }
  const everyReceived = app.platforms.every((p) => observed.has(p));
  if (everyReceived !== allReceived) setAllReceived(everyReceived);
  return observed;
}
