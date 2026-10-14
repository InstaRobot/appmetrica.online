// Синтетическая модель метрик демо-приложения. Приложение — набор сегментов-платформ (catalog.ts): у
// каждого своя дневная серия по ключу (приложение, платформа, environment, дата) — тренд, рост после
// запуска, недельная сезонность и шум mulberry32. Метрики приложения — сумма сегментов; фильтры
// уменьшают каждый сегмент на долю выбранных значений в его распределениях. Люди сразу на нескольких
// платформах (crossPlatformShare) делают итог unit=persons меньше суммы сегментов; unit=installs
// добавляет installsExtra, «сегодня» обрезается долей уже прошедшего дня в TZ приложения.
import type { App, BreakdownDimension, Environment, Platform, Unit } from "@/lib/api/gen/model";
import { addDays, todayIn } from "@/lib/period";
import { DEMO_APPS } from "./apps";
import {
  APP_MODELS,
  type AppModel,
  type ChildWeights,
  DIM_PARENT,
  type ParentDim,
  type SegmentModel,
  type Weights,
} from "./catalog";
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

export function segmentOf(demo: DemoApp, platform: string): SegmentModel | undefined {
  return demo.model.segments.find((s) => s.platform === platform);
}

export type FilterDim = "platform" | "app_version" | "os_name" | "country";
// Filters — выбранные значения измерений: фильтры отчёта и строка breakdown поверх них.
export type Filters = Readonly<Partial<Record<BreakdownDimension, readonly string[]>>>;
export const NO_FILTERS: Filters = {};

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
  // Доля аудитории каждого сегмента под фильтрами; null — платформа сегмента не выбрана.
  shares: readonly (number | null)[];
  now: number;
  today: string;
  cache: Map<string, SegmentDay[]>;
};

export function makeScope(
  demo: DemoApp,
  now: number,
  opts: { env?: Environment; unit?: Unit; filters?: Filters } = {},
): Scope {
  const filters = opts.filters ?? NO_FILTERS;
  return {
    demo,
    env: opts.env ?? "production",
    unit: opts.unit ?? "persons",
    filters,
    shares: demo.model.segments.map((seg) => segmentShare(seg, filters)),
    now,
    today: todayIn(demo.app.timezone, new Date(now)),
    cache: new Map(),
  };
}

export function withFilters(scope: Scope, filters: Filters): Scope {
  return {
    ...scope,
    filters,
    shares: scope.demo.model.segments.map((seg) => segmentShare(seg, filters)),
    cache: new Map(),
  };
}

// scopePlatforms — платформы сегментов, которые проходят фильтр platform, в порядке каталога.
export function scopePlatforms(scope: Scope): Platform[] {
  return scope.demo.model.segments
    .filter((_, i) => scope.shares[i] !== null)
    .map((s) => s.platform);
}

// --- распределения измерений ---

export type DimEntry = { value: string; parent: string | null; weight: number };

const UNKNOWN: Weights = [["", 1]];
const entriesCache = new WeakMap<SegmentModel, Map<BreakdownDimension, DimEntry[]>>();

function normalized(weights: Weights): DimEntry[] {
  const total = weights.reduce((s, [, w]) => s + w, 0);
  return weights.map(([value, w]) => ({ value, parent: null, weight: total > 0 ? w / total : 0 }));
}

function childWeights(children: ChildWeights, parent: string): Weights {
  return children[parent] ?? children["*"] ?? UNKNOWN;
}

// dimEntries — доли значений измерения в сегменте (сумма 1); у дочерних измерений — по родителю.
export function dimEntries(seg: SegmentModel, dim: BreakdownDimension): DimEntry[] {
  let byDim = entriesCache.get(seg);
  if (!byDim) {
    byDim = new Map();
    entriesCache.set(seg, byDim);
  }
  const cached = byDim.get(dim);
  if (cached) return cached;
  let out: DimEntry[];
  if (dim === "platform") {
    out = [{ value: seg.platform, parent: null, weight: 1 }];
  } else if (dim === "os_name" || dim === "country" || dim === "app_version") {
    out = normalized(seg.dims[dim]);
  } else {
    const parent = DIM_PARENT[dim] ?? "os_name";
    out = [];
    for (const p of dimEntries(seg, parent)) {
      for (const child of normalized(childWeights(seg.dims[dim], p.value))) {
        out.push({ value: child.value, parent: p.value, weight: p.weight * child.weight });
      }
    }
  }
  byDim.set(dim, out);
  return out;
}

type ChildDim = "os_version" | "device_model" | "device_type" | "city" | "locale";

const CHILDREN: Readonly<Record<ParentDim, readonly ChildDim[]>> = {
  os_name: ["os_version", "device_model", "device_type"],
  country: ["city", "locale"],
};

function chosenShare(entries: readonly DimEntry[], chosen: readonly string[]): number {
  return entries.filter((e) => chosen.includes(e.value)).reduce((s, e) => s + e.weight, 0);
}

// groupShare — доля сегмента по родителю (ОС, страна) и его дочерним измерениям: дочерние значения
// независимы внутри значения родителя.
function groupShare(seg: SegmentModel, filters: Filters, parent: ParentDim): number {
  const own = filters[parent] ?? [];
  const children = CHILDREN[parent].filter((c) => (filters[c] ?? []).length > 0);
  if (own.length === 0 && children.length === 0) return 1;
  let share = 0;
  for (const p of dimEntries(seg, parent)) {
    if (own.length > 0 && !own.includes(p.value)) continue;
    let w = p.weight;
    for (const c of children) {
      w *= chosenShare(normalized(childWeights(seg.dims[c], p.value)), filters[c] ?? []);
    }
    share += w;
  }
  return share;
}

function segmentShare(seg: SegmentModel, filters: Filters): number | null {
  const platforms = filters.platform ?? [];
  if (platforms.length > 0 && !platforms.includes(seg.platform)) return null;
  const versions = filters.app_version ?? [];
  return (
    groupShare(seg, filters, "os_name") *
    groupShare(seg, filters, "country") *
    (versions.length > 0 ? chosenShare(dimEntries(seg, "app_version"), versions) : 1)
  );
}

// dimValues — значения измерения у сегментов scope (без повторов).
export function dimValues(scope: Scope, dim: BreakdownDimension): string[] {
  const out = new Set<string>();
  scope.demo.model.segments.forEach((seg, i) => {
    if (scope.shares[i] === null) return;
    for (const e of dimEntries(seg, dim)) out.add(e.value);
  });
  return [...out];
}

// versionShares — доли версий приложения по всем сегментам (вес сегмента — его базовый DAU).
export function versionShares(model: AppModel): DimEntry[] {
  const total = model.segments.reduce((s, seg) => s + seg.baseDau, 0);
  const byVersion = new Map<string, number>();
  for (const seg of model.segments) {
    for (const e of dimEntries(seg, "app_version")) {
      byVersion.set(e.value, (byVersion.get(e.value) ?? 0) + (e.weight * seg.baseDau) / total);
    }
  }
  return [...byVersion]
    .map(([value, weight]) => ({ value, parent: null, weight }))
    .sort((a, b) => b.weight - a.weight || b.value.localeCompare(a.value));
}

export type InstallAttrs = Record<BreakdownDimension, string>;

const SAMPLE_ORDER: readonly Exclude<BreakdownDimension, "platform">[] = [
  "os_name",
  "country",
  "app_version",
  "os_version",
  "device_model",
  "device_type",
  "city",
  "locale",
];

// sampleAttrs — атрибуты одной установки сегмента (город — из страны, модель — из ОС).
export function sampleAttrs(seg: SegmentModel, next: () => number): InstallAttrs {
  const out: InstallAttrs = {
    platform: seg.platform,
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
    const parent = DIM_PARENT[dim];
    const spec = seg.dims[dim];
    const weights = parent ? childWeights(spec as ChildWeights, out[parent]) : (spec as Weights);
    out[dim] = pickWeighted(weights, next());
  }
  return out;
}

// --- суточные метрики ---

const shareCache = new WeakMap<SegmentModel, number[]>();

// hourShares — доли суток сегмента по часам местного времени (сумма 1).
export function hourShares(seg: SegmentModel): number[] {
  let shares = shareCache.get(seg);
  if (!shares) {
    const total = seg.hourly.reduce((s, w) => s + w, 0);
    shares = seg.hourly.map((w) => w / total);
    shareCache.set(seg, shares);
  }
  return shares;
}

// dayFraction — доля суточной активности сегмента, уже прошедшая к now в TZ приложения.
function dayFraction(scope: Scope, seg: SegmentModel): number {
  const t = localTime(scope.now, scope.demo.app.timezone);
  const shares = hourShares(seg);
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

const rawCache = new Map<string, RawDay | null>();

function rawDay(demo: DemoApp, seg: SegmentModel, env: Environment, day: string): RawDay | null {
  const id = `${demo.app.id}:${seg.platform}`;
  const key = `${id}|${env}|${day}`;
  const cached = rawCache.get(key);
  if (cached !== undefined) return cached;
  let out: RawDay | null = null;
  if (day >= demo.firstDay && day >= seg.launch && (demo.lastDay === null || day <= demo.lastDay)) {
    const wd = weekdayIndex(day);
    const weekend = wd >= 5;
    const ramp =
      seg.rampDays > 0 ? 1 - Math.exp(-(dayDiff(seg.launch, day) + 1) / seg.rampDays) : 1;
    let active: number;
    if (env === "production") {
      const t = dayDiff(TREND_ANCHOR, day);
      const trend = Math.max(0.25, 1 + (seg.growthPerYear * t) / 365);
      const wave = 1 + 0.035 * Math.sin((2 * Math.PI * t) / 47 + rand(id, "phase") * 2 * Math.PI);
      active =
        seg.baseDau * trend * ramp * wave * (seg.weekly[wd] ?? 1) * jitter(0.06, id, env, day, "a");
    } else {
      active = seg.devDau * ramp * (weekend ? 0.35 : 1) * jitter(0.5, id, env, day, "a");
    }
    const [minNew, maxNew] = seg.newShare;
    // Пока платформа набирает аудиторию, новых больше обычного.
    const newShare = (minNew + (maxNew - minNew) * rand(id, env, day, "n")) * (2 - ramp);
    const durationMs =
      seg.avgDurationSec * 1000 * jitter(0.1, id, env, day, "d") * (weekend ? 1.05 : 1);
    out = {
      active,
      new: active * Math.min(1, newShare),
      sessions: active * seg.sessionsPerUser * jitter(0.08, id, env, day, "s"),
      durationMs,
      medianMs: durationMs * (0.46 + 0.06 * rand(id, env, day, "m")),
    };
  }
  rawCache.set(key, out);
  return out;
}

// SegmentDay — сегмент за день в scope: all — активные платформы без остальных фильтров (для
// пересечения людей), active/new — с фильтрами и unit, sessions — дробные.
type SegmentDay = {
  seg: SegmentModel;
  all: number;
  active: number;
  new: number;
  sessions: number;
  durationMs: number;
  medianMs: number;
};

function segmentDays(scope: Scope, day: string, full: boolean): SegmentDay[] {
  const key = `${day}|${full}`;
  const cached = scope.cache.get(key);
  if (cached) return cached;
  const out: SegmentDay[] = [];
  scope.demo.model.segments.forEach((seg, i) => {
    const share = scope.shares[i];
    if (share === null || share === undefined) return;
    const raw = rawDay(scope.demo, seg, scope.env, day);
    if (!raw) return;
    const f = !full && day === scope.today ? dayFraction(scope, seg) : 1;
    const unit = scope.unit === "installs" ? 1 + seg.installsExtra : 1;
    const all = raw.active * f ** 0.75;
    out.push({
      seg,
      all,
      active: all * share * unit,
      new: raw.new * f * share * unit,
      sessions: raw.sessions * f * share,
      durationMs: raw.durationMs,
      medianMs: raw.medianMs,
    });
  });
  scope.cache.set(key, out);
  return out;
}

// personsRatio — итог по людям / сумма сегментов: люди меньших платформ частично активны и на
// крупнейшей. Для одного сегмента и для unit=installs — 1.
function personsRatio(scope: Scope, all: readonly number[]): number {
  if (scope.unit === "installs" || all.length < 2) return 1;
  const sum = all.reduce((s, v) => s + v, 0);
  if (sum <= 0) return 1;
  return 1 - (scope.demo.model.crossPlatformShare * (sum - Math.max(...all))) / sum;
}

type Part = {
  all: number;
  active: number;
  new: number;
  sessions: number;
  dur: number;
  med: number;
};

function combine(scope: Scope, parts: readonly Part[]): DayStats {
  if (parts.length === 0) return ZERO;
  const ratio = personsRatio(
    scope,
    parts.map((p) => p.all),
  );
  let active = 0;
  let created = 0;
  let sessions = 0;
  let durSum = 0;
  let medSum = 0;
  for (const p of parts) {
    active += p.active;
    created += p.new;
    sessions += p.sessions;
    durSum += p.dur * p.sessions;
    medSum += p.med * p.sessions;
  }
  const people = Math.round(active * ratio);
  return {
    active: people,
    new: Math.min(people, Math.round(created * ratio)),
    sessions,
    durationMs: sessions > 0 ? durSum / sessions : null,
    medianMs: sessions > 0 ? medSum / sessions : null,
  };
}

// dayStats — метрики дня в scope; full — без обрезки «сегодня» (для почасовой раскладки).
export function dayStats(scope: Scope, day: string, full = false): DayStats {
  if (day > scope.today) return ZERO;
  return combine(
    scope,
    segmentDays(scope, day, full).map((d) => ({
      all: d.all,
      active: d.active,
      new: d.new,
      sessions: Math.round(d.sessions),
      dur: d.durationMs,
      med: d.medianMs,
    })),
  );
}

// hourStats — метрики часа hour (местное время) дня day; start — начало часа, будущие часы пустые.
export function hourStats(scope: Scope, day: string, hour: number, start: number): DayStats {
  if (start > scope.now) return ZERO;
  const elapsed = Math.min(1, (scope.now - start) / 3_600_000);
  const unique = elapsed ** 0.75;
  const out = combine(
    scope,
    segmentDays(scope, day, true).map((d) => {
      const share = hourShares(d.seg)[hour] ?? 0;
      const k = Math.min(1, HOURLY_UNIQUE_RATIO * share) * unique;
      return {
        all: d.all * k,
        active: d.active * k,
        new: d.new * share * elapsed,
        sessions: Math.round(d.sessions * share * elapsed),
        dur: d.durationMs,
        med: d.medianMs,
      };
    }),
  );
  const id = scope.demo.app.id;
  return {
    ...out,
    durationMs:
      out.durationMs === null
        ? null
        : out.durationMs * jitter(0.12, id, scope.env, day, hour, "hd"),
    medianMs:
      out.medianMs === null ? null : out.medianMs * jitter(0.12, id, scope.env, day, hour, "hm"),
  };
}

// hourSessions — ожидаемое (дробное) число сессий часа hour дня day целиком.
export function hourSessions(scope: Scope, day: string, hour: number): number {
  return segmentDays(scope, day, true).reduce(
    (s, d) => s + d.sessions * (hourShares(d.seg)[hour] ?? 0),
    0,
  );
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

// segmentRates — доли когорты сегмента D0…Dperiods (W0…): степенная кривая через D1 и D30 (W1 и W12)
// с шумом, не возрастающая по периодам. Значение периода N зависит только от когорты и N.
function segmentRates(
  scope: Scope,
  seg: SegmentModel,
  cohortStart: string,
  granularity: "day" | "week",
  periods: number,
): number[] {
  const { d1, d30, w1, w12 } = seg.retention;
  const [first, last, n] = granularity === "day" ? [d1, d30, 30] : [w1, w12, 12];
  const decay = Math.log(first / last) / Math.log(n);
  const id = `${scope.demo.app.id}:${seg.platform}`;
  const out = [1];
  for (let p = 1; p <= periods; p++) {
    const noisy = first * p ** -decay * jitter(0.12, id, scope.env, granularity, cohortStart, p);
    out.push(Math.min(out[p - 1] ?? 1, noisy));
  }
  return out;
}

// retentionRates — доли когорты приложения: кривые сегментов, взвешенные новыми когорты в сегменте.
export function retentionRates(
  scope: Scope,
  cohortStart: string,
  granularity: "day" | "week",
  periods: number,
): number[] {
  const step = granularity === "week" ? 7 : 1;
  const weights = new Map<SegmentModel, number>();
  for (const day of eachDay(cohortStart, addDays(cohortStart, step - 1))) {
    if (day > scope.today) break;
    for (const d of segmentDays(scope, day, false)) {
      weights.set(d.seg, (weights.get(d.seg) ?? 0) + d.new);
    }
  }
  const total = [...weights.values()].reduce((s, w) => s + w, 0);
  const out: number[] = new Array(periods + 1).fill(0);
  if (total <= 0) return out.map((_, p) => (p === 0 ? 1 : 0));
  for (const [seg, w] of weights) {
    segmentRates(scope, seg, cohortStart, granularity, periods).forEach((r, p) => {
      out[p] = (out[p] ?? 0) + (r * w) / total;
    });
  }
  out[0] = 1;
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
