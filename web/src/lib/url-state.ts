// TODO(T-038): заглушка URL-состояния S-05.
import { createLoader, createSerializer, parseAsString } from "nuqs";
import type { Environment, Unit } from "@/lib/api/gen/model";
import type { Granularity, PeriodValue } from "./period";

export const FILTER_DIMENSIONS = ["platform", "app_version", "os_name", "country"] as const;
export type FilterDimension = (typeof FILTER_DIMENSIONS)[number];
export type Filters = Partial<Record<FilterDimension, string[]>>;
export type ChartMetric = "active" | "new" | "sessions" | "avg_duration_ms";
export type BreakdownDim =
  | "platform"
  | "os_name"
  | "os_version"
  | "app_version"
  | "device_model"
  | "device_type"
  | "country"
  | "city"
  | "locale";
export type BreakdownMetricKey = "active" | "new" | "sessions";

export const audienceSearchParams = { period: parseAsString };
export const loadAudienceSearch = createLoader(audienceSearchParams);
export const serializeAudienceSearch = createSerializer(audienceSearchParams);
export type AudienceSearch = Record<string, unknown>;

export type AudienceState = {
  period: PeriodValue;
  granularity: Granularity;
  granularityAdjusted: boolean;
  compare: boolean;
  metric: ChartMetric;
  dim: BreakdownDim;
  bmetric: BreakdownMetricKey;
  filters: Filters;
  env: Environment;
  unit: Unit;
};

export type ReportParams = {
  from: string;
  to: string;
  environment: Environment;
  unit: Unit;
} & Filters;

export function resolveAudienceState(_search: AudienceSearch, _today: string): AudienceState {
  throw new Error("not implemented");
}

export function periodSearch(_period: PeriodValue): AudienceSearch {
  throw new Error("not implemented");
}

export function reportParams(_state: AudienceState): ReportParams {
  throw new Error("not implemented");
}

export function useAudienceState(
  _timezone: string,
  _today?: string,
): { state: AudienceState; update: (patch: AudienceSearch) => Promise<URLSearchParams> } {
  throw new Error("not implemented");
}
