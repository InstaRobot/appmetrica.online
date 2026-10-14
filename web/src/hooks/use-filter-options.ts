"use client";

import { useQueries } from "@tanstack/react-query";
import { useState } from "react";
import { getGetAudienceBreakdownQueryOptions } from "@/lib/api/gen/endpoints/audience/audience";
import { FILTER_DIMENSIONS, type FilterDimension, type ReportParams } from "@/lib/url-state";

type FilterOptions = Partial<Record<FilterDimension, { value: string; count: number }[]>>;

// Значения FilterBar (OQ-053): getAudienceBreakdown по четырём измерениям за текущий период (limit 50).
// Запросы идут только после первого открытия списка «Фильтр».
export function useFilterOptions(appId: string, params: ReportParams) {
  const [opened, setOpened] = useState(false);
  const queries = useQueries({
    queries: FILTER_DIMENSIONS.map((dimension) => ({
      ...getGetAudienceBreakdownQueryOptions(appId, {
        ...params,
        dimension,
        metric: "active",
        limit: 50,
      }),
      enabled: opened,
    })),
  });

  const options: FilterOptions = {};
  FILTER_DIMENSIONS.forEach((dim, i) => {
    const q = queries[i];
    if (q?.data?.status === 200) options[dim] = q.data.data.rows;
  });

  return {
    options,
    loading: opened && queries.some((q) => q.isPending),
    onOpenChange: (open: boolean) => {
      if (open) setOpened(true);
    },
  };
}
