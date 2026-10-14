// Отчёты демо-режима в форме ответов Read API (06-api-contracts.md §2.4.3–2.4.5) поверх model.ts.
import type {
  AudienceSummary,
  Breakdown,
  BreakdownDimension,
  BreakdownMetric,
  BreakdownRow,
  CohortGranularity,
  Environment,
  Granularity,
  Portfolio,
  PortfolioRow,
  RetentionCohort,
  RetentionMatrix,
  RollingActive,
  TimeSeries,
  TimeSeriesMetric,
  TimeSeriesPoint,
  TimeSeriesSplit,
  Unit,
} from "@/lib/api/gen/model";
import { Platform } from "@/lib/api/gen/model";
import { addDays, rangeDays, todayIn } from "@/lib/period";
import { DEMO_APPS, DEMO_OWNER_TIMEZONE } from "./apps";
import {
  cohortReturned,
  dayStats,
  demoApp,
  dimValues,
  hourStats,
  makeScope,
  type PeriodStats,
  periodStats,
  type Scope,
  scopePlatforms,
  uniqueActive,
  withFilters,
} from "./model";
import {
  addMonths,
  eachDay,
  firstOfMonth,
  formatZoned,
  HOUR_MS,
  localTime,
  maxDay,
  minDay,
  mondayOf,
  zonedMidnight,
} from "./time";

// changePct — изменение в процентах с округлением до 10^-digits; null при пустой базе.
function changePct(cur: number | null, base: number | null, digits: number): number | null {
  if (cur === null || base === null || base === 0) return null;
  const k = 10 ** digits;
  return Math.round(((cur - base) / base) * 100 * k) / k;
}

function previousPeriod(from: string, to: string): [string, string] {
  const len = rangeDays(from, to);
  return [addDays(from, -len), addDays(from, -1)];
}

// --- портфель ---

function dayRetention(scope: Scope, cohort: string, n: number): number | null {
  if (addDays(cohort, n + 1) > scope.today) return null;
  const size = dayStats(scope, cohort).new;
  if (size === 0) return null;
  return (cohortReturned(scope, cohort, "day", n, size)[n] ?? 0) / size;
}

export function portfolio(
  now: number,
  q: { date: string | null; env: Environment; unit: Unit; includeArchived: boolean },
): Portfolio {
  const date = q.date ?? addDays(todayIn(DEMO_OWNER_TIMEZONE, new Date(now)), -1);
  const rows: PortfolioRow[] = [];
  for (const app of DEMO_APPS) {
    const demo = demoApp(app.id);
    if (!demo || (app.status === "archived" && !q.includeArchived)) continue;
    const scope = makeScope(demo, now, { env: q.env, unit: q.unit });
    const day = dayStats(scope, date);
    const prevWeek = dayStats(scope, addDays(date, -7)).active;
    rows.push({
      app: {
        id: app.id,
        name: app.name,
        platform: app.platform,
        platforms: app.platforms,
        status: app.status,
      },
      active_today: dayStats(scope, scope.today).active,
      active_date: day.active,
      active_same_day_prev_week: prevWeek,
      active_delta_pct: changePct(day.active, prevWeek, 1),
      new_date: day.new,
      sessions_date: day.sessions,
      retention_d1: dayRetention(scope, addDays(date, -1), 1),
      retention_d7: dayRetention(scope, addDays(date, -7), 7),
      sparkline: eachDay(addDays(date, -29), date).map((d) => ({
        day: d,
        active: dayStats(scope, d).active,
      })),
      platforms: breakdown(
        scope,
        date,
        date,
        "platform",
        "active",
        Object.keys(Platform).length,
      ).rows.map((r) => ({
        platform: r.value,
        active: r.count,
      })),
    });
  }
  rows.sort((a, b) => b.active_date - a.active_date || a.app.name.localeCompare(b.app.name));
  return { date, environment: q.env, unit: q.unit, rows };
}

// --- аудитория ---

export function audienceSummary(
  scope: Scope,
  from: string,
  to: string,
  compare: boolean,
): AudienceSummary {
  const cur = periodStats(scope, from, to);
  const out: AudienceSummary = {
    from,
    to,
    active: cur.active,
    new: cur.new,
    sessions: cur.sessions,
    avg_duration_ms: cur.avgMs,
    median_duration_ms: cur.medianMs,
  };
  if (compare) {
    const [pFrom, pTo] = previousPeriod(from, to);
    const prev = periodStats(scope, pFrom, pTo);
    out.compare = {
      from: pFrom,
      to: pTo,
      active: prev.active,
      new: prev.new,
      sessions: prev.sessions,
      avg_duration_ms: prev.avgMs,
      median_duration_ms: prev.medianMs,
      delta_pct: {
        active: changePct(cur.active, prev.active, 2),
        new: changePct(cur.new, prev.new, 2),
        sessions: changePct(cur.sessions, prev.sessions, 2),
        avg_duration_ms: changePct(cur.avgMs, prev.avgMs, 2),
        median_duration_ms: changePct(cur.medianMs, prev.medianMs, 2),
      },
    };
  }
  return out;
}

function pick(stats: PeriodStats, metric: TimeSeriesMetric): number | null {
  switch (metric) {
    case "active":
      return stats.active;
    case "new":
      return stats.new;
    case "sessions":
      return stats.sessions;
    case "avg_duration_ms":
      return stats.avgMs;
  }
}

type Bucket = { start: string; from: string; to: string };

// buckets — корзины week (ISO, с понедельника) и month (календарные) от корзины from до корзины to,
// значения обрезаются по from…to.
function buckets(from: string, to: string, granularity: "day" | "week" | "month"): Bucket[] {
  const out: Bucket[] = [];
  if (granularity === "day") {
    for (const d of eachDay(from, to)) out.push({ start: d, from: d, to: d });
    return out;
  }
  let start = granularity === "week" ? mondayOf(from) : firstOfMonth(from);
  while (start <= to) {
    const next = granularity === "week" ? addDays(start, 7) : addMonths(start, 1);
    out.push({ start, from: maxDay(start, from), to: minDay(addDays(next, -1), to) });
    start = next;
  }
  return out;
}

function series(
  scope: Scope,
  from: string,
  to: string,
  granularity: Granularity,
  metric: TimeSeriesMetric,
): TimeSeriesPoint[] {
  const tz = scope.demo.app.timezone;
  if (granularity === "hour") {
    const out: TimeSeriesPoint[] = [];
    const end = zonedMidnight(addDays(to, 1), tz);
    for (let t = zonedMidnight(from, tz); t < end; t += HOUR_MS) {
      const local = localTime(t, tz);
      const s = hourStats(scope, local.date, local.hour, t);
      out.push({
        ts: formatZoned(t, tz),
        value: pick(
          {
            active: s.active,
            new: s.new,
            sessions: s.sessions,
            avgMs: s.durationMs === null ? null : Math.round(s.durationMs * 10) / 10,
            medianMs: s.medianMs,
          },
          metric,
        ),
      });
    }
    return out;
  }
  return buckets(from, to, granularity).map((b) => ({
    ts: formatZoned(zonedMidnight(b.start, tz), tz),
    value: pick(periodStats(scope, b.from, b.to), metric),
  }));
}

// timeseries — ряд метрики; split — ещё ряд на каждую платформу, у которой в периоде есть сессии
// (по алфавиту, как ORDER BY platform в Go), без compare_points.
export function timeseries(
  scope: Scope,
  from: string,
  to: string,
  granularity: Granularity,
  metric: TimeSeriesMetric,
  compare: boolean,
  split = false,
): TimeSeries {
  const out: TimeSeries = {
    metric,
    granularity,
    points: series(scope, from, to, granularity, metric),
  };
  if (split) {
    out.splits = scopePlatforms(scope)
      .toSorted()
      .flatMap((platform): TimeSeriesSplit[] => {
        const one = withFilters(scope, { ...scope.filters, platform: [platform] });
        if (periodStats(one, from, to).sessions === 0) return [];
        return [{ platform, points: series(one, from, to, granularity, metric) }];
      });
  } else if (compare) {
    const [pFrom, pTo] = previousPeriod(from, to);
    out.compare_points = series(scope, pFrom, pTo, granularity, metric);
  }
  return out;
}

export function rollingActive(scope: Scope, from: string, to: string): RollingActive {
  return {
    points: eachDay(from, to).map((day) => {
      const dau = dayStats(scope, day).active;
      const wau = Math.max(dau, uniqueActive(scope, addDays(day, -6), day));
      const mau = Math.max(wau, uniqueActive(scope, addDays(day, -29), day));
      return { day, dau, wau, mau };
    }),
  };
}

// breakdown — строки измерения: значение строки добавляется к фильтрам, поэтому строка совпадает с
// отчётом под таким фильтром. Фильтр по самому измерению оставляет только выбранные значения, фильтр по
// родителю (страна для города) — значения внутри выбранных родителей.
export function breakdown(
  scope: Scope,
  from: string,
  to: string,
  dimension: BreakdownDimension,
  metric: BreakdownMetric,
  limit: number,
): Breakdown {
  const metricOf = (s: Scope) => {
    const p = periodStats(s, from, to);
    return metric === "active" ? p.active : metric === "new" ? p.new : p.sessions;
  };
  const total = metricOf(scope);
  const own = scope.filters[dimension] ?? [];
  const rows: BreakdownRow[] = dimValues(scope, dimension)
    .filter((value) => own.length === 0 || own.includes(value))
    .map((value) => ({
      value,
      count: metricOf(withFilters(scope, { ...scope.filters, [dimension]: [value] })),
      share: 0,
    }))
    .filter((r) => r.count > 0)
    .sort((a, b) => b.count - a.count || a.value.localeCompare(b.value))
    .slice(0, limit)
    .map((r) => ({
      ...r,
      share: total > 0 ? Math.min(1, Math.round((r.count / total) * 1e4) / 1e4) : 0,
    }));
  return { dimension, metric, total, rows };
}

// --- retention ---

// retention — классический retention когорт from…to (дни или ISO-недели). Период N когорты известен,
// если его последний день раньше сегодняшнего в TZ приложения; иначе null (как retentionMatrix в Go).
export function retention(
  scope: Scope,
  from: string,
  to: string,
  granularity: CohortGranularity,
  periods: number,
): RetentionMatrix {
  const step = granularity === "week" ? 7 : 1;
  const starts =
    granularity === "week" ? buckets(from, to, "week").map((b) => b.start) : eachDay(from, to);
  const returnedSum: number[] = new Array(periods + 1).fill(0);
  const sizeSum: number[] = new Array(periods + 1).fill(0);
  const cohorts: RetentionCohort[] = starts.map((start) => {
    let size = 0;
    for (let i = 0; i < step; i++) size += dayStats(scope, addDays(start, i)).new;
    const values: (number | null)[] = new Array(periods + 1).fill(null);
    if (size > 0) {
      const returned = cohortReturned(scope, start, granularity, periods, size);
      for (let n = 0; n <= periods; n++) {
        if (addDays(start, step * (n + 1)) > scope.today) break;
        const r = returned[n] ?? 0;
        values[n] = r / size;
        returnedSum[n] = (returnedSum[n] ?? 0) + r;
        sizeSum[n] = (sizeSum[n] ?? 0) + size;
      }
    }
    return { cohort_start: start, size, values };
  });
  return {
    cohort_granularity: granularity,
    periods,
    cohorts,
    average: returnedSum.map((r, n) => {
      const s = sizeSum[n] ?? 0;
      return s > 0 ? r / s : null;
    }),
  };
}
