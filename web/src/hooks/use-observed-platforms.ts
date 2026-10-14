"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { useGetAudienceBreakdown } from "@/lib/api/gen/endpoints/audience/audience";
import type { Environment, Unit } from "@/lib/api/gen/model";

type ObservedParams = { from: string; to: string; environment: Environment; unit: Unit };

// Платформы событий за период для PlatformSwitcher (FR-5): breakdown dimension=platform без фильтров отчёта,
// чтобы выбор платформы или версии не прятал остальные платформы. До ответа — пустой список.
export function useObservedPlatforms(appId: string, params: ObservedParams): string[] {
  const breakdown = useGetAudienceBreakdown(
    appId,
    {
      from: params.from,
      to: params.to,
      environment: params.environment,
      unit: params.unit,
      dimension: "platform",
      metric: "active",
      limit: 50,
    },
    { query: { placeholderData: keepPreviousData } },
  );
  return breakdown.data?.status === 200 ? breakdown.data.data.rows.map((r) => r.value) : [];
}
