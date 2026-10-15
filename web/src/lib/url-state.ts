// URL-состояние экрана S-05 (14-ux-ui-spec.md S-05, nuqs): период, детализация, сравнение, режим графика,
// метрики, разбивка, фильтры, окружение и сервисный unit. Значения по умолчанию в URL не пишутся; неизвестные
// и некорректные значения игнорируются и заменяются умолчаниями.
import {
  createParser,
  type inferParserType,
  parseAsNativeArrayOf,
  parseAsString,
  parseAsStringLiteral,
  useQueryStates,
} from "nuqs";
import {
  BreakdownDimension,
  BreakdownMetric,
  Environment,
  Platform,
  TimeSeriesMetric,
  Unit,
} from "@/lib/api/gen/model";
import {
  addDays,
  allowedGranularities,
  coerceGranularity,
  GRANULARITIES,
  type Granularity,
  isIsoDate,
  PERIOD_PRESETS,
  type PeriodValue,
  presetPeriod,
  todayIn,
} from "./period";

export const FILTER_DIMENSIONS = ["platform", "app_version", "os_name", "country"] as const;
export type FilterDimension = (typeof FILTER_DIMENSIONS)[number];
// Измерения FilterBar на страницах приложения: платформу выбирает PlatformSwitcher.
export const REPORT_FILTER_DIMENSIONS = ["app_version", "os_name", "country"] as const;
export type Filters = {
  platform?: Platform[];
  app_version?: string[];
  os_name?: string[];
  country?: string[];
};

export type ChartMetric = TimeSeriesMetric;
export type BreakdownDim = BreakdownDimension;
export type BreakdownMetricKey = BreakdownMetric;

const isoDate = createParser({
  parse: (value: string) => (isIsoDate(value) ? value : null),
  serialize: (value: string) => value,
});

// `compare=1` (14-ux-ui-spec.md S-05).
const flag = createParser({
  parse: (value: string) => (value === "1" ? true : value === "0" ? false : null),
  serialize: (value: boolean) => (value ? "1" : "0"),
});

// Страна — ISO 3166-1 alpha-2 в верхнем регистре, как в схеме фильтра api.yaml.
const countryCode = createParser({
  parse: (value: string) => (/^[A-Z]{2}$/.test(value) ? value : null),
  serialize: (value: string) => value,
});

// Фильтры, окружение и unit — общие для всех отчётов приложения (переживают смену вкладки).
const filterParams = {
  platform: parseAsNativeArrayOf(parseAsStringLiteral(Object.values(Platform))),
  app_version: parseAsNativeArrayOf(parseAsString),
  os_name: parseAsNativeArrayOf(parseAsString),
  country: parseAsNativeArrayOf(countryCode),
};
const scopeParams = {
  env: parseAsStringLiteral(Object.values(Environment)).withDefault(Environment.production),
  unit: parseAsStringLiteral(Object.values(Unit)).withDefault(Unit.persons),
};

// Порядок ключей — порядок параметров в сериализованном URL.
export const audienceSearchParams = {
  period: parseAsStringLiteral(PERIOD_PRESETS),
  from: isoDate,
  to: isoDate,
  g: parseAsStringLiteral(GRANULARITIES).withDefault("day"),
  compare: flag.withDefault(false),
  // `split=platform` — график «По платформам».
  split: parseAsStringLiteral(["platform"]),
  metric: parseAsStringLiteral(Object.values(TimeSeriesMetric)).withDefault("active"),
  dim: parseAsStringLiteral(Object.values(BreakdownDimension)).withDefault("app_version"),
  bmetric: parseAsStringLiteral(Object.values(BreakdownMetric)).withDefault("active"),
  ...filterParams,
  ...scopeParams,
};

export type AudienceSearch = inferParserType<typeof audienceSearchParams>;
export type AudienceSearchPatch = { [K in keyof AudienceSearch]?: AudienceSearch[K] | null };

export type AudienceState = {
  period: PeriodValue;
  granularity: Granularity;
  /** Детализация из URL недопустима для периода и заменена ближайшей. */
  granularityAdjusted: boolean;
  compare: boolean;
  /** Режим «По платформам» запрошен в URL; применяется при «Все» и 2+ платформах. */
  split: boolean;
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

function resolvePeriod(search: AudienceSearch, today: string): PeriodValue {
  const { period, from, to } = search;
  if (period && period !== "custom") return presetPeriod(period, today);
  // Без пресета даты задают произвольный период; неполный или перевёрнутый — «7 дней».
  if (from && to && from <= to) return { preset: "custom", from, to };
  return presetPeriod("7d", today);
}

type FilterSearch = inferParserType<typeof filterParams>;

// Пустые фильтры в состояние не попадают. Платформа — одно значение (PlatformSwitcher): из старых ссылок с
// несколькими платформами берётся первая.
function resolveFilters(search: FilterSearch): Filters {
  const filters: Filters = {};
  if (search.platform.length > 0) filters.platform = search.platform.slice(0, 1);
  if (search.app_version.length > 0) filters.app_version = search.app_version;
  if (search.os_name.length > 0) filters.os_name = search.os_name;
  if (search.country.length > 0) filters.country = search.country;
  return filters;
}

export function resolveAudienceState(search: AudienceSearch, today: string): AudienceState {
  const period = resolvePeriod(search, today);
  const granularity = coerceGranularity(search.g, allowedGranularities(period));
  const filters = resolveFilters(search);

  return {
    period,
    granularity,
    granularityAdjusted: granularity !== search.g,
    compare: search.compare,
    split: search.split === "platform",
    metric: search.metric,
    dim: search.dim,
    bmetric: search.bmetric,
    filters,
    env: search.env,
    unit: search.unit,
  };
}

// periodSearch — параметры URL для выбранного периода: пресет без дат, custom — с датами.
export function periodSearch(
  period: PeriodValue,
): Pick<AudienceSearchPatch, "period" | "from" | "to"> {
  return period.preset === "custom"
    ? { period: "custom", from: period.from, to: period.to }
    : { period: period.preset, from: null, to: null };
}

// filtersSearch — все четыре фильтра: снятый фильтр удаляется из URL.
export function filtersSearch(
  filters: Filters,
): Pick<AudienceSearchPatch, "platform" | "app_version" | "os_name" | "country"> {
  return {
    platform: filters.platform ?? null,
    app_version: filters.app_version ?? null,
    os_name: filters.os_name ?? null,
    country: filters.country ?? null,
  };
}

// Общие параметры отчётов (период, окружение, unit, фильтры) для orval-клиента.
export function reportParams(
  state: Pick<AudienceState, "period" | "env" | "unit" | "filters">,
): ReportParams {
  return {
    from: state.period.from,
    to: state.period.to,
    environment: state.env,
    unit: state.unit,
    ...state.filters,
  };
}

// useAudienceState читает состояние S-05 из URL. Каждое изменение — новая запись истории, чтобы
// «Назад» в браузере возвращал прежний период и фильтры. «Сегодня» — в TZ приложения.
export function useAudienceState(timezone: string, today: string = todayIn(timezone)) {
  const [search, setSearch] = useQueryStates(audienceSearchParams, { history: "push" });
  return {
    state: resolveAudienceState(search, today),
    update: (patch: AudienceSearchPatch) => setSearch(patch),
  };
}

// URL-состояние экрана S-06 (14-ux-ui-spec.md S-06): когорты по дням или ISO-неделям, период, фильтры,
// окружение и unit. Без периода в URL — последние 30 дневных когорт по вчера или 12 завершённых недель.
export const COHORT_GRANULARITIES = ["day", "week"] as const;
export type CohortGranularity = (typeof COHORT_GRANULARITIES)[number];

// Число периодов матрицы: D0…D30 и W0…W12 (умолчания getRetention).
export const RETENTION_PERIODS: Record<CohortGranularity, number> = { day: 30, week: 12 };

// Однодневные пресеты для когорт бессмысленны: вместо них — период по умолчанию.
export const RETENTION_PRESETS = ["7d", "30d", "90d"] as const;

export const retentionSearchParams = {
  cohort: parseAsStringLiteral(COHORT_GRANULARITIES).withDefault("day"),
  period: parseAsStringLiteral(PERIOD_PRESETS),
  from: isoDate,
  to: isoDate,
  ...filterParams,
  ...scopeParams,
};

export type RetentionSearch = inferParserType<typeof retentionSearchParams>;
export type RetentionSearchPatch = { [K in keyof RetentionSearch]?: RetentionSearch[K] | null };

export type RetentionState = {
  cohort: CohortGranularity;
  period: PeriodValue;
  periods: number;
  filters: Filters;
  env: Environment;
  unit: Unit;
};

export type RetentionParams = ReportParams & {
  cohort_granularity: CohortGranularity;
  periods: number;
};

// Период когорт по умолчанию: дни — вчера и 29 дней до него, недели — 12 ISO-недель до текущей.
export function retentionDefaultPeriod(cohort: CohortGranularity, today: string): PeriodValue {
  if (cohort === "day") {
    return { preset: "custom", from: addDays(today, -30), to: addDays(today, -1) };
  }
  const monday = addDays(today, -isoWeekday(today));
  return {
    preset: "custom",
    from: addDays(monday, -7 * RETENTION_PERIODS.week),
    to: addDays(monday, -1),
  };
}

// Номер дня ISO-недели от 0 (понедельник) до 6 (воскресенье).
function isoWeekday(date: string): number {
  return (new Date(`${date}T00:00:00Z`).getUTCDay() + 6) % 7;
}

export function resolveRetentionState(search: RetentionSearch, today: string): RetentionState {
  const { cohort, period, from, to } = search;
  let value: PeriodValue;
  if (period && (RETENTION_PRESETS as readonly string[]).includes(period)) {
    value = presetPeriod(period as (typeof RETENTION_PRESETS)[number], today);
  } else if ((!period || period === "custom") && from && to && from <= to) {
    value = { preset: "custom", from, to };
  } else {
    value = retentionDefaultPeriod(cohort, today);
  }
  return {
    cohort,
    period: value,
    periods: RETENTION_PERIODS[cohort],
    filters: resolveFilters(search),
    env: search.env,
    unit: search.unit,
  };
}

// Параметры getRetention для orval-клиента.
export function retentionParams(state: RetentionState): RetentionParams {
  const { from, to, ...rest } = reportParams(state);
  return { from, to, cohort_granularity: state.cohort, periods: state.periods, ...rest };
}

// useRetentionState читает состояние S-06 из URL; каждое изменение — новая запись истории.
export function useRetentionState(timezone: string, today: string = todayIn(timezone)) {
  const [search, setSearch] = useQueryStates(retentionSearchParams, { history: "push" });
  return {
    state: resolveRetentionState(search, today),
    update: (patch: RetentionSearchPatch) => setSearch(patch),
  };
}

// URL-состояние экрана S-07 (14-ux-ui-spec.md S-07): платформа и окружение. Период и остальные фильтры других
// вкладок остаются в URL нетронутыми, чтобы пережить переход обратно.
const realtimeSearchParams = { platform: filterParams.platform, env: scopeParams.env };

// useRealtimeState читает платформу и окружение S-07 из URL; смена — новая запись истории.
export function useRealtimeState() {
  const [search, setSearch] = useQueryStates(realtimeSearchParams, { history: "push" });
  return {
    platform: search.platform[0] ?? null,
    env: search.env,
    setPlatform: (platform: Platform | null) =>
      setSearch({ platform: platform ? [platform] : null }),
    setEnv: (env: Environment) => setSearch({ env }),
  };
}
