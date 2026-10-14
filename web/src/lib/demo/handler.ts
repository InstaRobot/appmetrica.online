// Демо-режим дашборда без бэкенда: fetcher вместо сети вызывает demoFetch и получает настоящий Response
// в форме Go API (contracts/openapi/api.yaml). GET-операции отвечают синтетическим детерминированным
// датасетом, мутации — 403 demo_read_only (кроме logout), неизвестные пути — 404 not_found.
import type {
  BreakdownDimension,
  BreakdownMetric,
  CohortGranularity,
  Environment,
  Granularity,
  Platform,
  SplitParameter,
  TimeSeriesMetric,
  Unit,
} from "@/lib/api/gen/model";
import { isIsoDate, MAX_HOUR_RANGE_DAYS, MAX_RANGE_DAYS, rangeDays } from "@/lib/period";
import {
  AUTH_STATE,
  apiKeys,
  deletionRequests,
  HEALTH,
  mergePreview,
  passkeys,
  READY,
  SETTINGS,
  systemStatus,
  writeKeys,
} from "./admin";
import { DEMO_APPS } from "./apps";
import { integrationHealth, realtime } from "./live";
import { type DemoApp, demoApp, type FilterDim, type Filters, makeScope } from "./model";
import { getProfile, searchProfiles } from "./profiles";
import {
  audienceSummary,
  breakdown,
  portfolio,
  retention,
  rollingActive,
  timeseries,
} from "./reports";

const REQUEST_ID = "demo";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const PERSON_KEY =
  /^(u:.{1,128}|i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/su;
const COUNTRY = /^[A-Z]{2}$/;

const PLATFORMS: readonly Platform[] = [
  "ios",
  "android",
  "web",
  "macos",
  "windows",
  "linux",
  "bot",
  "server",
  "other",
];
const ENVIRONMENTS: readonly Environment[] = ["production", "development"];
const UNITS: readonly Unit[] = ["persons", "installs"];
const GRANULARITIES: readonly Granularity[] = ["hour", "day", "week", "month"];
const TS_METRICS: readonly TimeSeriesMetric[] = ["active", "new", "sessions", "avg_duration_ms"];
const SPLITS: readonly SplitParameter[] = ["none", "platform"];
const BREAKDOWN_METRICS: readonly BreakdownMetric[] = ["active", "new", "sessions"];
const COHORT_GRANULARITIES: readonly CohortGranularity[] = ["day", "week"];
const DIMENSIONS: readonly BreakdownDimension[] = [
  "platform",
  "os_name",
  "os_version",
  "app_version",
  "device_model",
  "device_type",
  "country",
  "city",
  "locale",
];

class DemoError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details?: Record<string, unknown>,
  ) {
    super(message);
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", "x-request-id": REQUEST_ID },
  });
}

function errorResponse(e: DemoError): Response {
  return json(
    {
      error: {
        code: e.code,
        message: e.message,
        request_id: REQUEST_ID,
        ...(e.details ? { details: e.details } : {}),
      },
    },
    e.status,
  );
}

const invalid = (field: string, message: string) =>
  new DemoError(400, "invalid_request", message, { field });

// --- параметры запроса ---

type Query = URLSearchParams;

function oneOf<T extends string>(q: Query, name: string, allowed: readonly T[], fallback: T): T {
  const raw = q.get(name);
  if (raw === null) return fallback;
  const found = allowed.find((v) => v === raw);
  if (!found) throw invalid(name, `${name}: допустимо ${allowed.join(", ")}.`);
  return found;
}

function integer(q: Query, name: string, min: number, max: number, fallback: number): number {
  const raw = q.get(name);
  if (raw === null) return fallback;
  const n = Number(raw);
  if (!/^-?\d+$/.test(raw) || n < min || n > max) {
    throw invalid(name, `${name}: целое от ${min} до ${max}.`);
  }
  return n;
}

function bool(q: Query, name: string): boolean {
  const raw = q.get(name);
  if (raw === null || raw === "false") return false;
  if (raw === "true") return true;
  throw invalid(name, `${name}: допустимо true или false.`);
}

function date(q: Query, name: string): string {
  const raw = q.get(name);
  if (raw === null || raw === "") throw invalid(name, `Параметр ${name} обязателен.`);
  if (!isIsoDate(raw)) throw invalid(name, `${name}: ожидается дата YYYY-MM-DD.`);
  return raw;
}

function filters(q: Query): Filters {
  const platform = q.getAll("platform");
  if (platform.some((p) => !PLATFORMS.some((v) => v === p))) {
    throw invalid("platform", "platform: неизвестная платформа.");
  }
  const appVersion = q.getAll("app_version");
  if (appVersion.some((v) => [...v].length > 64)) {
    throw invalid("app_version", "app_version: не длиннее 64 символов.");
  }
  const osName = q.getAll("os_name");
  if (osName.some((v) => [...v].length > 32)) {
    throw invalid("os_name", "os_name: не длиннее 32 символов.");
  }
  const country = q.getAll("country");
  if (country.some((v) => !COUNTRY.test(v))) {
    throw invalid(
      "country",
      "country: код страны ISO 3166-1 alpha-2 из двух заглавных латинских букв.",
    );
  }
  const out: Record<FilterDim, readonly string[]> = {
    platform,
    app_version: appVersion,
    os_name: osName,
    country,
  };
  return out;
}

type Period = { from: string; to: string };

// period — from/to отчёта: включительно, from ≤ to, не длиннее 366 дней (для hour — 7).
function period(q: Query, granularity: Granularity = "day"): Period {
  const from = date(q, "from");
  const to = date(q, "to");
  if (from > to) throw invalid("from", "from не может быть позже to.");
  const days = rangeDays(from, to);
  if (days > MAX_RANGE_DAYS) {
    throw new DemoError(
      400,
      "range_too_large",
      `Диапазон не может превышать ${MAX_RANGE_DAYS} дней (запрошено ${days}).`,
      { field: "to", max_days: MAX_RANGE_DAYS },
    );
  }
  if (granularity === "hour" && days > MAX_HOUR_RANGE_DAYS) {
    throw new DemoError(
      400,
      "range_too_large",
      `Диапазон для granularity=hour не может превышать ${MAX_HOUR_RANGE_DAYS} дней (запрошено ${days}).`,
      { field: "to", max_days: MAX_HOUR_RANGE_DAYS },
    );
  }
  return { from, to };
}

function reportScope(app: DemoApp, q: Query, now: number) {
  const unit = oneOf(q, "unit", UNITS, "persons");
  const env = oneOf(q, "environment", ENVIRONMENTS, "production");
  return makeScope(app, now, { env, unit, filters: filters(q) });
}

function compareOf(q: Query): boolean {
  return oneOf(q, "compare", ["previous_period", "none"] as const, "previous_period") !== "none";
}

function requireApp(id: string | undefined): DemoApp {
  if (!id || !UUID.test(id)) throw invalid("app_id", "app_id: ожидается UUID.");
  const app = demoApp(id.toLowerCase());
  if (!app) throw new DemoError(404, "app_not_found", "Приложение не найдено.");
  return app;
}

// --- маршруты ---

type Ctx = { params: Record<string, string>; query: Query; now: number };
type Route = { pattern: string[]; handle: (ctx: Ctx) => Response };

function route(path: string, handle: (ctx: Ctx) => Response): Route {
  return { pattern: path.split("/").filter(Boolean), handle };
}

const ROUTES: readonly Route[] = [
  route("/health", () => json(HEALTH)),
  route("/ready", () => json(READY)),
  route("/v1/apps", ({ query }) => {
    const archived = bool(query, "include_archived");
    const items = DEMO_APPS.filter((a) => archived || a.status === "active").toSorted((a, b) =>
      a.name.toLowerCase() < b.name.toLowerCase() ? -1 : 1,
    );
    return json({ items });
  }),
  route("/v1/apps/:app_id", ({ params }) => json(requireApp(params.app_id).app)),
  route("/v1/portfolio", ({ query, now }) => {
    const unit = oneOf(query, "unit", UNITS, "persons");
    const env = oneOf(query, "environment", ENVIRONMENTS, "production");
    const raw = query.get("date");
    if (raw !== null && !isIsoDate(raw)) throw invalid("date", "date: ожидается дата YYYY-MM-DD.");
    const includeArchived = bool(query, "include_archived");
    return json(portfolio(now, { date: raw, env, unit, includeArchived }));
  }),
  route("/v1/apps/:app_id/audience/summary", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const scope = reportScope(app, query, now);
    const { from, to } = period(query);
    return json(audienceSummary(scope, from, to, compareOf(query)));
  }),
  route("/v1/apps/:app_id/audience/timeseries", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const scope = reportScope(app, query, now);
    const granularity = oneOf(query, "granularity", GRANULARITIES, "day");
    const { from, to } = period(query, granularity);
    const metric = oneOf(query, "metric", TS_METRICS, "active");
    const split = oneOf(query, "split", SPLITS, "none") === "platform";
    return json(timeseries(scope, from, to, granularity, metric, compareOf(query), split));
  }),
  route("/v1/apps/:app_id/audience/rolling-active", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const scope = reportScope(app, query, now);
    const { from, to } = period(query);
    return json(rollingActive(scope, from, to));
  }),
  route("/v1/apps/:app_id/audience/breakdown", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const scope = reportScope(app, query, now);
    const { from, to } = period(query);
    if (query.get("dimension") === null)
      throw invalid("dimension", "Параметр dimension обязателен.");
    const dimension = oneOf(query, "dimension", DIMENSIONS, "platform");
    const metric = oneOf(query, "metric", BREAKDOWN_METRICS, "active");
    const limit = integer(query, "limit", 1, 200, 50);
    return json(breakdown(scope, from, to, dimension, metric, limit));
  }),
  route("/v1/apps/:app_id/retention", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const scope = reportScope(app, query, now);
    const granularity = oneOf(query, "cohort_granularity", COHORT_GRANULARITIES, "day");
    const [fallback, max] = granularity === "week" ? [12, 52] : [30, 90];
    const periods = integer(query, "periods", 1, max, fallback);
    const { from, to } = period(query);
    return json(retention(scope, from, to, granularity, periods));
  }),
  route("/v1/apps/:app_id/realtime", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const env = oneOf(query, "environment", ENVIRONMENTS, "production");
    // Из фильтров отчётов realtime принимает только platform.
    return json(realtime(app, env, now, filters(query).platform ?? []));
  }),
  route("/v1/apps/:app_id/profiles", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const q = query.get("q") ?? "";
    if (q.length < 1 || [...q].length > 128) throw invalid("q", "q: от 1 до 128 символов.");
    return json(searchProfiles(app, q, now));
  }),
  route("/v1/apps/:app_id/profiles/:person_key", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    const key = params.person_key ?? "";
    if (!PERSON_KEY.test(key)) {
      throw invalid("person_key", "person_key: ожидается u:<user_id> или i:<install_id>.");
    }
    const limit = integer(query, "sessions_limit", 1, 500, 100);
    const rawBefore = query.get("before");
    const before = rawBefore === null ? null : Date.parse(rawBefore);
    if (before !== null && (Number.isNaN(before) || !/T/.test(rawBefore ?? ""))) {
      throw invalid("before", "before: ожидается дата-время RFC 3339.");
    }
    const profile = getProfile(app, key, { limit, before }, now);
    if (!profile) throw new DemoError(404, "profile_not_found", "Профиль не найден.");
    return json(profile);
  }),
  route("/v1/apps/:app_id/integration/health", ({ params, query, now }) => {
    const app = requireApp(params.app_id);
    return json(integrationHealth(app, integer(query, "hours", 1, 168, 24), now));
  }),
  route("/v1/apps/:app_id/deletion-requests/:request_id", ({ params, now }) => {
    const app = requireApp(params.app_id);
    const found = deletionRequests(now, app.app.id).items.find(
      (r) => r.id === params.request_id?.toLowerCase(),
    );
    if (!found) throw new DemoError(404, "not_found", "Не найдено");
    return json(found);
  }),
  route("/admin/v1/auth/state", () => json(AUTH_STATE)),
  route("/admin/v1/passkeys", ({ now }) => json(passkeys(now))),
  route("/admin/v1/apps/:app_id/write-keys", ({ params, now }) =>
    json(writeKeys(requireApp(params.app_id).app.id, now)),
  ),
  route("/admin/v1/apps/:app_id/merge-preview", ({ params, query, now }) => {
    const target = requireApp(params.app_id);
    const sourceId = query.get("source_app_id");
    if (sourceId === null || !UUID.test(sourceId)) {
      throw invalid("source_app_id", "source_app_id: ожидается UUID.");
    }
    const source = requireApp(sourceId);
    if (source.app.id === target.app.id) {
      throw invalid("source_app_id", "Нельзя объединить приложение с самим собой.");
    }
    return json(mergePreview(target, source, now));
  }),
  route("/admin/v1/apps/:app_id/merges", ({ params }) => {
    requireApp(params.app_id);
    return json({ items: [] });
  }),
  route("/admin/v1/api-keys", ({ now }) => json(apiKeys(now))),
  route("/admin/v1/settings", () => json(SETTINGS)),
  route("/admin/v1/system/status", ({ now }) => json(systemStatus(now))),
  route("/admin/v1/deletion-requests", ({ query, now }) => {
    const appId = query.get("app_id");
    if (appId !== null && !UUID.test(appId)) throw invalid("app_id", "app_id: ожидается UUID.");
    return json(deletionRequests(now, appId?.toLowerCase() ?? null));
  }),
];

function match(
  pattern: readonly string[],
  segments: readonly string[],
): Record<string, string> | null {
  if (pattern.length !== segments.length) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < pattern.length; i++) {
    const p = pattern[i] ?? "";
    const s = segments[i] ?? "";
    if (p.startsWith(":")) params[p.slice(1)] = s;
    else if (p !== s) return null;
  }
  return params;
}

function decodeSegment(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

// demoFetch — ответ демо-API на запрос orval-клиента. url — путь API с query (`/v1/apps/<id>/…?from=…`),
// now — «сейчас» датасета (по умолчанию текущее время).
export async function demoFetch(
  url: string,
  init?: RequestInit,
  now: Date = new Date(),
): Promise<Response> {
  // Как fetch: отменённый запрос (смена фильтров, размонтирование) отклоняется AbortError.
  init?.signal?.throwIfAborted();
  const method = (init?.method ?? "GET").toUpperCase();
  const parsed = new URL(url, "http://demo.invalid");
  const segments = parsed.pathname.split("/").filter(Boolean).map(decodeSegment);
  if (segments[0] === "api") segments.shift();

  if (method !== "GET" && method !== "HEAD") {
    if (method === "POST" && segments.join("/") === "admin/v1/auth/logout") {
      return new Response(null, { status: 204 });
    }
    return errorResponse(new DemoError(403, "demo_read_only", "Демо-режим: изменения недоступны"));
  }

  for (const r of ROUTES) {
    const params = match(r.pattern, segments);
    if (!params) continue;
    try {
      return r.handle({ params, query: parsed.searchParams, now: now.getTime() });
    } catch (e) {
      if (e instanceof DemoError) return errorResponse(e);
      throw e;
    }
  }
  return errorResponse(new DemoError(404, "not_found", "Не найдено"));
}
