import { describe, expect, it } from "vitest";
import type { ZodType } from "zod";
import * as endpoints from "@/lib/api/gen/endpoints";
import * as schemas from "@/lib/api/gen/zod/index.zod";
import { DEMO_APP_IDS, DEMO_APPS } from "./apps";
import { demoFetch } from "./handler";
import { DEMO_PROFILES } from "./profiles";

// «Сейчас» демо-тестов: 12:30 по Москве, среда.
const NOW = new Date("2026-10-14T09:30:00Z");
const IOS = DEMO_APP_IDS.notesIos;
const BOT = DEMO_APP_IDS.bot;
const UNKNOWN_APP = "0199d5f0-0000-7000-8000-000000000000";
const MONTH = { from: "2026-09-15", to: "2026-10-14" };
const DIMENSIONS = [
  "platform",
  "os_name",
  "os_version",
  "app_version",
  "device_model",
  "device_type",
  "country",
  "city",
  "locale",
] as const;

const iosProfile = DEMO_PROFILES.find((p) => p.appId === IOS && p.userId !== null);
if (!iosProfile) throw new Error("demo profile for Notes iOS is missing");

type Case = { url: string; schema: ZodType };

async function firstDeletionRequestUrl(): Promise<string> {
  const res = await demoFetch(endpoints.getListDeletionRequestsUrl(), undefined, NOW);
  const body = (await res.json()) as { items: { id: string; app_id: string }[] };
  const first = body.items[0];
  if (!first) throw new Error("demo deletion requests are empty");
  return endpoints.getGetDeletionRequestUrl(first.app_id, first.id);
}

// Ответы демо на каждую GET-операцию orval-клиента: URL строит сгенерированный клиент, форму проверяет
// сгенерированная zod-схема ответа (контракт api.yaml).
async function contractCases(): Promise<Record<string, Case[]>> {
  const p = iosProfile;
  if (!p) throw new Error("unreachable");
  return {
    apiHealth: [{ url: endpoints.getApiHealthUrl(), schema: schemas.ApiHealthResponse }],
    apiReady: [{ url: endpoints.getApiReadyUrl(), schema: schemas.ApiReadyResponse }],
    getAuthState: [{ url: endpoints.getGetAuthStateUrl(), schema: schemas.GetAuthStateResponse }],
    listApps: [
      { url: endpoints.getListAppsUrl(), schema: schemas.ListAppsResponse },
      {
        url: endpoints.getListAppsUrl({ include_archived: true }),
        schema: schemas.ListAppsResponse,
      },
    ],
    getApp: DEMO_APPS.map((app) => ({
      url: endpoints.getGetAppUrl(app.id),
      schema: schemas.GetAppResponse,
    })),
    getPortfolio: [
      { url: endpoints.getGetPortfolioUrl(), schema: schemas.GetPortfolioResponse },
      {
        url: endpoints.getGetPortfolioUrl({
          date: "2026-10-12",
          unit: "installs",
          include_archived: true,
        }),
        schema: schemas.GetPortfolioResponse,
      },
      {
        url: endpoints.getGetPortfolioUrl({ environment: "development" }),
        schema: schemas.GetPortfolioResponse,
      },
    ],
    getAudienceSummary: [
      {
        url: endpoints.getGetAudienceSummaryUrl(IOS, MONTH),
        schema: schemas.GetAudienceSummaryResponse,
      },
      {
        url: endpoints.getGetAudienceSummaryUrl(BOT, {
          ...MONTH,
          compare: "none",
          unit: "installs",
          country: ["RU", "KZ"],
        }),
        schema: schemas.GetAudienceSummaryResponse,
      },
    ],
    getAudienceTimeseries: [
      {
        url: endpoints.getGetAudienceTimeseriesUrl(IOS, {
          ...MONTH,
          granularity: "day",
          metric: "active",
        }),
        schema: schemas.GetAudienceTimeseriesResponse,
      },
      {
        url: endpoints.getGetAudienceTimeseriesUrl(IOS, {
          from: "2026-10-13",
          to: "2026-10-14",
          granularity: "hour",
          metric: "sessions",
          platform: ["ios"],
          app_version: ["3.4.0"],
        }),
        schema: schemas.GetAudienceTimeseriesResponse,
      },
      {
        url: endpoints.getGetAudienceTimeseriesUrl(DEMO_APP_IDS.site, {
          from: "2026-07-01",
          to: "2026-10-14",
          granularity: "week",
          metric: "new",
          os_name: ["Windows"],
        }),
        schema: schemas.GetAudienceTimeseriesResponse,
      },
      {
        url: endpoints.getGetAudienceTimeseriesUrl(DEMO_APP_IDS.notesAndroid, {
          from: "2026-04-01",
          to: "2026-10-14",
          granularity: "month",
          metric: "avg_duration_ms",
          compare: "none",
          environment: "development",
        }),
        schema: schemas.GetAudienceTimeseriesResponse,
      },
    ],
    getRollingActive: [
      {
        url: endpoints.getGetRollingActiveUrl(IOS, MONTH),
        schema: schemas.GetRollingActiveResponse,
      },
    ],
    getAudienceBreakdown: DIMENSIONS.map((dimension) => ({
      url: endpoints.getGetAudienceBreakdownUrl(IOS, { ...MONTH, dimension }),
      schema: schemas.GetAudienceBreakdownResponse,
    })).concat(
      DIMENSIONS.map((dimension) => ({
        url: endpoints.getGetAudienceBreakdownUrl(DEMO_APP_IDS.site, {
          ...MONTH,
          dimension,
          metric: "sessions",
          limit: 5,
        }),
        schema: schemas.GetAudienceBreakdownResponse,
      })),
    ),
    getRetention: [
      {
        url: endpoints.getGetRetentionUrl(IOS, { ...MONTH, cohort_granularity: "day" }),
        schema: schemas.GetRetentionResponse,
      },
      {
        url: endpoints.getGetRetentionUrl(IOS, {
          from: "2026-07-06",
          to: "2026-10-14",
          cohort_granularity: "week",
          periods: 12,
        }),
        schema: schemas.GetRetentionResponse,
      },
    ],
    getRealtime: [
      { url: endpoints.getGetRealtimeUrl(IOS), schema: schemas.GetRealtimeResponse },
      {
        url: endpoints.getGetRealtimeUrl(BOT, { environment: "development" }),
        schema: schemas.GetRealtimeResponse,
      },
      {
        url: endpoints.getGetRealtimeUrl(DEMO_APP_IDS.oldGame),
        schema: schemas.GetRealtimeResponse,
      },
    ],
    searchProfiles: [
      {
        url: endpoints.getSearchProfilesUrl(IOS, { q: p.installId }),
        schema: schemas.SearchProfilesResponse,
      },
      {
        url: endpoints.getSearchProfilesUrl(IOS, { q: "nobody" }),
        schema: schemas.SearchProfilesResponse,
      },
    ],
    getProfile: [
      {
        url: endpoints.getGetProfileUrl(IOS, encodeURIComponent(p.personKey)),
        schema: schemas.GetProfileResponse,
      },
      ...DEMO_PROFILES.map((profile) => ({
        url: endpoints.getGetProfileUrl(profile.appId, encodeURIComponent(profile.personKey), {
          sessions_limit: 20,
        }),
        schema: schemas.GetProfileResponse,
      })),
    ],
    getIntegrationHealth: DEMO_APPS.map((app) => ({
      url: endpoints.getGetIntegrationHealthUrl(app.id),
      schema: schemas.GetIntegrationHealthResponse,
    })),
    getDeletionRequest: [
      { url: await firstDeletionRequestUrl(), schema: schemas.GetDeletionRequestResponse },
    ],
    listPasskeys: [{ url: endpoints.getListPasskeysUrl(), schema: schemas.ListPasskeysResponse }],
    listWriteKeys: DEMO_APPS.map((app) => ({
      url: endpoints.getListWriteKeysUrl(app.id),
      schema: schemas.ListWriteKeysResponse,
    })),
    listApiKeys: [{ url: endpoints.getListApiKeysUrl(), schema: schemas.ListApiKeysResponse }],
    getSettings: [{ url: endpoints.getGetSettingsUrl(), schema: schemas.GetSettingsResponse }],
    getSystemStatus: [
      { url: endpoints.getGetSystemStatusUrl(), schema: schemas.GetSystemStatusResponse },
    ],
    listDeletionRequests: [
      {
        url: endpoints.getListDeletionRequestsUrl(),
        schema: schemas.ListDeletionRequestsResponse,
      },
      {
        url: endpoints.getListDeletionRequestsUrl({ app_id: IOS }),
        schema: schemas.ListDeletionRequestsResponse,
      },
    ],
  };
}

// GET-операции orval-клиента: query key запроса начинается с пути, у мутаций — с HTTP-метода.
function generatedGetOperations(): string[] {
  const ids: string[] = [];
  for (const [name, value] of Object.entries(endpoints)) {
    const op = /^get(\w+)QueryKey$/.exec(name)?.[1];
    if (!op || typeof value !== "function") continue;
    const key = (value as (...args: string[]) => readonly unknown[])("app", "id", "key");
    if (typeof key[0] === "string" && key[0].startsWith("/")) {
      ids.push(op.charAt(0).toLowerCase() + op.slice(1));
    }
  }
  return ids.sort();
}

async function errorOf(res: Response) {
  const body = (await res.json()) as {
    error: { code: string; message: string; request_id: string; details?: Record<string, unknown> };
  };
  return body.error;
}

describe("demoFetch contract", () => {
  it("covers every GET operation of the generated client", async () => {
    const cases = await contractCases();
    expect(Object.keys(cases).sort()).toEqual(generatedGetOperations());
  });

  it("answers every GET with JSON that matches the zod schema of the contract", async () => {
    const cases = await contractCases();
    for (const [operation, list] of Object.entries(cases)) {
      for (const { url, schema } of list) {
        const res = await demoFetch(url, { method: "GET" }, NOW);
        expect(res, `${operation} ${url}`).toBeInstanceOf(Response);
        expect(res.status, `${operation} ${url}`).toBe(200);
        expect(res.headers.get("content-type"), url).toContain("application/json");
        const body: unknown = await res.json();
        const parsed = schema.safeParse(body);
        expect(parsed.error?.issues ?? [], `${operation} ${url}`).toEqual([]);
        // Схема не дописывает default-значения и не выкидывает лишние поля.
        expect(parsed.data, `${operation} ${url}`).toEqual(body);
      }
    }
  });

  it("returns the same body for the same url and now", async () => {
    const cases = await contractCases();
    for (const list of Object.values(cases)) {
      for (const { url } of list) {
        const a = await (await demoFetch(url, undefined, NOW)).text();
        const b = await (await demoFetch(url, undefined, new Date(NOW))).text();
        expect(b, url).toBe(a);
      }
    }
  });

  it("defaults the method to GET and now to the current time", async () => {
    const res = await demoFetch("/v1/apps");
    expect(res.status).toBe(200);
  });

  it("rejects like fetch when the signal is already aborted", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(demoFetch("/v1/apps", { signal: controller.signal }, NOW)).rejects.toMatchObject({
      name: "AbortError",
    });
  });

  it("reports authenticated owner without setup", async () => {
    const res = await demoFetch("/admin/v1/auth/state", undefined, NOW);
    expect(await res.json()).toEqual({ authenticated: true, setup_required: false });
  });
});

describe("demoFetch read-only and errors", () => {
  it.each([
    ["POST", "/admin/v1/apps"],
    ["PATCH", `/admin/v1/apps/${IOS}`],
    ["DELETE", `/admin/v1/apps/${IOS}`],
    ["POST", `/admin/v1/apps/${IOS}/archive`],
    ["POST", `/admin/v1/apps/${IOS}/write-keys`],
    ["POST", "/admin/v1/api-keys"],
    ["PATCH", "/admin/v1/settings"],
    ["POST", "/admin/v1/settings/telegram/test"],
    ["DELETE", "/admin/v1/passkeys/0199d6e0-4444-7ddd-8eee-0f1011121314"],
    ["POST", "/admin/v1/auth/login/begin"],
    ["POST", `/v1/apps/${IOS}/deletion-requests`],
    ["PUT", "/v1/apps"],
    ["post", "/v1/unknown"],
  ])("rejects %s %s as read-only demo", async (method, url) => {
    const res = await demoFetch(url, { method, body: "{}" }, NOW);
    expect(res.status).toBe(403);
    expect(res.headers.get("content-type")).toContain("application/json");
    expect(await res.json()).toEqual({
      error: {
        code: "demo_read_only",
        message: "Демо-режим: изменения недоступны",
        request_id: "demo",
      },
    });
  });

  it("lets logout succeed with 204 and no body", async () => {
    const res = await demoFetch("/admin/v1/auth/logout", { method: "POST" }, NOW);
    expect(res.status).toBe(204);
    expect(await res.text()).toBe("");
  });

  it.each([
    "/v1/unknown",
    "/v2/apps",
    "/admin/v1/nothing",
    `/v1/apps/${IOS}/unknown`,
    `/v1/apps/${IOS}/audience/summary/extra`,
  ])("answers 404 not_found for unknown path %s", async (url) => {
    const res = await demoFetch(url, undefined, NOW);
    expect(res.status).toBe(404);
    expect(await res.json()).toEqual({
      error: { code: "not_found", message: "Не найдено", request_id: "demo" },
    });
  });

  it.each([
    `/v1/apps/${UNKNOWN_APP}`,
    `/v1/apps/${UNKNOWN_APP}/audience/summary?from=2026-10-01&to=2026-10-14`,
    `/v1/apps/${UNKNOWN_APP}/audience/timeseries?from=2026-10-01&to=2026-10-14`,
    `/v1/apps/${UNKNOWN_APP}/retention?from=2026-10-01&to=2026-10-14`,
    `/v1/apps/${UNKNOWN_APP}/realtime`,
    `/v1/apps/${UNKNOWN_APP}/profiles?q=abc`,
    `/v1/apps/${UNKNOWN_APP}/integration/health`,
    `/admin/v1/apps/${UNKNOWN_APP}/write-keys`,
  ])("answers 404 app_not_found for unknown app %s", async (url) => {
    const res = await demoFetch(url, undefined, NOW);
    expect(res.status).toBe(404);
    const error = await errorOf(res);
    expect(error.code).toBe("app_not_found");
    expect(error.request_id).toBe("demo");
  });

  it("answers 404 profile_not_found for an unknown person", async () => {
    const res = await demoFetch(
      `/v1/apps/${IOS}/profiles/${encodeURIComponent("u:nobody-at-all")}`,
      undefined,
      NOW,
    );
    expect(res.status).toBe(404);
    expect((await errorOf(res)).code).toBe("profile_not_found");
  });

  it("answers 404 not_found for an unknown deletion request", async () => {
    const res = await demoFetch(
      `/v1/apps/${IOS}/deletion-requests/0199d6d0-0000-7000-8000-000000000000`,
      undefined,
      NOW,
    );
    expect(res.status).toBe(404);
    expect((await errorOf(res)).code).toBe("not_found");
  });

  it.each([
    [
      "hour range over 7 days",
      `/v1/apps/${IOS}/audience/timeseries?from=2026-10-01&to=2026-10-08&granularity=hour`,
      "range_too_large",
      { field: "to", max_days: 7 },
    ],
    [
      "range over 366 days",
      `/v1/apps/${IOS}/audience/summary?from=2025-10-12&to=2026-10-14`,
      "range_too_large",
      { field: "to", max_days: 366 },
    ],
    [
      "from after to",
      `/v1/apps/${IOS}/audience/summary?from=2026-10-12&to=2026-10-06`,
      "invalid_request",
      { field: "from" },
    ],
    [
      "missing from",
      `/v1/apps/${IOS}/audience/rolling-active?to=2026-10-06`,
      "invalid_request",
      { field: "from" },
    ],
    [
      "bad date",
      `/v1/apps/${IOS}/audience/summary?from=2026-02-30&to=2026-03-06`,
      "invalid_request",
      { field: "from" },
    ],
    ["bad portfolio date", "/v1/portfolio?date=2026-13-01", "invalid_request", { field: "date" }],
    [
      "bad dimension",
      `/v1/apps/${IOS}/audience/breakdown?from=2026-10-01&to=2026-10-14&dimension=ip`,
      "invalid_request",
      { field: "dimension" },
    ],
    [
      "bad environment",
      `/v1/apps/${IOS}/realtime?environment=staging`,
      "invalid_request",
      { field: "environment" },
    ],
    [
      "bad country filter",
      `/v1/apps/${IOS}/audience/summary?from=2026-10-01&to=2026-10-14&country=ru`,
      "invalid_request",
      { field: "country" },
    ],
    [
      "too many retention periods",
      `/v1/apps/${IOS}/retention?from=2026-10-01&to=2026-10-14&periods=91`,
      "invalid_request",
      { field: "periods" },
    ],
    [
      "too many weekly periods",
      `/v1/apps/${IOS}/retention?from=2026-10-01&to=2026-10-14&cohort_granularity=week&periods=53`,
      "invalid_request",
      { field: "periods" },
    ],
    [
      "hours over 168",
      `/v1/apps/${IOS}/integration/health?hours=169`,
      "invalid_request",
      { field: "hours" },
    ],
    ["empty search", `/v1/apps/${IOS}/profiles?q=`, "invalid_request", { field: "q" }],
    [
      "bad person key",
      `/v1/apps/${IOS}/profiles/${encodeURIComponent("x:123")}`,
      "invalid_request",
      { field: "person_key" },
    ],
    [
      "bad sessions limit",
      `/v1/apps/${IOS}/profiles/${encodeURIComponent(iosProfile.personKey)}?sessions_limit=501`,
      "invalid_request",
      { field: "sessions_limit" },
    ],
    ["bad app id", "/v1/apps/not-a-uuid", "invalid_request", { field: "app_id" }],
  ])("answers 400 for %s", async (_name, url, code, details) => {
    const res = await demoFetch(url, undefined, NOW);
    expect(res.status).toBe(400);
    const error = await errorOf(res);
    expect(error.code).toBe(code);
    expect(error.request_id).toBe("demo");
    expect(error.message).not.toBe("");
    expect(error.details).toEqual(details);
  });
});
