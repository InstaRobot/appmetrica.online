// TODO(T-038): заглушка «Скопировать как запрос к Read API».
import type { AudienceState } from "./url-state";

export const READ_API_URL = "https://api.appmetrica.online";
export type AudienceReport = "summary" | "timeseries" | "rolling-active" | "breakdown";

export function readApiBaseUrl(): string {
  return "";
}

export function buildCurl(_path: string, _baseUrl?: string): string {
  return "";
}

export function audienceCurl(
  _appId: string,
  _state: AudienceState,
  _report: AudienceReport = "summary",
): string {
  return "";
}
