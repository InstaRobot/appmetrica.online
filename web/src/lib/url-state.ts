// URL-состояние экрана S-05 (14-ux-ui-spec.md S-05, nuqs): период, детализация, сравнение, метрики,
// разбивка, фильтры, окружение и сервисный unit. Значения по умолчанию в URL не пишутся; неизвестные
// и некорректные значения игнорируются и заменяются умолчаниями.
import {
  createLoader,
  createParser,
  createSerializer,
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

// Порядок ключей — порядок параметров в сериализованном URL.
export const audienceSearchParams = {
  period: parseAsStringLiteral(PERIOD_PRESETS),
  from: isoDate,
  to: isoDate,
  g: parseAsStringLiteral(GRANULARITIES).withDefault("day"),
  compare: flag.withDefault(false),
  metric: parseAsStringLiteral(Object.values(TimeSeriesMetric)).withDefault("active"),
  dim: parseAsStringLiteral(Object.values(BreakdownDimension)).withDefault("app_version"),
  bmetric: parseAsStringLiteral(Object.values(BreakdownMetric)).withDefault("active"),
  platform: parseAsNativeArrayOf(parseAsStringLiteral(Object.values(Platform))),
  app_version: parseAsNativeArrayOf(parseAsString),
  os_name: parseAsNativeArrayOf(parseAsString),
  country: parseAsNativeArrayOf(countryCode),
  env: parseAsStringLiteral(Object.values(Environment)).withDefault(Environment.production),
  unit: parseAsStringLiteral(Object.values(Unit)).withDefault(Unit.persons),
};

export type AudienceSearch = inferParserType<typeof audienceSearchParams>;
export type AudienceSearchPatch = { [K in keyof AudienceSearch]?: AudienceSearch[K] | null };

export const loadAudienceSearch = createLoader(audienceSearchParams);
export const serializeAudienceSearch = createSerializer(audienceSearchParams);

export type AudienceState = {
  period: PeriodValue;
  granularity: Granularity;
  /** Детализация из URL недопустима для периода и заменена ближайшей. */
  granularityAdjusted: boolean;
  compare: boolean;
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

export function resolveAudienceState(search: AudienceSearch, today: string): AudienceState {
  const period = resolvePeriod(search, today);
  const granularity = coerceGranularity(search.g, allowedGranularities(period));
  const filters: Filters = {};
  if (search.platform.length > 0) filters.platform = search.platform;
  if (search.app_version.length > 0) filters.app_version = search.app_version;
  if (search.os_name.length > 0) filters.os_name = search.os_name;
  if (search.country.length > 0) filters.country = search.country;

  return {
    period,
    granularity,
    granularityAdjusted: granularity !== search.g,
    compare: search.compare,
    metric: search.metric,
    dim: search.dim,
    bmetric: search.bmetric,
    filters,
    env: search.env,
    unit: search.unit,
  };
}

// periodSearch — параметры URL для выбранного периода: пресет без дат, custom — с датами.
export function periodSearch(period: PeriodValue): AudienceSearchPatch {
  return period.preset === "custom"
    ? { period: "custom", from: period.from, to: period.to }
    : { period: period.preset, from: null, to: null };
}

// filtersSearch — все четыре фильтра: снятый фильтр удаляется из URL.
export function filtersSearch(filters: Filters): AudienceSearchPatch {
  return {
    platform: filters.platform ?? null,
    app_version: filters.app_version ?? null,
    os_name: filters.os_name ?? null,
    country: filters.country ?? null,
  };
}

// Общие параметры отчётов аудитории для orval-клиента.
export function reportParams(state: AudienceState): ReportParams {
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

export const retentionSearchParams = {
  cohort: parseAsStringLiteral(["day", "week"] as const).withDefault("day"),
};

export type RetentionSearch = inferParserType<typeof retentionSearchParams>;
export type RetentionSearchPatch = { [K in keyof RetentionSearch]?: RetentionSearch[K] | null };

export const loadRetentionSearch = createLoader(retentionSearchParams);
export const serializeRetentionSearch = createSerializer(retentionSearchParams);

export type RetentionState = {
  cohort: "day" | "week";
  period: PeriodValue;
  periods: number;
  filters: Filters;
  env: Environment;
  unit: Unit;
};

export function resolveRetentionState(_search: RetentionSearch, _today: string): RetentionState {
  throw new Error("not implemented");
}

export function retentionParams(_state: RetentionState): Record<string, unknown> {
  throw new Error("not implemented");
}

export function useRetentionState(_timezone: string, _today?: string) {
  throw new Error("not implemented");
  // biome-ignore lint/correctness/noUnreachable: заглушка до реализации
  return {
    state: {} as RetentionState,
    update: (_patch: RetentionSearchPatch) => Promise.resolve(),
  };
}
