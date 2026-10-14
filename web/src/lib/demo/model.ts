// Синтетическая модель метрик демо-приложения. День → active/new/sessions/длительности по ключу
// (приложение, environment, дата): плавный тренд, недельная сезонность и шум mulberry32. Фильтры
// уменьшают значения пропорционально доле значения в распределении измерения, unit=installs добавляет
// installsExtra, «сегодня» обрезается долей уже прошедшего дня в TZ приложения.
import type { App, BreakdownDimension, Environment, Unit } from "@/lib/api/gen/model";
import { todayIn } from "@/lib/period";
import { DEMO_APPS } from "./apps";
import { APP_MODELS, type AppModel, type Weights } from "./catalog";
import { jitter, pickWeighted, rand } from "./random";
import { dayDiff, eachDay, localTime, weekdayIndex } from "./time";

// Точка отсчёта тренда: значения прошлых дней не зависят от «сейчас».
const TREND_ANCHOR = "2026-10-14";
const HOURLY_UNIQUE_RATIO = 1.7;
const DEV_STICKINESS = 0.35;

export type DemoApp = { app: App; model: AppModel; firstDay: string; lastDay: string | null };

const demoApps = new Map<string, DemoApp>();
for (const app of DEMO_APPS) {
  const model = APP_MODELS[app.id];
  if (!model) continue;
  demoApps.set(app.id, {
    app,
    model,
    firstDay: localTime(Date.parse(app.created_at), app.timezone).date,
    lastDay: app.archived_at ? localTime(Date.parse(app.archived_at), app.timezone).date : null,
  });
}

export function demoApp(id: string): DemoApp | undefined {
  return demoApps.get(id);
}

export const FILTER_DIMS = ["platform", "app_version", "os_name", "country"] as const;
export type FilterDim = (typeof FILTER_DIMS)[number];
export type Filters = Readonly<Record<FilterDim, readonly string[]>>;
export const NO_FILTERS: Filters = { platform: [], app_version: [], os_name: [], country: [] };

export function isFilterDim(dim: string): dim is FilterDim {
  return (FILTER_DIMS as readonly string[]).includes(dim);
}

export type DayStats = {
  active: number;
  new: number;
  sessions: number;
  durationMs: number | null;
  medianMs: number | null;
};

const ZERO: DayStats = { active: 0, new: 0, sessions: 0, durationMs: null, medianMs: null };

export type Scope = {
  demo: DemoApp;
  env: Environment;
  unit: Unit;
  filters: Filters;
  now: number;
  today: string;
  cache: Map<string, DayStats>;
};

export function makeScope(
  demo: DemoApp,
  now: number,
  opts: { env?: Environment; unit?: Unit; filters?: Filters } = {},
): Scope {
  return {
    demo,
    env: opts.env ?? "production",
    unit: opts.unit ?? "persons",
    filters: opts.filters ?? NO_FILTERS,
    now,
    today: todayIn(demo.app.timezone, new Date(now)),
    cache: new Map(),
  };
}

export function withFilters(scope: Scope, filters: Filters): Scope {
  return { ...scope, filters, cache: new Map() };
}

// --- распределения измерений ---

export type DimEntry = { value: string; parent: string | null; weight: number };

const UNKNOWN: Weights = [["", 1]];
const entriesCache = new WeakMap<AppModel, Map<BreakdownDimension, DimEntry[]>>();

function normalized(weights: Weights): DimEntry[] {
  const total = weights.reduce((s, [, w]) => s + w, 0);
  return weights.map(([value, w]) => ({ value, parent: null, weight: total > 0 ? w / total : 0 }));
}

function childWeights(children: Readonly<Record<string, Weights>>, parent: string): Weights {
  return children[parent] ?? children["*"] ?? UNKNOWN;
}

// dimEntries — глобальные доли значений измерения (сумма 1); у дочерних измерений — по родителю.
export function dimEntries(model: AppModel, dim: BreakdownDimension): DimEntry[] {
  let byDim = entriesCache.get(model);
  if (!byDim) {
    byDim = new Map();
    entriesCache.set(model, byDim);
  }
  const cached = byDim.get(dim);
  if (cached) return cached;
  const spec = model.dims[dim];
  let out: DimEntry[];
  if ("weights" in spec) {
    out = normalized(spec.weights);
  } else {
    out = [];
    for (const p of dimEntries(model, spec.parent)) {
      for (const child of normalized(childWeights(spec.children, p.value))) {
        out.push({ value: child.value, parent: p.value, weight: p.weight * child.weight });
      }
    }
  }
  byDim.set(dim, out);
  return out;
}

export function dimParent(model: AppModel, dim: BreakdownDimension): FilterDim | null {
  const spec = model.dims[dim];
  return "parent" in spec ? spec.parent : null;
}

export function filterFactor(model: AppModel, filters: Filters): number {
  let factor = 1;
  for (const dim of FILTER_DIMS) {
    const selected = new Set(filters[dim]);
    if (selected.size === 0) continue;
    factor *= dimEntries(model, dim)
      .filter((e) => selected.has(e.value))
      .reduce((s, e) => s + e.weight, 0);
  }
  return factor;
}

export type InstallAttrs = Record<BreakdownDimension, string>;

const SAMPLE_ORDER: readonly BreakdownDimension[] = [
  "platform",
  "os_name",
  "country",
  "app_version",
  "os_version",
  "device_model",
  "device_type",
  "city",
  "locale",
];

// sampleAttrs — атрибуты одной установки из распределений приложения (город — из страны, модель — из ОС).
export function sampleAttrs(model: AppModel, next: () => number): InstallAttrs {
  const out: InstallAttrs = {
    platform: "",
    os_name: "",
    os_version: "",
    app_version: "",
    device_model: "",
    device_type: "",
    country: "",
    city: "",
    locale: "",
  };
  for (const dim of SAMPLE_ORDER) {
    const spec = model.dims[dim];
    const weights =
      "weights" in spec ? spec.weights : childWeights(spec.children, out[spec.parent]);
    out[dim] = pickWeighted(weights, next());
  }
  return out;
}

// --- суточные метрики ---

const shareCache = new Map<AppModel, number[]>();

// hourShares — доли суток по часам местного времени (сумма 1).
export function hourShares(model: AppModel): number[] {
  let shares = shareCache.get(model);
  if (!shares) {
    const total = model.hourly.reduce((s, w) => s + w, 0);
    shares = model.hourly.map((w) => w / total);
    shareCache.set(model, shares);
  }
  return shares;
}

// dayFraction — доля суточной активности, уже прошедшая к now в TZ приложения.
function dayFraction(scope: Scope): number {
  const t = localTime(scope.now, scope.demo.app.timezone);
  const shares = hourShares(scope.demo.model);
  let f = 0;
  for (let h = 0; h < t.hour; h++) f += shares[h] ?? 0;
  f += ((shares[t.hour] ?? 0) * (t.minute * 60 + t.second)) / 3600;
  return Math.min(1, f);
}

type RawDay = {
  active: number;
  new: number;
  sessions: number;
  durationMs: number;
  medianMs: number;
};

function rawDay(demo: DemoApp, env: Environment, day: string): RawDay | null {
  if (day < demo.firstDay || (demo.lastDay !== null && day > demo.lastDay)) return null;
  const { model } = demo;
  const id = demo.app.id;
  const wd = weekdayIndex(day);
  const weekend = wd >= 5;
  let active: number;
  if (env === "production") {
    const t = dayDiff(TREND_ANCHOR, day);
    const trend = Math.max(0.25, 1 + (model.growthPerYear * t) / 365);
    const wave = 1 + 0.035 * Math.sin((2 * Math.PI * t) / 47 + rand(id, "phase") * 2 * Math.PI);
    active =
      model.baseDau * trend * wave * (model.weekly[wd] ?? 1) * jitter(0.06, id, env, day, "a");
  } else {
    active = model.devDau * (weekend ? 0.35 : 1) * jitter(0.5, id, env, day, "a");
  }
  const [minNew, maxNew] = model.newShare;
  const newShare = minNew + (maxNew - minNew) * rand(id, env, day, "n");
  const durationMs =
    model.avgDurationSec * 1000 * jitter(0.1, id, env, day, "d") * (weekend ? 1.05 : 1);
  return {
    active,
    new: active * newShare,
    sessions: active * model.sessionsPerUser * jitter(0.08, id, env, day, "s"),
    durationMs,
    medianMs: durationMs * (0.46 + 0.06 * rand(id, env, day, "m")),
  };
}

// dayStats — метрики дня в scope; full — без обрезки «сегодня» (для почасовой раскладки).
export function dayStats(scope: Scope, day: string, full = false): DayStats {
  if (day > scope.today) return ZERO;
  const key = `${day}|${full}`;
  const cached = scope.cache.get(key);
  if (cached) return cached;
  const raw = rawDay(scope.demo, scope.env, day);
  let out = ZERO;
  if (raw) {
    const { model } = scope.demo;
    const factor = filterFactor(model, scope.filters);
    const unit = scope.unit === "installs" ? 1 + model.installsExtra : 1;
    const f = !full && day === scope.today ? dayFraction(scope) : 1;
    const active = Math.round(raw.active * f ** 0.75 * factor * unit);
    const sessions = Math.round(raw.sessions * f * factor);
    out = {
      active,
      new: Math.min(active, Math.round(raw.new * f * factor * unit)),
      sessions,
      durationMs: sessions > 0 ? raw.durationMs : null,
      medianMs: sessions > 0 ? raw.medianMs : null,
    };
  }
  scope.cache.set(key, out);
  return out;
}

// hourStats — метрики часа hour (местное время) дня day; start — начало часа, будущие часы пустые.
export function hourStats(scope: Scope, day: string, hour: number, start: number): DayStats {
  if (start > scope.now) return ZERO;
  const full = dayStats(scope, day, true);
  const share = hourShares(scope.demo.model)[hour] ?? 0;
  const elapsed = Math.min(1, (scope.now - start) / 3_600_000);
  const sessions = Math.round(full.sessions * share * elapsed);
  const id = scope.demo.app.id;
  return {
    active: Math.round(full.active * Math.min(1, HOURLY_UNIQUE_RATIO * share) * elapsed ** 0.75),
    new: Math.round(full.new * share * elapsed),
    sessions,
    durationMs:
      sessions > 0 && full.durationMs !== null
        ? full.durationMs * jitter(0.12, id, scope.env, day, hour, "hd")
        : null,
    medianMs:
      sessions > 0 && full.medianMs !== null
        ? full.medianMs * jitter(0.12, id, scope.env, day, hour, "hm")
        : null,
  };
}

// uniqueActive — уникальные за from…to: средний DAU дней с данными × n^stickiness (n = 1 → DAU).
export function uniqueActive(scope: Scope, from: string, to: string): number {
  let sum = 0;
  let n = 0;
  for (const day of eachDay(from, to)) {
    const a = dayStats(scope, day).active;
    if (a > 0) {
      sum += a;
      n++;
    }
  }
  if (n === 0) return 0;
  const p = scope.env === "production" ? scope.demo.model.stickiness : DEV_STICKINESS;
  return Math.round((sum / n) * n ** p);
}

export type PeriodStats = {
  active: number;
  new: number;
  sessions: number;
  avgMs: number | null;
  medianMs: number | null;
};

export function periodStats(scope: Scope, from: string, to: string): PeriodStats {
  let created = 0;
  let sessions = 0;
  let durSum = 0;
  let medSum = 0;
  for (const day of eachDay(from, to)) {
    const s = dayStats(scope, day);
    created += s.new;
    sessions += s.sessions;
    durSum += (s.durationMs ?? 0) * s.sessions;
    medSum += (s.medianMs ?? 0) * s.sessions;
  }
  return {
    active: uniqueActive(scope, from, to),
    new: created,
    sessions,
    avgMs: sessions > 0 ? Math.round((durSum / sessions) * 10) / 10 : null,
    medianMs: sessions > 0 ? Math.round(medSum / sessions) : null,
  };
}

// --- retention ---

// retentionRates — доли когорты D0…Dperiods (W0…): степенная кривая через D1 и D30 (W1 и W12) с шумом,
// не возрастающая по периодам. Значение периода N зависит только от когорты и N.
export function retentionRates(
  scope: Scope,
  cohortStart: string,
  granularity: "day" | "week",
  periods: number,
): number[] {
  const { d1, d30, w1, w12 } = scope.demo.model.retention;
  const [first, last, n] = granularity === "day" ? [d1, d30, 30] : [w1, w12, 12];
  const decay = Math.log(first / last) / Math.log(n);
  const id = scope.demo.app.id;
  const out = [1];
  for (let p = 1; p <= periods; p++) {
    const noisy = first * p ** -decay * jitter(0.12, id, scope.env, granularity, cohortStart, p);
    out.push(Math.min(out[p - 1] ?? 1, noisy));
  }
  return out;
}

// cohortReturned — число вернувшихся в периоды 0…periods для когорты размера size.
export function cohortReturned(
  scope: Scope,
  cohortStart: string,
  granularity: "day" | "week",
  periods: number,
  size: number,
): number[] {
  return retentionRates(scope, cohortStart, granularity, periods).map((r) => Math.round(size * r));
}
