import { afterEach, beforeEach, describe, expect, it } from "vitest";
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
const NOTES = DEMO_APP_IDS.notes;
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

// Демо отвечает на языке страницы (<html lang>, lib/demo/texts.ts); проверки ниже написаны по-русски.
beforeEach(() => {
  document.documentElement.lang = "ru";
});
afterEach(() => {
  document.documentElement.lang = "";
});

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

// pearson — коэффициент корреляции Пирсона двух рядов одной длины.
function pearson(a: readonly number[], b: readonly number[]): number {
  const mean = (x: readonly number[]) => x.reduce((s, v) => s + v, 0) / x.length;
  const ma = mean(a);
  const mb = mean(b);
  let cov = 0;
  let va = 0;
  let vb = 0;
  a.forEach((x, i) => {
    const y = b[i] ?? 0;
    cov += (x - ma) * (y - mb);
    va += (x - ma) ** 2;
    vb += (y - mb) ** 2;
  });
  return cov / Math.sqrt(va * vb);
}

function values(points: readonly { value: number | null }[]): number[] {
  return points.map((p) => p.value ?? 0);
}

describe("demo apps", () => {
  it("exports four fixed apps in a fixed order", () => {
    expect(DEMO_APPS.map((a) => [a.name, a.platforms, a.status])).toEqual([
      ["Notes", ["ios", "android", "web"], "active"],
      ["site.ru", ["web"], "active"],
      ["TG Helper", ["bot"], "active"],
      ["Old Game", ["ios"], "archived"],
    ]);
    expect(DEMO_APPS.map((a) => a.id)).toEqual([NOTES, SITE, BOT, OLD]);
    for (const app of DEMO_APPS) expect(app.platform).toBe(app.platforms[0]);
    expect(DEMO_APPS[0]?.timezone).toBe("Europe/Moscow");
    expect(DEMO_APPS.find((a) => a.id === OLD)?.archived_at).not.toBeNull();
  });

  it("lists active apps sorted by name and archived ones on request", async () => {
    const active = await get<AppList>("/v1/apps");
    expect(active.items.map((a) => a.name)).toEqual(["Notes", "site.ru", "TG Helper"]);
    const all = await get<AppList>("/v1/apps?include_archived=true");
    expect(all.items).toHaveLength(4);
  });
});

describe("demo portfolio", () => {
  it("has a row per active app with a 30-day sparkline ending at the date", async () => {
    const p = await get<Portfolio>("/v1/portfolio");
    expect(p.date).toBe("2026-10-13"); // вчера в Europe/Moscow
    expect(p.environment).toBe("production");
    expect(p.unit).toBe("persons");
    expect(p.rows.map((r) => r.app.id).sort()).toEqual([NOTES, SITE, BOT].sort());
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

  it("splits active people of the date by platform", async () => {
    const p = await get<Portfolio>("/v1/portfolio");
    for (const row of p.rows.filter((r) => r.app.id !== NOTES)) {
      expect(row.platforms).toEqual([{ platform: row.app.platform, active: row.active_date }]);
    }
    const notes = p.rows.find((r) => r.app.id === NOTES);
    expect(notes?.platforms.map((x) => x.platform)).toEqual(["ios", "android", "web"]);
    const sum = notes?.platforms.reduce((s, x) => s + x.active, 0) ?? 0;
    // Люди сразу на нескольких платформах: сумма долей больше итога.
    expect(sum).toBeGreaterThan(notes?.active_date ?? 0);
    for (const x of notes?.platforms ?? []) expect(x.active).toBeLessThan(notes?.active_date ?? 0);

    // До запуска веб-версии у «Notes» только мобильные платформы.
    const july = await get<Portfolio>("/v1/portfolio?date=2026-07-15");
    const before = july.rows.find((r) => r.app.id === NOTES);
    expect(before?.platforms.map((x) => x.platform)).toEqual(["ios", "android"]);
  });

  it("uses realistic scales for every app", async () => {
    const p = await get<Portfolio>("/v1/portfolio");
    const byId = new Map(p.rows.map((r) => [r.app.id, r]));
    const ranges: [string, number, number][] = [
      [NOTES, 17_000, 25_000],
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
    expect(old?.platforms).toEqual([]);
    expect(old?.active_delta_pct).toBeNull();
  });
});

describe("demo audience", () => {
  it("counts installs 3–8 % above persons and development in tens", async () => {
    const platformOf: Record<string, string[]> = { [NOTES]: ["ios"], [SITE]: [], [BOT]: [] };
    for (const id of [NOTES, SITE, BOT]) {
      const platform = platformOf[id] ?? [];
      const persons = await summary(id, { platform });
      const installs = await summary(id, { unit: "installs", platform });
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
    // У «Notes» установки на разных платформах — разные устройства одних людей.
    const persons = await summary(NOTES);
    const installs = await summary(NOTES, { unit: "installs" });
    expect(installs.active / persons.active).toBeGreaterThan(1.1);
    expect(installs.active / persons.active).toBeLessThan(1.2);
  });

  it("keeps sessions ≈ 2.3×, new ≈ 10 % and durations in 3–6 minutes", async () => {
    const s = await summary(NOTES, { from: "2026-10-13", to: "2026-10-13" });
    expect(s.sessions / s.active).toBeGreaterThan(1.9);
    expect(s.sessions / s.active).toBeLessThan(2.8);
    expect(s.new / s.active).toBeGreaterThan(0.07);
    expect(s.new / s.active).toBeLessThan(0.13);
    expect(s.avg_duration_ms).toBeGreaterThanOrEqual(180_000);
    expect(s.avg_duration_ms).toBeLessThanOrEqual(360_000);
    expect(s.median_duration_ms).toBeLessThan(s.avg_duration_ms ?? 0);
  });

  it("compares with the previous period of the same length", async () => {
    const s = await summary(NOTES);
    expect(s.compare?.from).toBe("2026-09-30");
    expect(s.compare?.to).toBe("2026-10-06");
    const base = s.compare?.active ?? 0;
    expect(s.compare?.delta_pct.active).toBe(Math.round(((s.active - base) / base) * 10000) / 100);
    const none = await summary(NOTES, { compare: "none" });
    expect(none.compare).toBeUndefined();
  });

  it("shrinks numbers by filters in proportion to the breakdown share", async () => {
    const all = await summary(NOTES);
    const ru = await summary(NOTES, { country: ["RU"] });
    const breakdown = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=country`,
    );
    const ruRow = breakdown.rows.find((r) => r.value === "RU");
    expect(ruRow).toBeDefined();
    expect(ru.active).toBeLessThan(all.active);
    expect(Math.abs(ru.active - (ruRow?.count ?? 0)) / ru.active).toBeLessThan(0.02);
    const ruKz = await summary(NOTES, { country: ["RU", "KZ"] });
    expect(ruKz.active).toBeGreaterThan(ru.active);
    const iosRu = await summary(NOTES, { platform: ["ios"], country: ["RU"] });
    expect(iosRu.active).toBeGreaterThan(0);
    expect(iosRu.active).toBeLessThan(ru.active);
  });

  it("platform filter returns the segment", async () => {
    const june = { from: "2026-06-01", to: "2026-06-30" };
    const october = { from: "2026-10-01", to: "2026-10-13" };
    const share = async (period: Record<string, string>) => {
      const all = await summary(NOTES, period);
      const android = await summary(NOTES, { ...period, platform: ["android"] });
      expect(android.active).toBeGreaterThan(0);
      expect(android.active).toBeLessThan(all.active);
      return android.active / all.active;
    };
    // Множитель дал бы одну и ту же долю: Android ровный, iOS растёт, веб запущен в августе.
    expect(Math.abs((await share(june)) - (await share(october)))).toBeGreaterThan(0.05);

    const web = await summary(NOTES, { ...june, platform: ["web"] });
    expect(web.active).toBe(0);
    expect(web.sessions).toBe(0);
    const mobile = await summary(NOTES, { ...october, platform: ["ios", "android"] });
    const ios = await summary(NOTES, { ...october, platform: ["ios"] });
    const android = await summary(NOTES, { ...october, platform: ["android"] });
    expect(mobile.active).toBeLessThan(ios.active + android.active);
    expect(mobile.sessions).toBe(ios.sessions + android.sessions);
    const site = await summary(SITE, { platform: ["android"] });
    expect(site.active).toBe(0);
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
      for (const id of [NOTES, SITE, BOT]) {
        const b = await get<Breakdown>(
          `/v1/apps/${id}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=${dimension}&limit=200`,
        );
        expect(b.dimension).toBe(dimension);
        expect(b.metric).toBe("active");
        expect(b.rows.length).toBeGreaterThan(0);
        const share = b.rows.reduce((sum, r) => sum + r.share, 0);
        // По платформам люди «Notes» с двух платформ попадают в обе строки, по версиям — люди,
        // обновившиеся за период.
        const max =
          (id === NOTES && dimension === "platform") || dimension === "app_version" ? 1.12 : 1.03;
        expect(share, `${id} ${dimension}`).toBeGreaterThan(0.97);
        expect(share, `${id} ${dimension}`).toBeLessThan(max);
        const counts = b.rows.map((r) => r.count);
        expect(counts).toEqual([...counts].sort((x, y) => y - x));
        expect(new Set(b.rows.map((r) => r.value)).size).toBe(b.rows.length);
      }
    }
    const limited = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=device_model&limit=3`,
    );
    expect(limited.rows).toHaveLength(3);
  });

  it("breaks Notes down by platform as the platform filter does", async () => {
    const b = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=platform`,
    );
    expect(b.rows.map((r) => r.value)).toEqual(["ios", "android", "web"]);
    for (const row of b.rows) {
      const one = await summary(NOTES, { platform: [row.value] });
      expect(row.count, row.value).toBe(one.active);
    }
    const filtered = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=platform&platform=web`,
    );
    expect(filtered.rows.map((r) => r.value)).toEqual(["web"]);
    const june = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-06-01&to=2026-06-30&dimension=platform`,
    );
    expect(june.rows.map((r) => r.value)).toEqual(["ios", "android"]);
    // Распределения измерений у каждого сегмента свои: Windows только в вебе.
    const webOs = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=os_name&platform=web`,
    );
    expect(webOs.rows.map((r) => r.value)).toContain("Windows");
    const iosOs = await get<Breakdown>(
      `/v1/apps/${NOTES}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=os_name&platform=ios`,
    );
    expect(iosOs.rows.map((r) => r.value).sort()).toEqual(["iOS", "iPadOS"]);
  });

  it("fills a daily series from from to to with zeros in the future", async () => {
    const ts = await get<TimeSeries>(
      `/v1/apps/${NOTES}/audience/timeseries?from=2026-10-01&to=2026-10-20&granularity=day`,
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
    expect(past.every((p) => (p.value ?? 0) > 15_000)).toBe(true);
    // Выходные ниже будней: 2026-10-03/04 — суббота и воскресенье.
    const value = (i: number) => ts.points[i]?.value ?? 0;
    expect(Math.max(value(2), value(3))).toBeLessThan(Math.min(value(0), value(1), value(5)));
  });

  it("builds hourly, weekly and monthly buckets in the app time zone", async () => {
    const hours = await get<TimeSeries>(
      `/v1/apps/${NOTES}/audience/timeseries?from=2026-10-13&to=2026-10-14&granularity=hour&metric=sessions`,
    );
    expect(hours.points).toHaveLength(48);
    expect(hours.points[0]?.ts).toBe("2026-10-13T00:00:00+03:00");
    expect(hours.points[47]?.ts).toBe("2026-10-14T23:00:00+03:00");
    // 12:30 MSK: час 13:00 и позже — ещё не наступил.
    expect(hours.points.slice(24 + 13).every((p) => p.value === 0)).toBe(true);
    expect(hours.points[24 + 11]?.value).toBeGreaterThan(0);

    const weeks = await get<TimeSeries>(
      `/v1/apps/${NOTES}/audience/timeseries?from=2026-09-10&to=2026-10-14&granularity=week&compare=none`,
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
    const url = `/v1/apps/${NOTES}/audience/timeseries?from=2026-09-01&to=2026-09-30&split=platform`;
    const a = await get<TimeSeries>(url, NOW);
    const b = await get<TimeSeries>(url, new Date("2026-10-20T18:00:00Z"));
    expect(b).toEqual(a);
  });

  it("segments of Notes have their own shape", async () => {
    const ts = await get<TimeSeries>(
      `/v1/apps/${NOTES}/audience/timeseries?from=2026-07-01&to=2026-10-13&metric=active&split=platform`,
    );
    expect(ts.compare_points).toBeUndefined();
    expect(ts.splits?.map((s) => s.platform)).toEqual(["android", "ios", "web"]);
    const split = (p: string) => ts.splits?.find((s) => s.platform === p)?.points ?? [];
    for (const p of ["android", "ios", "web"]) {
      expect(split(p).map((x) => x.ts)).toEqual(ts.points.map((x) => x.ts));
    }
    const ios = values(split("ios"));
    const android = values(split("android"));
    const web = values(split("web"));
    expect(pearson(ios, web)).toBeLessThan(0.9);
    ts.points.forEach((point, i) => {
      const day = point.ts.slice(0, 10);
      if (day < "2026-08-14") {
        expect(web[i], day).toBe(0);
        return;
      }
      expect(web[i], day).toBeGreaterThan(0);
      const sum = (ios[i] ?? 0) + (android[i] ?? 0) + (web[i] ?? 0);
      const total = point.value ?? 0;
      // Пересечение людей на нескольких платформах — 5–10 % суммы.
      expect(total, day).toBeLessThan(sum * 0.95);
      expect(total, day).toBeGreaterThan(sum * 0.9);
    });

    // Ряд платформы совпадает с рядом при фильтре platform=[p].
    const filtered = await get<TimeSeries>(
      `/v1/apps/${NOTES}/audience/timeseries?from=2026-07-01&to=2026-10-13&metric=active&platform=web`,
    );
    expect(filtered.points).toEqual(split("web"));
  });

  it("splits only the platforms seen in the period and under the filters", async () => {
    const base = `/v1/apps/${NOTES}/audience/timeseries`;
    const june = await get<TimeSeries>(`${base}?from=2026-06-01&to=2026-06-30&split=platform`);
    expect(june.splits?.map((s) => s.platform)).toEqual(["android", "ios"]);
    const ios = await get<TimeSeries>(
      `${base}?from=2026-10-01&to=2026-10-13&split=platform&platform=ios&metric=sessions`,
    );
    expect(ios.splits?.map((s) => s.platform)).toEqual(["ios"]);
    expect(ios.splits?.[0]?.points).toEqual(ios.points);
    const hours = await get<TimeSeries>(
      `${base}?from=2026-10-14&to=2026-10-14&granularity=hour&split=platform`,
    );
    expect(hours.splits?.map((s) => s.platform)).toEqual(["android", "ios", "web"]);
    expect(hours.splits?.every((s) => s.points.length === 24)).toBe(true);
    const site = await get<TimeSeries>(
      `/v1/apps/${SITE}/audience/timeseries?from=2026-10-01&to=2026-10-13&split=platform`,
    );
    expect(site.splits).toEqual([{ platform: "web", points: site.points }]);
    const future = await get<TimeSeries>(`${base}?from=2026-10-20&to=2026-10-25&split=platform`);
    expect(future.splits).toEqual([]);
    const none = await get<TimeSeries>(`${base}?from=2026-10-01&to=2026-10-13&split=none`);
    expect(none.splits).toBeUndefined();
    expect(none.compare_points).toHaveLength(13);
  });

  it("returns rolling active with dau ≤ wau ≤ mau", async () => {
    const r = await get<RollingActive>(
      `/v1/apps/${NOTES}/audience/rolling-active?from=2026-09-14&to=2026-10-13`,
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
    const m = await get<RetentionMatrix>(
      `/v1/apps/${NOTES}/retention?from=2026-08-01&to=2026-10-20`,
    );
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
      `/v1/apps/${NOTES}/retention?from=2026-07-08&to=2026-10-14&cohort_granularity=week`,
    );
    expect(m.cohort_granularity).toBe("week");
    expect(m.periods).toBe(12);
    expect(m.cohorts[0]?.cohort_start).toBe("2026-07-06");
    expect(m.cohorts.at(-1)?.cohort_start).toBe("2026-10-12");
    expectNullsOnlyInFuture(m, 7);
  });

  it("keeps web retention present but lower than mobile", async () => {
    const url = (id: string) => `/v1/apps/${id}/retention?from=2026-08-01&to=2026-09-30&periods=7`;
    const notes = await get<RetentionMatrix>(url(NOTES));
    const site = await get<RetentionMatrix>(url(SITE));
    expectNullsOnlyInFuture(site, 1);
    expect(site.average[1]).not.toBeNull();
    expect(site.average[1] ?? 1).toBeLessThan(notes.average[1] ?? 0);
    expect(site.average[7] ?? 1).toBeLessThan(notes.average[7] ?? 0);
    // Веб-сегмент «Notes» удерживает хуже мобильных.
    const segment = (p: string) => get<RetentionMatrix>(`${url(NOTES)}&platform=${p}`);
    const ios = await segment("ios");
    const web = await segment("web");
    expect(web.average[7] ?? 1).toBeLessThan(ios.average[7] ?? 0);
  });

  it("matches portfolio retention with the retention matrix", async () => {
    const p = await get<Portfolio>("/v1/portfolio?date=2026-10-12");
    const row = p.rows.find((r) => r.app.id === NOTES);
    const m = await get<RetentionMatrix>(
      `/v1/apps/${NOTES}/retention?from=2026-10-05&to=2026-10-11&periods=7`,
    );
    const d1 = m.cohorts.find((c) => c.cohort_start === "2026-10-11")?.values[1];
    const d7 = m.cohorts.find((c) => c.cohort_start === "2026-10-05")?.values[7];
    expect(row?.retention_d1).toBe(d1);
    expect(row?.retention_d7).toBe(d7);
  });
});

describe("demo realtime", () => {
  it("returns 30 full minutes and 50 recent events from the last minutes", async () => {
    const r = await get<Realtime>(`/v1/apps/${NOTES}/realtime`);
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
      expect(["ios", "android", "web"]).toContain(e.platform);
    }
    expect(new Set(r.recent_events.map((e) => e.platform)).size).toBeGreaterThanOrEqual(2);
    expect(new Set(r.recent_events.map((e) => e.insert_id)).size).toBe(50);
  });

  it("is quiet for development and empty for the archived app", async () => {
    const dev = await get<Realtime>(`/v1/apps/${NOTES}/realtime?environment=development`);
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
    for (const id of [NOTES, SITE, BOT]) {
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
    const ios = await get<IntegrationHealth>(`/v1/apps/${NOTES}/integration/health?hours=48`);
    expect(ios.hourly).toHaveLength(48);
    expect(ios.hourly[47]?.hour).toBe("2026-10-14T12:00:00+03:00");
    const bot = await get<IntegrationHealth>(`/v1/apps/${BOT}/integration/health`);
    expect(bot.hourly[23]?.hour).toBe("2026-10-14T14:00:00+05:00");
  });
});

describe("demo stories", () => {
  const breakdownUrl = (id: string, params: Record<string, string | string[]>) =>
    `/v1/apps/${id}/audience/breakdown?${query({ limit: "50", ...params })}`;

  function shareOf(b: Breakdown, value: string): number {
    return b.rows.find((r) => r.value === value)?.share ?? 0;
  }

  function mean(xs: readonly number[]): number {
    return xs.reduce((s, x) => s + x, 0) / xs.length;
  }

  async function daily(id: string, from: string, to: string, params = {}): Promise<number[]> {
    const ts = await get<TimeSeries>(
      `/v1/apps/${id}/audience/timeseries?${query({ from, to, compare: "none", ...params })}`,
    );
    return values(ts.points);
  }

  async function hourly(id: string, day: string, params = {}): Promise<number[]> {
    const ts = await get<TimeSeries>(
      `/v1/apps/${id}/audience/timeseries?${query({ from: day, to: day, granularity: "hour", compare: "none", ...params })}`,
    );
    return values(ts.points);
  }

  it("new versions replace old ones", async () => {
    const versions = (day: string) =>
      get<Breakdown>(
        breakdownUrl(NOTES, { from: day, to: day, dimension: "app_version", platform: "ios" }),
      );
    const oct13 = await versions("2026-10-13");
    expect(shareOf(oct13, "3.5.0")).toBeGreaterThanOrEqual(0.6);
    expect(shareOf(oct13, "3.3.0")).toBeLessThanOrEqual(0.05);
    const oct7 = await versions("2026-10-07");
    expect(shareOf(oct7, "3.5.0")).toBeGreaterThan(0);
    expect(shareOf(oct7, "3.5.0")).toBeLessThanOrEqual(0.25);
    // До выхода версии её нет в отчёте; хотфикс быстро вытесняет 3.4.0.
    const sep20 = await versions("2026-09-20");
    expect(sep20.rows.map((r) => r.value)).not.toContain("3.5.0");
    expect(shareOf(sep20, "3.4.1")).toBeGreaterThan(shareOf(sep20, "3.4.0"));
    const aug1 = await versions("2026-08-01");
    expect(aug1.rows.map((r) => r.value)).not.toContain("3.3.0");
    // Веб обновляется при перезагрузке страницы: через день после релиза почти все на новой версии.
    const web = await get<Breakdown>(
      breakdownUrl(NOTES, {
        from: "2026-10-08",
        to: "2026-10-08",
        dimension: "app_version",
        platform: "web",
      }),
    );
    expect(shareOf(web, "3.5.0")).toBeGreaterThan(0.8);
  });

  it("the outage shows up hourly", async () => {
    const incident = await hourly(NOTES, "2026-09-23");
    const usual = await hourly(NOTES, "2026-09-16");
    expect(incident).toHaveLength(24);
    for (const h of [10, 11, 12, 13]) {
      expect(incident[h] ?? 1, `${h}:00`).toBeLessThan(0.05 * (usual[h] ?? 0));
    }
    // Офлайн-очередь досылает события после восстановления приёма.
    for (const h of [14, 15]) expect(incident[h] ?? 0, `${h}:00`).toBeGreaterThan(usual[h] ?? 0);
    for (const h of [8, 9, 17, 18]) {
      expect(Math.abs((incident[h] ?? 0) / (usual[h] ?? 1) - 1), `${h}:00`).toBeLessThan(0.2);
    }
    // Ingest недоступен для всех приложений: у бота (UTC+5) те же моменты — 12:00–15:00.
    const bot = await hourly(BOT, "2026-09-23", { metric: "sessions" });
    const botUsual = await hourly(BOT, "2026-09-16", { metric: "sessions" });
    for (const h of [12, 13, 14, 15]) {
      expect(bot[h] ?? 1, `bot ${h}:00`).toBeLessThan(0.05 * (botUsual[h] ?? 0));
    }
    // Здоровье интеграции видит провал принятых событий.
    const health = await get<IntegrationHealth>(
      `/v1/apps/${NOTES}/integration/health?hours=24`,
      new Date("2026-09-23T20:30:00Z"),
    );
    const accepted = (hour: string) =>
      health.hourly.find((x) => x.hour === `2026-09-23T${hour}:00:00+03:00`)?.accepted ?? -1;
    expect(accepted("11")).toBeLessThan(0.05 * accepted("09"));
    expect(accepted("14")).toBeGreaterThan(accepted("09"));
  });

  it("campaign cohorts retain worse", async () => {
    const android = { platform: "android", metric: "new" };
    const before = await daily(NOTES, "2026-05-13", "2026-05-19", android);
    const campaign = await daily(NOTES, "2026-05-20", "2026-05-24", android);
    expect(mean(campaign)).toBeGreaterThanOrEqual(1.4 * mean(before));
    // Кампания в RU не меняет новых из других стран.
    const byBefore = await daily(NOTES, "2026-05-13", "2026-05-19", { ...android, country: "BY" });
    const byCampaign = await daily(NOTES, "2026-05-20", "2026-05-24", {
      ...android,
      country: "BY",
    });
    expect(mean(byCampaign)).toBeGreaterThan(0);
    expect(mean(byCampaign)).toBeLessThan(1.2 * mean(byBefore));

    const m = await get<RetentionMatrix>(
      `/v1/apps/${NOTES}/retention?from=2026-05-13&to=2026-05-31&periods=7&platform=android`,
    );
    const d = (from: string, to: string, n: number) =>
      mean(
        m.cohorts
          .filter((c) => c.cohort_start >= from && c.cohort_start <= to)
          .map((c) => c.values[n] ?? 0),
      );
    const neighbours = (n: number) =>
      (d("2026-05-13", "2026-05-19", n) + d("2026-05-25", "2026-05-31", n)) / 2;
    expect(d("2026-05-20", "2026-05-24", 7)).toBeLessThanOrEqual(0.7 * neighbours(7));
    expect(d("2026-05-20", "2026-05-24", 1)).toBeLessThanOrEqual(0.7 * neighbours(1));

    // Кампания в Казахстане на iOS: новые из KZ резко растут.
    const kz = { platform: "ios", country: "KZ", metric: "new" };
    const kzIosBefore = await daily(NOTES, "2026-09-08", "2026-09-14", kz);
    const kzIos = await daily(NOTES, "2026-09-15", "2026-09-19", kz);
    expect(mean(kzIos)).toBeGreaterThan(2 * mean(kzIosBefore));
  });

  it("the buggy release is visible", async () => {
    const health = (now: string) =>
      get<IntegrationHealth>(`/v1/apps/${NOTES}/integration/health?hours=96`, new Date(now));
    const invalidShare = (h: IntegrationHealth) =>
      (h.reasons.find((r) => r.code === "invalid_field")?.count ?? 0) / h.accepted_total;
    const buggy = await health("2026-09-12T09:00:00Z"); // окно 2026-09-08…2026-09-12 MSK
    const fixed = await health("2026-09-16T09:00:00Z");
    expect(invalidShare(buggy)).toBeGreaterThan(3 * invalidShare(fixed));
    const invalid = buggy.reasons.find((r) => r.code === "invalid_field");
    expect(invalid?.example_message).toContain("context.app_version");
    const payload = JSON.parse(invalid?.example_payload ?? "{}") as {
      context?: { app_version?: unknown };
    };
    expect(typeof payload.context?.app_version).not.toBe("string");
    expect(buggy.versions.find((v) => v.app_version === "3.4.0")?.rejected).toBeGreaterThan(0);

    // Сессии 3.4.0 до хотфикса короче на 25–35 %, у хотфикса — как обычно.
    const avg = async (from: string, to: string, version: string) =>
      (await summary(NOTES, { from, to, platform: ["ios"], app_version: [version] }))
        .avg_duration_ms ?? 0;
    const bug =
      (await avg("2026-09-08", "2026-09-11", "3.4.0")) /
      (await avg("2026-09-08", "2026-09-11", "3.3.0"));
    expect(bug).toBeGreaterThanOrEqual(0.65);
    expect(bug).toBeLessThanOrEqual(0.75);
    const hotfix =
      (await avg("2026-09-13", "2026-09-17", "3.4.1")) /
      (await avg("2026-09-13", "2026-09-17", "3.3.0"));
    expect(Math.abs(hotfix - 1)).toBeLessThan(0.1);

    // Всплеск отказов у site.ru 2026-10-02.
    const site = (now: string) =>
      get<IntegrationHealth>(`/v1/apps/${SITE}/integration/health?hours=24`, new Date(now));
    expect(invalidShare(await site("2026-10-02T20:30:00Z"))).toBeGreaterThan(
      5 * invalidShare(await site("2026-10-05T20:30:00Z")),
    );
  });

  it("seasons and the Kazakhstan launch shape the year", async () => {
    const android = { platform: "android" };
    const july = mean(await daily(NOTES, "2026-07-06", "2026-07-26", android));
    const autumn = mean(await daily(NOTES, "2026-09-21", "2026-10-11", android));
    expect(july).toBeLessThan(0.92 * autumn);
    const newYear = mean(await daily(NOTES, "2025-12-31", "2026-01-06", android));
    const december = mean(await daily(NOTES, "2025-12-10", "2025-12-16", android));
    expect(newYear).toBeGreaterThan(1.08 * december);
    const may = mean(await daily(NOTES, "2026-05-04", "2026-05-10", android));
    const april = mean(await daily(NOTES, "2026-04-20", "2026-04-26", android));
    expect(may).toBeLessThan(0.95 * april);

    const countries = (day: string) =>
      get<Breakdown>(
        breakdownUrl(NOTES, { from: day, to: day, dimension: "country", platform: "ios" }),
      );
    expect((await countries("2026-05-25")).rows.map((r) => r.value)).not.toContain("KZ");
    const ramp = shareOf(await countries("2026-06-10"), "KZ");
    const settled = shareOf(await countries("2026-08-10"), "KZ");
    expect(ramp).toBeGreaterThan(0);
    expect(ramp).toBeLessThan(settled);
    expect(settled).toBeGreaterThanOrEqual(0.07);
    expect(settled).toBeLessThanOrEqual(0.1);
  });
});

describe("demo profiles", () => {
  it("exports a few searchable examples per active app", async () => {
    for (const id of [NOTES, SITE, BOT]) {
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
    const r = await get<ProfileSearchResult>(`/v1/apps/${NOTES}/profiles?q=nobody`);
    expect(r.items).toEqual([]);
  });

  it("pages sessions with before and next_before", async () => {
    const power = demoProfileExamples(NOTES)[0];
    if (!power) throw new Error("no profile");
    const url = `/v1/apps/${NOTES}/profiles/${encodeURIComponent(power.personKey)}`;
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
    const examples = demoProfileExamples(NOTES);
    const withDev = await Promise.all(
      examples.map((ex) =>
        get<Profile>(`/v1/apps/${NOTES}/profiles/${encodeURIComponent(ex.personKey)}`),
      ),
    );
    expect(withDev.some((p) => p.sessions.some((s) => s.environment === "development"))).toBe(true);
    const linked = examples.find((ex) => ex.userId !== null);
    if (!linked) throw new Error("no linked profile");
    const res = await demoFetch(
      `/v1/apps/${NOTES}/profiles/${encodeURIComponent(`i:${linked.installId}`)}`,
      undefined,
      NOW,
    );
    expect(res.status).toBe(404);
  });

  it("has people of Notes on several platforms whose sessions respect the web launch", async () => {
    const power = demoProfileExamples(NOTES)[0];
    if (!power) throw new Error("no profile");
    const profile = await get<Profile>(
      `/v1/apps/${NOTES}/profiles/${encodeURIComponent(power.personKey)}?sessions_limit=500`,
    );
    expect(new Set(profile.installs.map((i) => i.platform))).toEqual(new Set(["ios", "web"]));
    const web = profile.sessions.filter((s) => s.platform === "web");
    expect(web.length).toBeGreaterThan(0);
    expect(web.every((s) => s.started_at >= "2026-08-13T21:00:00.000Z")).toBe(true);
  });
});

describe("demo admin data", () => {
  it("uses obvious demo write keys and api key prefixes", async () => {
    const write = await get<WriteKeyList>(`/admin/v1/apps/${NOTES}/write-keys`);
    expect(write.items.length).toBeGreaterThan(0);
    for (const k of write.items) {
      expect(k.key.startsWith("pk_demo")).toBe(true);
      expect(k.app_id).toBe(NOTES);
    }
    // Ключ на каждую платформу приложения с меткой, как при создании.
    const active = write.items.filter((k) => k.revoked_at === null);
    expect(active.map((k) => [k.platform, k.label])).toEqual([
      ["ios", "iOS"],
      ["android", "Android"],
      ["web", "Веб"],
    ]);
    const site = await get<WriteKeyList>(`/admin/v1/apps/${SITE}/write-keys`);
    expect(site.items.map((k) => [k.platform, k.label])).toEqual([["web", "Веб"]]);
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
    const notes = await get<DeletionRequestList>(`/admin/v1/deletion-requests?app_id=${NOTES}`);
    expect(notes.items.length).toBeGreaterThan(0);
    expect(notes.items.every((r) => r.app_id === NOTES)).toBe(true);
    for (const r of all.items) {
      const one = await get<{ id: string }>(`/v1/apps/${r.app_id}/deletion-requests/${r.id}`);
      expect(one.id).toBe(r.id);
    }
  });
});

describe("demo texts in English", () => {
  beforeEach(() => {
    document.documentElement.lang = "en";
  });

  it("labels keys and failed deletions in English", async () => {
    const write = await get<WriteKeyList>(`/admin/v1/apps/${NOTES}/write-keys`);
    expect(write.items.map((k) => k.label)).toEqual([
      "iOS",
      "Android",
      "Web",
      "Old build (leaked into a log)",
    ]);
    const bot = await get<WriteKeyList>(`/admin/v1/apps/${BOT}/write-keys`);
    expect(bot.items.map((k) => k.label)).toEqual(["Bot"]);
    const api = await get<ApiKeyList>("/admin/v1/api-keys");
    expect(api.items.map((k) => k.label)).toEqual([
      "AppMetrica reconciliation",
      "On-request data deletion",
      "Old cron report",
    ]);
    const deletions = await get<DeletionRequestList>("/admin/v1/deletion-requests");
    expect(deletions.items.filter((r) => r.error !== null).map((r) => r.error)).toEqual([
      "ClickHouse: timeout after 5 attempts",
    ]);
    expect(JSON.stringify([write, bot, api, deletions])).not.toMatch(/[А-Яа-яЁё]/);
  });

  it("explains rejections in English", async () => {
    for (const id of [NOTES, SITE, BOT, OLD]) {
      const h = await get<IntegrationHealth>(`/v1/apps/${id}/integration/health?hours=168`);
      for (const reason of h.reasons) {
        expect(reason.example_message, reason.code).not.toBe("");
        expect(reason.example_message, reason.code).not.toMatch(/[А-Яа-яЁё]/);
      }
    }
    const buggy = await get<IntegrationHealth>(
      `/v1/apps/${NOTES}/integration/health?hours=96`,
      new Date("2026-09-12T09:00:00Z"),
    );
    expect(buggy.reasons.find((r) => r.code === "invalid_field")?.example_message).toBe(
      "context.app_version must be a string.",
    );
  });
});
