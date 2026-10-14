// «Скопировать как запрос к Read API» (FR-018, 02-tech-spec.md AC-6.1.3): curl с текущими параметрами
// отчёта и плейсхолдером ключа `$APPMETRICA_SK` — секрета в UI нет. Строки в двойных кавычках, чтобы
// shell подставил переменную; значения в URL percent-кодируются, поэтому `"`, `$` и `` ` `` в них не попадают.
import type { AudienceState } from "./url-state";

export const READ_API_URL = "https://api.appmetrica.online";

export type AudienceReport = "summary" | "timeseries" | "rolling-active" | "breakdown";

// Адрес Read API: NEXT_PUBLIC_API_URL (локально — http://localhost:8080), иначе production.
export function readApiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_API_URL || READ_API_URL).replace(/\/+$/, "");
}

export function buildCurl(path: string, baseUrl: string = readApiBaseUrl()): string {
  return `curl -H "Authorization: Bearer $APPMETRICA_SK" "${baseUrl}${path}"`;
}

// Параметры в порядке документации: период, параметры отчёта, фильтры, окружение, unit.
function reportQuery(state: AudienceState, report: AudienceReport): URLSearchParams {
  const q = new URLSearchParams({ from: state.period.from, to: state.period.to });
  if (report === "timeseries") {
    q.set("granularity", state.granularity);
    q.set("metric", state.metric);
    q.set("compare", state.compare ? "previous_period" : "none");
  }
  if (report === "breakdown") {
    q.set("dimension", state.dim);
    q.set("metric", state.bmetric);
  }
  for (const [key, values] of Object.entries(state.filters)) {
    for (const value of values) q.append(key, value);
  }
  q.set("environment", state.env);
  q.set("unit", state.unit);
  return q;
}

export function audienceCurl(
  appId: string,
  state: AudienceState,
  report: AudienceReport = "summary",
  baseUrl?: string,
): string {
  const path = `/v1/apps/${encodeURIComponent(appId)}/audience/${report}?${reportQuery(state, report)}`;
  return buildCurl(path, baseUrl);
}
