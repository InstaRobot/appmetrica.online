import { describe, expect, it } from "vitest";
import type {
  ApiKeyList,
  AppList,
  AudienceSummary,
  Breakdown,
  DeletionRequestList,
  IntegrationHealth,
  Portfolio,
  Profile,
  ProfileSearchResult,
  Realtime,
  RetentionMatrix,
  RollingActive,
  TimeSeries,
  WriteKeyList,
} from "@/lib/api/gen/model";
import { addDays } from "@/lib/period";
import { DEMO_APP_IDS, DEMO_APPS } from "./apps";
import { demoFetch } from "./handler";
import { DEMO_PROFILES, demoProfileExamples } from "./profiles";

const NOW = new Date("2026-10-14T09:30:00Z"); // 12:30 MSK, среда
const TODAY = "2026-10-14";
const IOS = DEMO_APP_IDS.notesIos;
const ANDROID = DEMO_APP_IDS.notesAndroid;
const SITE = DEMO_APP_IDS.site;
const BOT = DEMO_APP_IDS.bot;
const OLD = DEMO_APP_IDS.oldGame;
const REJECTION_CODES = [
  "missing_field",
  "invalid_field",
  "unknown_event_name",
  "event_too_old",
  "user_id_looks_like_pii",
  "duration_required",
  "install_rate_limited",
];

async function get<T>(url: string, now: Date = NOW): Promise<T> {
  const res = await demoFetch(url, undefined, now);
  if (res.status !== 200) throw new Error(`${url}: HTTP ${res.status} ${await res.text()}`);
  return (await res.json()) as T;
}

function query(params: Record<string, string | string[]>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    for (const v of Array.isArray(value) ? value : [value]) q.append(key, v);
  }
  return q.toString();
}

function summary(appId: string, params: Record<string, string | string[]> = {}) {
  return get<AudienceSummary>(
    `/v1/apps/${appId}/audience/summary?${query({ from: "2026-10-07", to: "2026-10-13", ...params })}`,
  );
}

describe("demo apps", () => {
  it("exports five fixed apps in a fixed order", () => {
    expect(DEMO_APPS.map((a) => [a.name, a.platform, a.status])).toEqual([
      ["Notes iOS", "ios", "active"],
      ["Notes Android", "android", "active"],
      ["site.ru", "web", "active"],
      ["Помощник TG", "bot", "active"],
      ["Old Game", "ios", "archived"],
    ]);
    expect(DEMO_APPS.map((a) => a.id)).toEqual([IOS, ANDROID, SITE, BOT, OLD]);
    expect(DEMO_APPS[0]?.timezone).toBe("Europe/Moscow");
    expect(DEMO_APPS.find((a) => a.id === OLD)?.archived_at).not.toBeNull();
  });

  it("lists active apps sorted by name and archived ones on request", async () => {
    const active = await get<AppList>("/v1/apps");
    expect(active.items.map((a) => a.name)).toEqual([
      "Notes Android",
      "Notes iOS",
      "site.ru",
      "Помощник TG",
    ]);
    const all = await get<AppList>("/v1/apps?include_archived=true");
    expect(all.items).toHaveLength(5);
  });
});

describe("demo portfolio", () => {
  it("has a row per active app with a 30-day sparkline ending at the date", async () => {
    const p = await get<Portfolio>("/v1/portfolio");
    expect(p.date).toBe("2026-10-13"); // вчера в Europe/Moscow
    expect(p.environment).toBe("production");
    expect(p.unit).toBe("persons");
    expect(p.rows.map((r) => r.app.id).sort()).toEqual([IOS, ANDROID, SITE, BOT].sort());
    for (const row of p.rows) {
      expect(row.sparkline).toHaveLength(30);
      expect(row.sparkline[0]?.day).toBe(addDays(p.date, -29));
      expect(row.sparkline[29]?.day).toBe(p.date);
      expect(row.sparkline[29]?.active).toBe(row.active_date);
      expect(row.retention_d1).not.toBeNull();
      expect(row.retention_d7).not.toBeNull();
    }
    const actives = p.rows.map((r) => r.active_date);
    expect(actives).toEqual([...actives].sort((a, b) => b - a));
  });

  it("uses realistic scales for every app", async () => {
    const p = await get<Portfolio>("/v1/portfolio");
    const byId = new Map(p.rows.map((r) => [r.app.id, r]));
    const ranges: [string, number, number][] = [
      [IOS, 9_000, 15_000],
      [ANDROID, 6_500, 11_500],
      [SITE, 2_000, 4_200],
      [BOT, 650, 1_400],
    ];
    for (const [id, min, max] of ranges) {
      const row = byId.get(id);
      if (!row) throw new Error(id);
      expect(row.active_date).toBeGreaterThanOrEqual(min);
      expect(row.active_date).toBeLessThanOrEqual(max);
      expect(row.new_date / row.active_date).toBeGreaterThanOrEqual(0.07);
      expect(row.new_date / row.active_date).toBeLessThanOrEqual(0.13);
      expect(row.sessions_date / row.active_date).toBeGreaterThanOrEqual(1.9);
      expect(row.sessions_date / row.active_date).toBeLessThanOrEqual(2.8);
      // Сегодня к 12:30 набралась только часть дневной аудитории.
      expect(row.active_today).toBeLessThan(row.active_date);
      expect(row.active_today).toBeGreaterThan(0);
    }
  });

  it("adds the archived app with an empty recent history", async () => {
    const p = await get<Portfolio>("/v1/portfolio?include_archived=true");
    const old = p.rows.find((r) => r.app.id === OLD);
    expect(old?.app.status).toBe("archived");
    expect(old?.active_date).toBe(0);
    expect(old?.active_delta_pct).toBeNull();
  });
});

describe("demo audience", () => {
  it("counts installs 3–8 % above persons and development in tens", async () => {
    for (const id of [IOS, ANDROID, SITE, BOT]) {
      const persons = await summary(id);
      const installs = await summary(id, { unit: "installs" });
      const ratio = installs.active / persons.active;
      expect(ratio).toBeGreaterThanOrEqual(1.03);
      expect(ratio).toBeLessThanOrEqual(1.08);
      const day = await summary(id, { from: "2026-10-13", to: "2026-10-13" });
      const dev = await summary(id, {
        from: "2026-10-13",
        to: "2026-10-13",
        environment: "development",
      });
      expect(dev.active).toBeGreaterThan(0);
      expect(dev.active).toBeLessThan(100);
      expect(dev.active).toBeLessThan(day.active / 10);
    }
  });

  it("keeps sessions ≈ 2.3×, new ≈ 10 % and durations in 3–6 minutes", async () => {
    const s = await summary(IOS, { from: "2026-10-13", to: "2026-10-13" });
    expect(s.sessions / s.active).toBeGreaterThan(1.9);
    expect(s.sessions / s.active).toBeLessThan(2.8);
    expect(s.new / s.active).toBeGreaterThan(0.07);
    expect(s.new / s.active).toBeLessThan(0.13);
    expect(s.avg_duration_ms).toBeGreaterThanOrEqual(180_000);
    expect(s.avg_duration_ms).toBeLessThanOrEqual(360_000);
    expect(s.median_duration_ms).toBeLessThan(s.avg_duration_ms ?? 0);
  });

  it("compares with the previous period of the same length", async () => {
    const s = await summary(IOS);
    expect(s.compare?.from).toBe("2026-09-30");
    expect(s.compare?.to).toBe("2026-10-06");
    const base = s.compare?.active ?? 0;
    expect(s.compare?.delta_pct.active).toBe(Math.round(((s.active - base) / base) * 10000) / 100);
    const none = await summary(IOS, { compare: "none" });
    expect(none.compare).toBeUndefined();
  });

  it("shrinks numbers by filters in proportion to the breakdown share", async () => {
    const all = await summary(IOS);
    const android = await summary(IOS, { platform: ["android"] });
    expect(android.active).toBe(0);
    const ru = await summary(IOS, { country: ["RU"] });
    const breakdown = await get<Breakdown>(
      `/v1/apps/${IOS}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=country`,
    );
    const ruRow = breakdown.rows.find((r) => r.value === "RU");
    expect(ruRow).toBeDefined();
    expect(ru.active).toBeLessThan(all.active);
    expect(Math.abs(ru.active - (ruRow?.count ?? 0)) / ru.active).toBeLessThan(0.02);
    const ruKz = await summary(IOS, { country: ["RU", "KZ"] });
    expect(ruKz.active).toBeGreaterThan(ru.active);
  });

  it("returns breakdown rows with shares summing to about one", async () => {
    for (const dimension of [
      "platform",
      "os_name",
      "os_version",
      "app_version",
      "device_model",
      "device_type",
      "country",
      "city",
      "locale",
    ]) {
      for (const id of [IOS, SITE, BOT]) {
        const b = await get<Breakdown>(
          `/v1/apps/${id}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=${dimension}&limit=200`,
        );
        expect(b.dimension).toBe(dimension);
        expect(b.metric).toBe("active");
        expect(b.rows.length).toBeGreaterThan(0);
        const share = b.rows.reduce((sum, r) => sum + r.share, 0);
        expect(share, `${id} ${dimension}`).toBeGreaterThan(0.97);
        expect(share, `${id} ${dimension}`).toBeLessThan(1.03);
        const counts = b.rows.map((r) => r.count);
        expect(counts).toEqual([...counts].sort((x, y) => y - x));
        expect(new Set(b.rows.map((r) => r.value)).size).toBe(b.rows.length);
      }
    }
    const limited = await get<Breakdown>(
      `/v1/apps/${IOS}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=device_model&limit=3`,
    );
    expect(limited.rows).toHaveLength(3);
  });

  it("fills a daily series from from to to with zeros in the future", async () => {
    const ts = await get<TimeSeries>(
      `/v1/apps/${IOS}/audience/timeseries?from=2026-10-01&to=2026-10-20&granularity=day`,
    );
    expect(ts.metric).toBe("active");
    expect(ts.granularity).toBe("day");
    expect(ts.points).toHaveLength(20);
    expect(ts.points[0]?.ts).toBe("2026-10-01T00:00:00+03:00");
    expect(ts.compare_points).toHaveLength(20);
    expect(ts.compare_points?.[0]?.ts).toBe("2026-09-11T00:00:00+03:00");
    const future = ts.points.slice(14);
    expect(future.every((p) => p.value === 0)).toBe(true);
    const past = ts.points.slice(0, 13);
    expect(past.every((p) => (p.value ?? 0) > 8_000)).toBe(true);
    // Выходные ниже будней: 2026-10-03/04 — суббота и воскресенье.
    const value = (i: number) => ts.points[i]?.value ?? 0;
    expect(Math.max(value(2), value(3))).toBeLessThan(Math.min(value(0), value(1), value(5)));
  });

  it("builds hourly, weekly and monthly buckets in the app time zone", async () => {
    const hours = await get<TimeSeries>(
      `/v1/apps/${IOS}/audience/timeseries?from=2026-10-13&to=2026-10-14&granularity=hour&metric=sessions`,
    );
    expect(hours.points).toHaveLength(48);
    expect(hours.points[0]?.ts).toBe("2026-10-13T00:00:00+03:00");
    expect(hours.points[47]?.ts).toBe("2026-10-14T23:00:00+03:00");
    // 12:30 MSK: час 13:00 и позже — ещё не наступил.
    expect(hours.points.slice(24 + 13).every((p) => p.value === 0)).toBe(true);
    expect(hours.points[24 + 11]?.value).toBeGreaterThan(0);

    const weeks = await get<TimeSeries>(
      `/v1/apps/${IOS}/audience/timeseries?from=2026-09-10&to=2026-10-14&granularity=week&compare=none`,
    );
    expect(weeks.points[0]?.ts).toBe("2026-09-07T00:00:00+03:00"); // понедельник недели from
    expect(weeks.points).toHaveLength(6);
    expect(weeks.compare_points).toBeUndefined();

    const months = await get<TimeSeries>(
      `/v1/apps/${BOT}/audience/timeseries?from=2026-08-15&to=2026-10-14&granularity=month&metric=avg_duration_ms`,
    );
    expect(months.points.map((p) => p.ts)).toEqual([
      "2026-08-01T00:00:00+05:00",
      "2026-09-01T00:00:00+05:00",
      "2026-10-01T00:00:00+05:00",
    ]);
  });

  it("keeps past values independent of now", async () => {
    const url = `/v1/apps/${ANDROID}/audience/timeseries?from=2026-09-01&to=2026-09-30&compare=none`;
    const a = await get<TimeSeries>(url, NOW);
    const b = await get<TimeSeries>(url, new Date("2026-10-20T18:00:00Z"));
    expect(b).toEqual(a);
  });

  it("returns rolling active with dau ≤ wau ≤ mau", async () => {
    const r = await get<RollingActive>(
      `/v1/apps/${IOS}/audience/rolling-active?from=2026-09-14&to=2026-10-13`,
    );
    expect(r.points).toHaveLength(30);
    for (const p of r.points) {
      expect(p.dau).toBeLessThanOrEqual(p.wau);
      expect(p.wau).toBeLessThanOrEqual(p.mau);
      expect(p.wau / p.dau).toBeGreaterThan(2);
      expect(p.mau / p.dau).toBeGreaterThan(4);
    }
  });
});

describe("demo retention", () => {
  // Период N когорты завершён, если его последний день раньше сегодняшнего (OQ-045).
  function expectNullsOnlyInFuture(m: RetentionMatrix, step: number) {
    for (const cohort of m.cohorts) {
      expect(cohort.values).toHaveLength(m.periods + 1);
      cohort.values.forEach((v, n) => {
        const complete = addDays(cohort.cohort_start, step * (n + 1)) <= TODAY;
        if (complete && cohort.size > 0) {
          expect(v, `${cohort.cohort_start} D${n}`).not.toBeNull();
          expect(v).toBeGreaterThanOrEqual(0);
          expect(v).toBeLessThanOrEqual(1);
        } else {
          expect(v, `${cohort.cohort_start} D${n}`).toBeNull();
        }
      });
      const known = cohort.values.filter((v): v is number => v !== null);
      if (known.length > 0) expect(known[0]).toBe(1);
      expect(known).toEqual([...known].sort((a, b) => b - a));
    }
    expect(m.average).toHaveLength(m.periods + 1);
    for (const v of m.average) {
      if (v !== null) {
        expect(v).toBeGreaterThanOrEqual(0);
        expect(v).toBeLessThanOrEqual(1);
      }
    }
  }

  it("builds daily cohorts with nulls only for periods not yet complete", async () => {
    const m = await get<RetentionMatrix>(`/v1/apps/${IOS}/retention?from=2026-08-01&to=2026-10-20`);
    expect(m.cohort_granularity).toBe("day");
    expect(m.periods).toBe(30);
    expect(m.cohorts).toHaveLength(81);
    expectNullsOnlyInFuture(m, 1);
    const future = m.cohorts.find((c) => c.cohort_start === "2026-10-20");
    expect(future?.size).toBe(0);
    const d = (n: number) => m.average[n] ?? 0;
    expect(d(1)).toBeGreaterThanOrEqual(0.3);
    expect(d(1)).toBeLessThanOrEqual(0.4);
    expect(d(7)).toBeGreaterThanOrEqual(0.12);
    expect(d(7)).toBeLessThanOrEqual(0.16);
    expect(d(30)).toBeGreaterThanOrEqual(0.05);
    expect(d(30)).toBeLessThanOrEqual(0.08);
  });

  it("builds weekly cohorts from Mondays", async () => {
    const m = await get<RetentionMatrix>(
      `/v1/apps/${ANDROID}/retention?from=2026-07-08&to=2026-10-14&cohort_granularity=week`,
    );
    expect(m.cohort_granularity).toBe("week");
    expect(m.periods).toBe(12);
    expect(m.cohorts[0]?.cohort_start).toBe("2026-07-06");
    expect(m.cohorts.at(-1)?.cohort_start).toBe("2026-10-12");
    expectNullsOnlyInFuture(m, 7);
  });

  it("keeps web retention present but lower than mobile", async () => {
    const url = (id: string) => `/v1/apps/${id}/retention?from=2026-08-01&to=2026-09-30&periods=7`;
    const ios = await get<RetentionMatrix>(url(IOS));
    const site = await get<RetentionMatrix>(url(SITE));
    expectNullsOnlyInFuture(site, 1);
    expect(site.average[1]).not.toBeNull();
    expect(site.average[1] ?? 1).toBeLessThan(ios.average[1] ?? 0);
    expect(site.average[7] ?? 1).toBeLessThan(ios.average[7] ?? 0);
  });

  it("matches portfolio retention with the retention matrix", async () => {
    const p = await get<Portfolio>("/v1/portfolio?date=2026-10-12");
    const row = p.rows.find((r) => r.app.id === IOS);
    const m = await get<RetentionMatrix>(
      `/v1/apps/${IOS}/retention?from=2026-10-05&to=2026-10-11&periods=7`,
    );
    const d1 = m.cohorts.find((c) => c.cohort_start === "2026-10-11")?.values[1];
    const d7 = m.cohorts.find((c) => c.cohort_start === "2026-10-05")?.values[7];
    expect(row?.retention_d1).toBe(d1);
    expect(row?.retention_d7).toBe(d7);
  });
});

describe("demo realtime", () => {
  it("returns 30 full minutes and 50 recent events from the last minutes", async () => {
    const r = await get<Realtime>(`/v1/apps/${IOS}/realtime`);
    expect(r.generated_at).toBe(NOW.toISOString());
    expect(r.active_now).toBeGreaterThan(0);
    expect(r.per_minute).toHaveLength(30);
    expect(r.per_minute[0]?.minute).toBe("2026-10-14T12:00:00+03:00");
    expect(r.per_minute[29]?.minute).toBe("2026-10-14T12:29:00+03:00");
    expect(r.per_minute.some((m) => m.sessions_started > 0)).toBe(true);
    expect(r.recent_events).toHaveLength(50);
    const received = r.recent_events.map((e) => Date.parse(e.received_at));
    expect(received).toEqual([...received].sort((a, b) => b - a));
    for (const e of r.recent_events) {
      const t = Date.parse(e.received_at);
      expect(t).toBeLessThanOrEqual(NOW.getTime());
      expect(t).toBeGreaterThan(NOW.getTime() - 8 * 60_000);
      expect(Date.parse(e.event_time)).toBeLessThanOrEqual(t);
      expect(e.person_key).toMatch(/^(u:.+|i:[0-9a-f-]{36})$/);
      expect(e.environment).toBe("production");
      expect(e.platform).toBe("ios");
    }
    expect(new Set(r.recent_events.map((e) => e.insert_id)).size).toBe(50);
  });

  it("is quiet for development and empty for the archived app", async () => {
    const dev = await get<Realtime>(`/v1/apps/${IOS}/realtime?environment=development`);
    expect(dev.active_now).toBeLessThan(10);
    expect(dev.recent_events.every((e) => e.environment === "development")).toBe(true);
    const old = await get<Realtime>(`/v1/apps/${OLD}/realtime`);
    expect(old.active_now).toBe(0);
    expect(old.recent_events).toEqual([]);
    expect(old.per_minute.every((m) => m.sessions_started === 0)).toBe(true);
  });

  it("links recent events to profiles that open", async () => {
    const r = await get<Realtime>(`/v1/apps/${SITE}/realtime`);
    for (const e of r.recent_events.slice(0, 10)) {
      const res = await demoFetch(
        `/v1/apps/${SITE}/profiles/${encodeURIComponent(e.person_key)}`,
        undefined,
        NOW,
      );
      expect(res.status, e.person_key).toBe(200);
    }
  });
});

describe("demo integration health", () => {
  it("returns hourly totals, reasons and versions", async () => {
    for (const id of [IOS, ANDROID, SITE, BOT]) {
      const h = await get<IntegrationHealth>(`/v1/apps/${id}/integration/health`);
      expect(h.hours).toBe(24);
      expect(h.hourly).toHaveLength(24);
      // Последняя точка — текущий час (12:30 MSK) в TZ приложения.
      expect(h.hourly[23]?.hour).toMatch(/^2026-10-14T(12:00:00\+03:00|14:00:00\+05:00)$/);
      expect(h.first_event_at).not.toBeNull();
      expect(h.accepted_total).toBe(h.hourly.reduce((s, x) => s + x.accepted, 0));
      expect(h.rejected_total).toBe(h.hourly.reduce((s, x) => s + x.rejected, 0));
      expect(h.rejected_total).toBeGreaterThan(0);
      expect(h.rejected_total).toBeLessThan(h.accepted_total / 50);
      expect(h.reasons.length).toBeGreaterThanOrEqual(2);
      expect(h.reasons.length).toBeLessThanOrEqual(3);
      expect(h.reasons.reduce((s, r) => s + r.count, 0)).toBe(h.rejected_total);
      for (const reason of h.reasons) {
        expect(REJECTION_CODES).toContain(reason.code);
        const payload = JSON.parse(reason.example_payload) as Record<string, unknown>;
        expect(payload).not.toHaveProperty("ip");
        expect(JSON.stringify(payload)).not.toMatch(/"ip"/);
        expect(reason.example_message).not.toBe("");
      }
      expect(h.versions.length).toBeGreaterThanOrEqual(3);
      expect(h.versions.length).toBeLessThanOrEqual(4);
      expect(h.versions.reduce((s, v) => s + v.accepted, 0)).toBe(h.accepted_total);
    }
  });

  it("honours the hours parameter and the time zone of the app", async () => {
    const ios = await get<IntegrationHealth>(`/v1/apps/${IOS}/integration/health?hours=48`);
    expect(ios.hourly).toHaveLength(48);
    expect(ios.hourly[47]?.hour).toBe("2026-10-14T12:00:00+03:00");
    const bot = await get<IntegrationHealth>(`/v1/apps/${BOT}/integration/health`);
    expect(bot.hourly[23]?.hour).toBe("2026-10-14T14:00:00+05:00");
  });
});

describe("demo profiles", () => {
  it("exports a few searchable examples per active app", async () => {
    for (const id of [IOS, ANDROID, SITE, BOT]) {
      const examples = demoProfileExamples(id);
      expect(examples.length).toBeGreaterThanOrEqual(3);
      expect(examples.length).toBeLessThanOrEqual(5);
      for (const ex of examples) {
        const byInstall = await get<ProfileSearchResult>(
          `/v1/apps/${id}/profiles?q=${ex.installId}`,
        );
        expect(byInstall.items).toHaveLength(1);
        expect(byInstall.items[0]?.person_key).toBe(ex.personKey);
        expect(byInstall.items[0]?.matched_by).toBe("install_id");
        if (ex.userId !== null) {
          const byUser = await get<ProfileSearchResult>(
            `/v1/apps/${id}/profiles?q=${encodeURIComponent(ex.userId)}`,
          );
          expect(byUser.items[0]?.person_key).toBe(`u:${ex.userId}`);
          expect(byUser.items[0]?.matched_by).toBe("user_id");
        } else {
          expect(ex.personKey).toBe(`i:${ex.installId}`);
        }
      }
    }
    expect(DEMO_PROFILES.every((p) => DEMO_APPS.some((a) => a.id === p.appId))).toBe(true);
  });

  it("finds nothing for an unknown query", async () => {
    const r = await get<ProfileSearchResult>(`/v1/apps/${IOS}/profiles?q=nobody`);
    expect(r.items).toEqual([]);
  });

  it("pages sessions with before and next_before", async () => {
    const power = demoProfileExamples(IOS)[0];
    if (!power) throw new Error("no profile");
    const url = `/v1/apps/${IOS}/profiles/${encodeURIComponent(power.personKey)}`;
    const first = await get<Profile>(url);
    expect(first.person_key).toBe(power.personKey);
    expect(first.sessions).toHaveLength(100);
    expect(first.next_before).toBe(first.sessions[99]?.started_at);
    expect(first.installs.length).toBeGreaterThanOrEqual(1);
    const started = first.sessions.map((s) => Date.parse(s.started_at));
    expect(started).toEqual([...started].sort((a, b) => b - a));
    expect(Date.parse(first.last_seen)).toBeLessThanOrEqual(NOW.getTime());

    const second = await get<Profile>(
      `${url}?${query({ sessions_limit: "100", before: first.next_before ?? "" })}`,
    );
    const cursor = Date.parse(first.next_before ?? "");
    expect(second.sessions.length).toBeGreaterThan(0);
    expect(second.sessions.every((s) => Date.parse(s.started_at) < cursor)).toBe(true);
    const ids = new Set(first.sessions.map((s) => s.session_id));
    expect(second.sessions.some((s) => ids.has(s.session_id))).toBe(false);

    const big = await get<Profile>(`${url}?sessions_limit=500`);
    expect(big.sessions.length).toBeGreaterThan(100);
    expect(big.sessions.length).toBeLessThanOrEqual(500);
  });

  it("shows development sessions of the tester and 404 for linked installs", async () => {
    const examples = demoProfileExamples(IOS);
    const withDev = await Promise.all(
      examples.map((ex) =>
        get<Profile>(`/v1/apps/${IOS}/profiles/${encodeURIComponent(ex.personKey)}`),
      ),
    );
    expect(withDev.some((p) => p.sessions.some((s) => s.environment === "development"))).toBe(true);
    const linked = examples.find((ex) => ex.userId !== null);
    if (!linked) throw new Error("no linked profile");
    const res = await demoFetch(
      `/v1/apps/${IOS}/profiles/${encodeURIComponent(`i:${linked.installId}`)}`,
      undefined,
      NOW,
    );
    expect(res.status).toBe(404);
  });
});

describe("demo admin data", () => {
  it("uses obvious demo write keys and api key prefixes", async () => {
    const write = await get<WriteKeyList>(`/admin/v1/apps/${IOS}/write-keys`);
    expect(write.items.length).toBeGreaterThan(0);
    for (const k of write.items) {
      expect(k.key.startsWith("pk_demo")).toBe(true);
      expect(k.app_id).toBe(IOS);
    }
    const api = await get<ApiKeyList>("/admin/v1/api-keys");
    expect(api.items.length).toBeGreaterThan(0);
    for (const k of api.items) expect(k.key_prefix).toMatch(/^sk_[A-Za-z0-9]{8}$/);
    expect(JSON.stringify(api)).not.toMatch(/"secret"/);
  });

  it("filters deletion requests by app and opens each of them", async () => {
    const all = await get<DeletionRequestList>("/admin/v1/deletion-requests");
    expect(all.items.length).toBeGreaterThanOrEqual(3);
    const requested = all.items.map((r) => Date.parse(r.requested_at));
    expect(requested).toEqual([...requested].sort((a, b) => b - a));
    const ios = await get<DeletionRequestList>(`/admin/v1/deletion-requests?app_id=${IOS}`);
    expect(ios.items.length).toBeGreaterThan(0);
    expect(ios.items.every((r) => r.app_id === IOS)).toBe(true);
    for (const r of all.items) {
      const one = await get<{ id: string }>(`/v1/apps/${r.app_id}/deletion-requests/${r.id}`);
      expect(one.id).toBe(r.id);
    }
  });
});
